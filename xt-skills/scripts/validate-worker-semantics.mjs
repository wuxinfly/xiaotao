import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const [kind, dataPath, ...extraArgs] = process.argv.slice(2);

if (!kind || !dataPath) {
  console.error(
    "Usage: node scripts/validate-worker-semantics.mjs " +
      "<requirements|worker|registry|instruction-registry|delegation> <data-path> " +
      "[--worker <worker-json>] [--builtin <registry-json>] " +
      "[--project-instructions <registry-json>] " +
      "[--core-root <path>] [--project-root <path>]",
  );
  process.exit(2);
}

function optionValue(name) {
  const index = extraArgs.indexOf(name);
  if (index < 0) return undefined;
  const value = extraArgs[index + 1];
  if (value === undefined || value.startsWith("--")) {
    console.error(`${name} requires a value`);
    process.exit(2);
  }
  return value;
}

function readJson(filePath, label) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    console.error(`Cannot read ${label} ${filePath}: ${error.message}`);
    process.exit(2);
  }
}

const data = readJson(dataPath, "data file");
const errors = [];

function validateInstructionSets(worker, label) {
  const required = new Set(worker.instructions?.required ?? []);
  const overlap = (worker.instructions?.optional ?? []).filter((ref) => required.has(ref));
  if (overlap.length > 0) {
    errors.push(`${label} instruction refs cannot be both required and optional: ${overlap.join(", ")}`);
  }
}

function setEquals(leftValues, rightValues) {
  const left = new Set(leftValues ?? []);
  const right = new Set(rightValues ?? []);
  return left.size === right.size && [...left].every((value) => right.has(value));
}

