#!/usr/bin/env node

import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const TOOL_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const RULES_PATH = "docs/agent/rules.json";
const CANDIDATES_DIR = "docs/agent/candidates";
const APPLIED_DIR = "docs/agent/candidates/applied";
const DECISIONS_DIR = "docs/agent/decisions";
const RULE_KINDS = new Set(["fact", "convention", "preference", "safety"]);
const ACTIONS = new Set(["add", "update", "remove"]);
const STABLE_ID = /^[a-z0-9][a-z0-9._-]*$/;
const SECRET_PATTERNS = [
  /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
  /\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b/,
  /\bgithub_pat_[A-Za-z0-9_]{20,}\b/,
  /\bAKIA[0-9A-Z]{16}\b/,
  /\bxox[baprs]-[A-Za-z0-9-]{10,}\b/,
];

class AgentRulesError extends Error {}

function fail(message) {
  throw new AgentRulesError(message);
}

function assertObject(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    fail(`${label} must be an object`);
  }
}

function requireString(value, label, { multiline = false } = {}) {
  if (typeof value !== "string" || !value.trim()) {
    fail(`${label} must be a non-empty string`);
  }
  if (!multiline && /[\r\n]/.test(value)) {
    fail(`${label} must stay on one line`);
  }
  return value.trim();
}

function requireStringArray(value, label, { nonEmpty = false } = {}) {
  if (!Array.isArray(value) || (nonEmpty && value.length === 0)) {
    fail(`${label} must be ${nonEmpty ? "a non-empty " : "an "}array`);
  }
  const result = value.map((item, index) => requireString(item, `${label}[${index}]`));
  if (new Set(result).size !== result.length) {
    fail(`${label} must not contain duplicates`);
  }
  return result;
}

function requireStableId(value, label) {
  const id = requireString(value, label);
  if (!STABLE_ID.test(id)) {
    fail(`${label} must match ${STABLE_ID}`);
  }
  return id;
}

function requireTimestamp(value, label) {
  const text = requireString(value, label);
  if (Number.isNaN(Date.parse(text)) || !/[zZ]|[+-]\d\d:\d\d$/.test(text)) {
    fail(`${label} must be an RFC 3339 timestamp with timezone`);
  }
  return text;
}

function assertNoSecrets(value, label) {
  const text = typeof value === "string" ? value : JSON.stringify(value);
  if (SECRET_PATTERNS.some((pattern) => pattern.test(text))) {
    fail(`${label} appears to contain credentials or private key material`);
  }
}

function readJson(path, label) {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    if (error instanceof AgentRulesError) throw error;
    fail(`cannot read ${label} at ${path}: ${error.message}`);
  }
}

function relativeProjectPath(root, path) {
  return relative(root, path).split("\\").join("/");
}

function resolveProjectPath(root, value, label) {
  const reference = requireString(value, label);
  if (isAbsolute(reference)) {
    fail(`${label} must be a project-relative path: ${reference}`);
  }
  const fullPath = resolve(root, reference);
  const relativePath = relative(root, fullPath);
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    fail(`${label} escapes the project root: ${reference}`);
  }
  return fullPath;
}

function validateRefs(root, refs, label) {
  for (const [index, reference] of refs.entries()) {
    const fullPath = resolveProjectPath(root, reference, `${label}[${index}]`);
    if (!existsSync(fullPath) || !statSync(fullPath).isFile()) {
      fail(`${label}[${index}] does not point to an existing file: ${reference}`);
    }
  }
}

