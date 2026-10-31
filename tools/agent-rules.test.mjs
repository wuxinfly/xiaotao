import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import { runCli } from "./agent-rules.mjs";

const projectRoot = resolve(import.meta.dirname, "..");

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "xiaotao-agent-rules-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(join(root, "docs", "agent", "candidates", "applied"), { recursive: true });
  await mkdir(join(root, "docs", "agent", "decisions"), { recursive: true });
  await mkdir(join(root, "xt-skills"), { recursive: true });
  await writeFile(join(root, "README.md"), "# fixture\n");
  await writeFile(join(root, "xt-skills", "AGENTS.md"), "# local rules\n");
  const document = {
    schema_version: 1,
    title: "Fixture rules",
    generated_path: "AGENTS.md",
    policy: {
      minimum_observations: {
        fact: 1,
        convention: 2,
        preference: 2,
        safety: 2,
      },
      manual_review_kinds: ["convention", "preference", "safety"],
    },
    sections: [
      {
        id: "project",
        title: "Project",
        rules: [
          {
            id: "project.summary",
            kind: "fact",
            text: "Fixture project.",
            source_refs: ["README.md"],
          },
        ],
      },
    ],
  };
  await writeFile(
    join(root, "docs", "agent", "rules.json"),
    `${JSON.stringify(document, null, 2)}\n`,
  );
  runCli(["render", "--root", root]);
  return root;
}

function candidate(overrides = {}) {
  return {
    schema_version: 1,
    candidate_id: "project-summary-update",
    created_at: "2026-10-31T00:00:00Z",
    action: "update",
    kind: "fact",
    section_id: "project",
    rule_id: "project.summary",
    expected_text: "Fixture project.",
    text: "Fixture project with a client.",
    rationale: "The repository now contains a client prototype.",
    source_refs: ["README.md", "xt-skills/AGENTS.md"],
    observations: 1,
    ...overrides,
  };
}

async function putCandidate(root, value) {
  await writeFile(
    join(root, "docs", "agent", "candidates", `${value.candidate_id}.json`),
    `${JSON.stringify(value, null, 2)}\n`,
  );
}

test("rendered rules pass check and stale output is rejected", async (t) => {
  const root = await fixture(t);
  assert.equal(runCli(["check", "--root", root]).status, "ok");
  await writeFile(join(root, "AGENTS.md"), "# Hand edited\n");
  assert.throws(() => runCli(["check", "--root", root]), /stale/);
});

test("missing evidence and malformed ids are rejected", async (t) => {
  const root = await fixture(t);
  const rules = JSON.parse(await readFile(join(root, "docs", "agent", "rules.json"), "utf8"));
  rules.sections[0].rules[0].source_refs = ["missing.md"];
  await writeFile(join(root, "docs", "agent", "rules.json"), JSON.stringify(rules));
  assert.throws(() => runCli(["render", "--root", root]), /does not point to an existing file/);

  rules.sections[0].rules[0].source_refs = ["README.md"];
  await writeFile(join(root, "docs", "agent", "rules.json"), JSON.stringify(rules));
  await putCandidate(root, candidate({ candidate_id: "Bad ID" }));
  assert.throws(() => runCli(["check", "--root", root]), /candidate_id/);
});

test("candidate expected text prevents stale overwrite", async (t) => {
  const root = await fixture(t);
  await putCandidate(root, candidate({ expected_text: "Old text." }));
  assert.throws(() => runCli(["check", "--root", root]), /expected_text/);
});

test("candidate section must match the existing rule", async (t) => {
  const root = await fixture(t);
  const rules = JSON.parse(await readFile(join(root, "docs", "agent", "rules.json"), "utf8"));
  rules.sections.push({ id: "other", title: "Other", rules: [] });
  await writeFile(join(root, "docs", "agent", "rules.json"), JSON.stringify(rules));
  runCli(["render", "--root", root]);
  await putCandidate(root, candidate({ section_id: "other" }));
  assert.throws(() => runCli(["check", "--root", root]), /does not match the current rule section/);
});

test("source refs cannot escape through a sibling path", async (t) => {
  const root = await fixture(t);
  await writeFile(join(root, "docs", "agent", "rules.json"), JSON.stringify({
    schema_version: 1,
    title: "Fixture rules",
    generated_path: "AGENTS.md",
    policy: {
      minimum_observations: { fact: 1, convention: 2, preference: 2, safety: 2 },
      manual_review_kinds: ["convention", "preference", "safety"],
    },
    sections: [{
      id: "project",
      title: "Project",
      rules: [{
        id: "project.summary",
        kind: "fact",
        text: "Fixture project.",
        source_refs: ["..\\fixture-sibling\\README.md"],
      }],
    }],
  }));
  assert.throws(() => runCli(["render", "--root", root]), /escapes the project root/);
});

test("convention candidates require review and two observations", async (t) => {
  const root = await fixture(t);
  await putCandidate(
    root,
    candidate({
      candidate_id: "project-convention",
      action: "add",
      kind: "convention",
      rule_id: "project.tests",
      text: "Run tests before delivery.",
      observations: 1,
      expected_text: undefined,
    }),
  );
  assert.throws(() => runCli(["check", "--root", root]), /at least 2 observation/);
  const valid = candidate({
    candidate_id: "project-convention",
    action: "add",
    kind: "convention",
    rule_id: "project.tests",
    text: "Run tests before delivery.",
    observations: 2,
    expected_text: undefined,
  });
  await putCandidate(root, valid);
  assert.throws(
    () => runCli(["apply", valid.candidate_id, "--confirm", valid.candidate_id, "--root", root]),
    /requires --reviewed-by/,
  );
});

test("apply updates source, generated entry, and immutable decision", async (t) => {
  const root = await fixture(t);
  const value = candidate();
  await putCandidate(root, value);
  const result = runCli([
    "apply",
    value.candidate_id,
    "--confirm",
    value.candidate_id,
    "--root",
    root,
  ]);
  assert.equal(result.status, "applied");
  assert.match(await readFile(join(root, "AGENTS.md"), "utf8"), /client/);
  const decision = JSON.parse(
    await readFile(join(root, "docs", "agent", "decisions", `${value.candidate_id}.decision.json`), "utf8"),
  );
  assert.equal(decision.outcome, "approved");
  assert.equal(decision.after.text, value.text);
  await assert.rejects(
    readFile(join(root, "docs", "agent", "candidates", `${value.candidate_id}.json`)),
  );
  assert.equal(runCli(["check", "--root", root]).pending_candidates, 0);
});

test("repository rule source is valid and generated AGENTS.md is current", () => {
  const result = runCli(["check", "--root", projectRoot]);
  assert.equal(result.status, "ok");
  assert.ok(result.rules >= 10);
});