function arraysEqual(leftValues, rightValues) {
  const left = leftValues ?? [];
  const right = rightValues ?? [];
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function validateSubset(values, allowedValues, label) {
  const allowed = new Set(allowedValues ?? []);
  const expanded = (values ?? []).filter((value) => !allowed.has(value));
  if (expanded.length > 0) {
    errors.push(`${label} exceeds Worker snapshot: ${expanded.join(", ")}`);
  }
}

function pathIsWithinBoundary(candidate, boundary) {
  if (boundary === ".") return true;
  if (boundary.endsWith("/")) return candidate.startsWith(boundary);
  return candidate === boundary;
}

function validatePathBoundary(candidate, boundaries, label) {
  if (!(boundaries ?? []).some((boundary) => pathIsWithinBoundary(candidate, boundary))) {
    errors.push(`${label} exceeds Worker snapshot context boundary: ${candidate}`);
  }
}

function validateInstructionRegistry(registry, label) {
  const seen = new Set();
  const duplicates = new Set();
  for (const instruction of registry.instructions ?? []) {
    if (seen.has(instruction.ref)) duplicates.add(instruction.ref);
    seen.add(instruction.ref);
  }
  if (duplicates.size > 0) {
    errors.push(`${label} contains duplicate instruction refs: ${[...duplicates].join(", ")}`);
  }
  return seen;
}

function resolveInstructionSource(sourceScope, sourcePath, coreRoot, projectRoot) {
  const configuredRoot = sourceScope === "core" ? coreRoot : projectRoot;
  if (configuredRoot === undefined) {
    errors.push(`resolved instruction source_scope '${sourceScope}' has no trusted root`);
    return undefined;
  }
  try {
    const root = fs.realpathSync(path.resolve(configuredRoot));
    const resolved = fs.realpathSync(path.resolve(root, ...sourcePath.split("/")));
    const relative = path.relative(root, resolved);
    if (relative.startsWith("..") || path.isAbsolute(relative)) {
      errors.push(`resolved instruction source escapes its trusted root: ${sourcePath}`);
      return undefined;
    }
    return fs.readFileSync(resolved);
  } catch (error) {
    errors.push(`cannot read resolved instruction source '${sourcePath}': ${error.message}`);
    return undefined;
  }
}

function computeInstructionDigest(instruction, coreRoot, projectRoot) {
  const hash = crypto.createHash("sha256");
  let complete = true;
  for (const sourcePath of instruction.source_paths ?? []) {
    const content = resolveInstructionSource(
      instruction.source_scope,
      sourcePath,
      coreRoot,
      projectRoot,
    );
    if (content === undefined) {
      complete = false;
      continue;
    }
    hash.update(Buffer.from(sourcePath, "utf8"));
    hash.update(Buffer.from([0]));
    hash.update(content);
    hash.update(Buffer.from([0]));
  }
  return complete ? hash.digest("hex") : undefined;
}

if (kind === "requirements") {
  const required = new Set(data.required_capabilities ?? []);
  const overlap = (data.optional_capabilities ?? []).filter((capability) => required.has(capability));
  if (overlap.length > 0) {
    errors.push(`capabilities cannot be both required and optional: ${overlap.join(", ")}`);
  }
} else if (kind === "worker") {
  validateInstructionSets(data, `Worker '${data.id ?? "unknown"}'`);
} else if (kind === "registry") {
  const seen = new Set();
  const duplicates = new Set();
  for (const worker of data.workers ?? []) {
    if (seen.has(worker.id)) duplicates.add(worker.id);
    seen.add(worker.id);
    validateInstructionSets(worker, `Worker '${worker.id ?? "unknown"}'`);
  }
  if (duplicates.size > 0) {
    errors.push(`duplicate Worker IDs: ${[...duplicates].join(", ")}`);
  }
} else if (kind === "instruction-registry") {
  const currentRefs = validateInstructionRegistry(data, `Instruction registry '${data.id ?? "unknown"}'`);
  const builtinPath = optionValue("--builtin");
  if (builtinPath !== undefined) {
    const builtin = readJson(builtinPath, "built-in instruction registry");
    const builtinRefs = validateInstructionRegistry(
      builtin,
      `Built-in instruction registry '${builtin.id ?? "unknown"}'`,
    );
    const overrides = [...currentRefs].filter((ref) => builtinRefs.has(ref));
    if (overrides.length > 0) {
      errors.push(`project instruction registry overrides built-in refs: ${overrides.join(", ")}`);
    }
  }
} else if (kind === "delegation") {
  const workerPath = optionValue("--worker");
  const builtinPath = optionValue("--builtin");
  if (workerPath === undefined || builtinPath === undefined) {
    console.error(
      "delegation validation requires --worker <trusted-worker-json> and " +
        "--builtin <trusted-instruction-registry-json>",
    );
    process.exit(2);
  }
  const worker = readJson(workerPath, "trusted Worker spec");
  const builtinInstructions = readJson(builtinPath, "trusted built-in instruction registry");
  const knownInstructions = new Map(
    (builtinInstructions.instructions ?? []).map((instruction) => [instruction.ref, instruction]),
  );
  validateInstructionRegistry(
    builtinInstructions,
    `Built-in instruction registry '${builtinInstructions.id ?? "unknown"}'`,
  );
  const projectInstructionsPath = optionValue("--project-instructions");
  if (projectInstructionsPath !== undefined) {
    const projectInstructions = readJson(
      projectInstructionsPath,
      "trusted project instruction registry",
    );
    const projectRefs = validateInstructionRegistry(
      projectInstructions,
      `Project instruction registry '${projectInstructions.id ?? "unknown"}'`,
    );
    const overrides = [...projectRefs].filter((ref) => knownInstructions.has(ref));
    if (overrides.length > 0) {
      errors.push(`project instruction registry overrides built-in refs: ${overrides.join(", ")}`);
    }
    for (const instruction of projectInstructions.instructions ?? []) {
      if (!knownInstructions.has(instruction.ref)) knownInstructions.set(instruction.ref, instruction);
    }
  }
  const coreRoot = optionValue("--core-root");
  const projectRoot = optionValue("--project-root");

  if (data.worker_id !== worker.id) {
    errors.push(`packet worker_id '${data.worker_id}' does not match Worker snapshot '${worker.id}'`);
  }
  if (!setEquals(data.instructions?.required_refs, worker.instructions?.required)) {
    errors.push("packet required instruction refs must exactly match Worker snapshot");
  }
  validateSubset(
    data.instructions?.optional_refs,
    worker.instructions?.optional,
    "packet optional instruction refs",
  );
  validateSubset(data.tools, worker.tools, "packet tools");
  validateSubset(
    data.permissions?.autonomous,
    worker.permissions?.autonomous,
    "packet autonomous permissions",
  );
  validateSubset(
    data.permissions?.conditional,
    worker.permissions?.conditional,
    "packet conditional permissions",
  );
  for (const contextRef of data.context_refs ?? []) {
    validatePathBoundary(
      contextRef.path,
      worker.context?.read_paths,
      `packet context_refs '${contextRef.kind}'`,
    );
  }
  validatePathBoundary(data.result_path, worker.context?.write_paths, "packet result_path");
  validatePathBoundary(data.handoff_path, worker.context?.write_paths, "packet handoff_path");

  const required = new Set(data.instructions?.required_refs ?? []);
  const optional = new Set(data.instructions?.optional_refs ?? []);
  const adapterStatus = data.host_adapter?.status;
  const unsupportedRequirements = new Set(data.host_adapter?.unsupported_requirements ?? []);
  for (const instructionRef of [...required, ...optional]) {
    if (knownInstructions.has(instructionRef)) continue;
    const reportsUnknownInstruction = unsupportedRequirements.has(`instruction:${instructionRef}`);
    const safelyStopsForUnknownRequired =
      required.has(instructionRef) && adapterStatus === "unsupported" && reportsUnknownInstruction;
    if (!safelyStopsForUnknownRequired) {
      errors.push(`packet declares unknown instruction ref: ${instructionRef}`);
    }
  }
  const overlap = [...optional].filter((ref) => required.has(ref));
  if (overlap.length > 0) {
    errors.push(`instruction refs cannot be both required and optional: ${overlap.join(", ")}`);
  }

  const resolved = new Set();
  for (const instruction of data.instructions?.resolved ?? []) {
    if (resolved.has(instruction.ref)) {
      errors.push(`duplicate resolved instruction ref: ${instruction.ref}`);
    }
    resolved.add(instruction.ref);
    if (!required.has(instruction.ref) && !optional.has(instruction.ref)) {
      errors.push(`resolved instruction ref was not declared by the packet: ${instruction.ref}`);
    }
    const registered = knownInstructions.get(instruction.ref);
    if (registered === undefined) {
      errors.push(
        `resolved instruction ref is absent from the trusted registries: ${instruction.ref}`,
      );
    } else {
      if (
        instruction.source_scope !== registered.source_scope ||
        !arraysEqual(instruction.source_paths, registered.source_paths)
      ) {
        errors.push(
          `resolved instruction '${instruction.ref}' sources do not match the trusted registry`,
        );
      }
    }
    const actualDigest = computeInstructionDigest(instruction, coreRoot, projectRoot);
    if (actualDigest !== undefined && actualDigest !== instruction.sha256) {
      errors.push(`resolved instruction '${instruction.ref}' sha256 does not match its source_paths`);
    }
  }

  const missingRequired = [...required].filter((ref) => !resolved.has(ref));
  if (data.host_adapter?.status !== "unsupported" && missingRequired.length > 0) {
    errors.push(`required instruction refs were not resolved: ${missingRequired.join(", ")}`);
  }
  if (data.host_adapter?.status === "unsupported") {
    const unreported = missingRequired.filter(
      (ref) => !unsupportedRequirements.has(`instruction:${ref}`),
    );
    if (unreported.length > 0) {
      errors.push(`missing required instruction refs were not reported: ${unreported.join(", ")}`);
    }
  }
} else {
  console.error(`Unknown contract kind: ${kind}`);
  process.exit(2);
}

if (errors.length > 0) {
  for (const error of errors) console.error(`${dataPath}: ${error}`);
  process.exit(1);
}