function validateRuleDocument(document, root) {
  assertObject(document, RULES_PATH);
  if (document.schema_version !== 1) {
    fail(`${RULES_PATH}.schema_version must be 1`);
  }
  requireString(document.title, `${RULES_PATH}.title`);
  requireString(document.generated_path, `${RULES_PATH}.generated_path`);
  const generatedPath = resolveProjectPath(
    root,
    document.generated_path,
    `${RULES_PATH}.generated_path`,
  );
  if (!generatedPath.endsWith(".md")) {
    fail(`${RULES_PATH}.generated_path must point to a Markdown file`);
  }

  assertObject(document.policy, `${RULES_PATH}.policy`);
  assertObject(
    document.policy.minimum_observations,
    `${RULES_PATH}.policy.minimum_observations`,
  );
  for (const kind of RULE_KINDS) {
    const value = document.policy.minimum_observations[kind];
    if (!Number.isInteger(value) || value < 1) {
      fail(`${RULES_PATH}.policy.minimum_observations.${kind} must be a positive integer`);
    }
  }
  const manualReviewKinds = requireStringArray(
    document.policy.manual_review_kinds,
    `${RULES_PATH}.policy.manual_review_kinds`,
  );
  for (const kind of manualReviewKinds) {
    if (!RULE_KINDS.has(kind)) {
      fail(`unknown manual review kind '${kind}'`);
    }
  }

  if (!Array.isArray(document.sections) || document.sections.length === 0) {
    fail(`${RULES_PATH}.sections must be a non-empty array`);
  }
  const sectionIds = new Set();
  const ruleIds = new Set();
  for (const [sectionIndex, section] of document.sections.entries()) {
    const sectionLabel = `${RULES_PATH}.sections[${sectionIndex}]`;
    assertObject(section, sectionLabel);
    const sectionId = requireStableId(section.id, `${sectionLabel}.id`);
    if (sectionIds.has(sectionId)) fail(`duplicate section id '${sectionId}'`);
    sectionIds.add(sectionId);
    requireString(section.title, `${sectionLabel}.title`);
    if (!Array.isArray(section.rules)) {
      fail(`${sectionLabel}.rules must be an array`);
    }
    for (const [ruleIndex, rule] of section.rules.entries()) {
      const ruleLabel = `${sectionLabel}.rules[${ruleIndex}]`;
      assertObject(rule, ruleLabel);
      const ruleId = requireStableId(rule.id, `${ruleLabel}.id`);
      if (ruleIds.has(ruleId)) fail(`duplicate rule id '${ruleId}'`);
      ruleIds.add(ruleId);
      if (!RULE_KINDS.has(rule.kind)) {
        fail(`${ruleLabel}.kind must be one of ${[...RULE_KINDS].join(", ")}`);
      }
      const text = requireString(rule.text, `${ruleLabel}.text`);
      assertNoSecrets(text, `${ruleLabel}.text`);
      const refs = requireStringArray(rule.source_refs, `${ruleLabel}.source_refs`, {
        nonEmpty: true,
      });
      validateRefs(root, refs, `${ruleLabel}.source_refs`);
    }
  }

  return { document, generatedPath, sectionIds, ruleIds };
}

function renderDocument(document) {
  const lines = [
    "<!-- Generated by tools/agent-rules.mjs from docs/agent/rules.json. Do not edit directly. -->",
    `# ${document.title}`,
    "",
    "> 修改规则请编辑 `docs/agent/rules.json`，然后运行 `node tools/agent-rules.mjs render`。",
  ];
  for (const section of document.sections) {
    lines.push("", `## ${section.title}`);
    for (const rule of section.rules) {
      lines.push(`- ${rule.text}`);
    }
  }
  return `${lines.join("\n")}\n`;
}

function readRules(root) {
  return validateRuleDocument(readJson(resolve(root, RULES_PATH), RULES_PATH), root);
}

function loadCandidates(root, { includeApplied = false } = {}) {
  const results = [];
  const locations = [{ directory: CANDIDATES_DIR, status: "pending" }];
  if (includeApplied) {
    locations.push({ directory: APPLIED_DIR, status: "applied" });
  }
  for (const location of locations) {
    const directory = resolve(root, location.directory);
    if (!existsSync(directory)) continue;
    for (const name of readdirSync(directory).sort()) {
      if (name === "README.md" || !name.endsWith(".json")) continue;
      const candidateId = name.slice(0, -".json".length);
      const candidate = validateCandidate(
        root,
        readJson(join(directory, name), `${location.directory}/${name}`),
        candidateId,
      );
      results.push({ ...candidate, path: join(directory, name), status: location.status });
    }
  }
  return results;
}

function validateCandidate(root, candidate, filenameId) {
  assertObject(candidate, `candidate ${filenameId}`);
  if (candidate.schema_version !== 1) fail(`candidate ${filenameId}.schema_version must be 1`);
  const candidateId = requireStableId(candidate.candidate_id, `candidate ${filenameId}.candidate_id`);
  if (candidateId !== filenameId) {
    fail(`candidate filename '${filenameId}.json' must match candidate_id '${candidateId}'`);
  }
  requireTimestamp(candidate.created_at, `candidate ${candidateId}.created_at`);
  if (!ACTIONS.has(candidate.action)) {
    fail(`candidate ${candidateId}.action must be one of ${[...ACTIONS].join(", ")}`);
  }
  if (!RULE_KINDS.has(candidate.kind)) {
    fail(`candidate ${candidateId}.kind must be one of ${[...RULE_KINDS].join(", ")}`);
  }
  const sectionId = requireStableId(
    candidate.section_id,
    `candidate ${candidateId}.section_id`,
  );
  const ruleId = requireStableId(candidate.rule_id, `candidate ${candidateId}.rule_id`);
  const text = requireString(candidate.text, `candidate ${candidateId}.text`);
  assertNoSecrets(text, `candidate ${candidateId}.text`);
  requireString(candidate.rationale, `candidate ${candidateId}.rationale`);
  const refs = requireStringArray(
    candidate.source_refs,
    `candidate ${candidateId}.source_refs`,
    { nonEmpty: true },
  );
  validateRefs(root, refs, `candidate ${candidateId}.source_refs`);
  if (!Number.isInteger(candidate.observations) || candidate.observations < 1) {
    fail(`candidate ${candidateId}.observations must be a positive integer`);
  }
  if (candidate.action !== "add") {
    requireString(candidate.expected_text, `candidate ${candidateId}.expected_text`);
  }
  return { ...candidate, sectionId, ruleId, refs };
}

function findRule(document, ruleId) {
  for (const section of document.sections) {
    const index = section.rules.findIndex((rule) => rule.id === ruleId);
    if (index !== -1) return { section, rule: section.rules[index], index };
  }
  return null;
}

function validateCandidateAgainstRules(document, candidate) {
  const section = document.sections.find((item) => item.id === candidate.sectionId);
  if (!section) fail(`candidate ${candidate.candidate_id} targets unknown section '${candidate.sectionId}'`);
  const existing = findRule(document, candidate.ruleId);
  if (candidate.action === "add" && existing) {
    fail(`candidate ${candidate.candidate_id} wants to add existing rule '${candidate.ruleId}'`);
  }
  if (candidate.action !== "add") {
    if (!existing) {
      fail(`candidate ${candidate.candidate_id} targets missing rule '${candidate.ruleId}'`);
    }
    if (existing.section.id !== candidate.sectionId) {
      fail(
        `candidate ${candidate.candidate_id}.section_id does not match the current rule section`,
      );
    }
    if (existing.rule.kind !== candidate.kind) {
      fail(`candidate ${candidate.candidate_id} cannot change rule kind`);
    }
    if (existing.rule.text !== candidate.expected_text) {
      fail(
        `candidate ${candidate.candidate_id}.expected_text does not match current rule text`,
      );
    }
  }
  const minimum = document.policy.minimum_observations[candidate.kind];
  if (candidate.observations < minimum) {
    fail(
      `candidate ${candidate.candidate_id} needs at least ${minimum} observation(s) for '${candidate.kind}'`,
    );
  }
  return section;
}

function applyCandidateToDocument(document, candidate) {
  const next = structuredClone(document);
  const section = validateCandidateAgainstRules(next, candidate);
  const targetSection = next.sections.find((item) => item.id === section.id);
  const existing = findRule(next, candidate.ruleId);
  const nextRule = {
    id: candidate.ruleId,
    kind: candidate.kind,
    text: candidate.text,
    source_refs: candidate.refs,
  };
  if (candidate.action === "add") {
    targetSection.rules.push(nextRule);
  } else if (candidate.action === "update") {
    targetSection.rules[existing.index] = nextRule;
  } else {
    targetSection.rules.splice(existing.index, 1);
  }
  return { document: next, before: existing?.rule ?? null, after: candidate.action === "remove" ? null : nextRule };
}

function ensureDirectory(path) {
  mkdirSync(path, { recursive: true });
}

function writeAtomic(path, content) {
  ensureDirectory(dirname(path));
  const temporary = `${path}.tmp-${process.pid}`;
  writeFileSync(temporary, content, "utf8");
  try {
    renameSync(temporary, path);
  } catch (error) {
    rmSync(temporary, { force: true });
    throw error;
  }
}

function restoreFile(path, original) {
  if (original === null) {
    rmSync(path, { force: true });
  } else {
    writeAtomic(path, original);
  }
}

function makeDecision(candidate, before, after, reviewedBy, now) {
  return {
    schema_version: 1,
    record_type: "agent-rule-decision",
    decision_id: candidate.candidate_id,
    candidate_id: candidate.candidate_id,
    action: candidate.action,
    section_id: candidate.sectionId,
    rule_id: candidate.ruleId,
    kind: candidate.kind,
    outcome: "approved",
    decided_at: now,
    decided_by: reviewedBy ?? "user-confirmed",
    reason: candidate.rationale,
    source_refs: candidate.refs,
    before,
    after,
  };
}

function checkCommand(root) {
  const rules = readRules(root);
  const expected = renderDocument(rules.document);
  const generated = readFileSync(rules.generatedPath, "utf8");
  if (generated !== expected) {
    fail(
      `${relativeProjectPath(root, rules.generatedPath)} is stale; run 'node tools/agent-rules.mjs render'`,
    );
  }
  for (const candidate of loadCandidates(root)) {
    validateCandidateAgainstRules(rules.document, candidate);
  }
  return {
    status: "ok",
    sections: rules.document.sections.length,
    rules: rules.ruleIds.size,
    pending_candidates: loadCandidates(root).length,
  };
}

function renderCommand(root) {
  const rules = readRules(root);
  const expected = renderDocument(rules.document);
  writeAtomic(rules.generatedPath, expected);
  return { status: "rendered", path: relativeProjectPath(root, rules.generatedPath) };
}

function applyCommand(root, candidateId, confirmations, reviewedBy) {
  const id = requireStableId(candidateId, "candidate id");
  if (!confirmations.includes(id)) {
    fail(`apply requires --confirm ${id}`);
  }
  const rules = readRules(root);
  const candidatePath = resolve(root, CANDIDATES_DIR, `${id}.json`);
  if (!existsSync(candidatePath)) fail(`pending candidate '${id}' does not exist`);
  const candidate = validateCandidate(root, readJson(candidatePath, `candidate ${id}`), id);
  validateCandidateAgainstRules(rules.document, candidate);
  if (rules.document.policy.manual_review_kinds.includes(candidate.kind) && !reviewedBy) {
    fail(`candidate '${id}' requires --reviewed-by for kind '${candidate.kind}'`);
  }
  if (reviewedBy) requireString(reviewedBy, "--reviewed-by");

  const decisionPath = resolve(root, DECISIONS_DIR, `${id}.decision.json`);
  const appliedPath = resolve(root, APPLIED_DIR, `${id}.json`);
  if (existsSync(decisionPath) || existsSync(appliedPath)) {
    fail(`candidate '${id}' has already been applied`);
  }

  const mutation = applyCandidateToDocument(rules.document, candidate);
  validateRuleDocument(mutation.document, root);
  const rulesPath = resolve(root, RULES_PATH);
  const originalRules = readFileSync(rulesPath, "utf8");
  const originalGenerated = existsSync(rules.generatedPath)
    ? readFileSync(rules.generatedPath, "utf8")
    : null;
  const now = new Date().toISOString();
  const decision = makeDecision(candidate, mutation.before, mutation.after, reviewedBy, now);
  let archived = false;
  let decisionWritten = false;
  try {
    writeAtomic(rulesPath, `${JSON.stringify(mutation.document, null, 2)}\n`);
    writeAtomic(rules.generatedPath, renderDocument(mutation.document));
    writeAtomic(decisionPath, `${JSON.stringify(decision, null, 2)}\n`);
    decisionWritten = true;
    ensureDirectory(dirname(appliedPath));
    copyFileSync(candidatePath, appliedPath);
    archived = true;
    unlinkSync(candidatePath);
  } catch (error) {
    restoreFile(rulesPath, originalRules);
    restoreFile(rules.generatedPath, originalGenerated);
    if (decisionWritten) rmSync(decisionPath, { force: true });
    if (archived) {
      rmSync(appliedPath, { force: true });
      if (!existsSync(candidatePath)) writeAtomic(candidatePath, `${JSON.stringify(candidate, null, 2)}\n`);
    }
    fail(`cannot apply candidate '${id}': ${error.message}`);
  }
  return {
    status: "applied",
    candidate_id: id,
    action: candidate.action,
    rule_id: candidate.ruleId,
    decision_path: relativeProjectPath(root, decisionPath),
    archived_path: relativeProjectPath(root, appliedPath),
  };
}

function listCommand(root) {
  const rules = readRules(root);
  const candidates = loadCandidates(root);
  return {
    rules: [...rules.ruleIds].sort(),
    pending_candidates: candidates.map((candidate) => ({
      candidate_id: candidate.candidate_id,
      action: candidate.action,
      rule_id: candidate.ruleId,
      kind: candidate.kind,
    })),
  };
}

export function runCli(argv = process.argv.slice(2), root = TOOL_ROOT) {
  const parsed = parseArgs({
    args: argv,
    allowPositionals: true,
    options: {
      root: { type: "string" },
      confirm: { type: "string", multiple: true, default: [] },
      "reviewed-by": { type: "string" },
    },
  });
  root = resolve(parsed.values.root ?? root);
  const [command, ...positionals] = parsed.positionals;
  if (command === "check") {
    if (positionals.length) fail("check does not accept positional arguments");
    return checkCommand(root);
  }
  if (command === "render") {
    if (positionals.length) fail("render does not accept positional arguments");
    return renderCommand(root);
  }
  if (command === "list") {
    if (positionals.length) fail("list does not accept positional arguments");
    return listCommand(root);
  }
  if (command === "apply") {
    if (positionals.length !== 1) fail("apply requires exactly one candidate id");
    return applyCommand(
      root,
      positionals[0],
      parsed.values.confirm,
      parsed.values["reviewed-by"],
    );
  }
  fail("usage: agent-rules.mjs <check|render|list|apply> [options]");
}

function main() {
  try {
    const result = runCli();
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  } catch (error) {
    process.stderr.write(`agent rules error: ${error.message}\n`);
    process.exitCode = 2;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
