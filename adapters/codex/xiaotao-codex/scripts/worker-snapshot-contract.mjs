import { safeRelative } from './delegation-contract.mjs';

const SLUG = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
const AUTONOMOUS = new Set([
  'inspect-project', 'run-nondestructive-checks', 'write-xiaotao-state', 'write-work-artifacts',
]);
const CONDITIONAL = new Set([
  'edit-project-files', 'external-action', 'destructive-action', 'secrets-operation',
  'permissions-change', 'scope-expansion',
]);

function scalar(source) {
  const value = source.trim();
  if (value === '[]') return [];
  if (value === '{}') return {};
  if (/^-?(?:0|[1-9][0-9]*)$/.test(value)) return Number(value);
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (value === 'null' || value === '~') return null;
  if (value.startsWith('"')) return JSON.parse(value);
  if (value.startsWith("'")) {
    if (!value.endsWith("'")) throw new Error('yaml_quote_invalid');
    return value.slice(1, -1).replaceAll("''", "'");
  }
  if (!value || /[\[\]{},]|^(?:---|\.\.\.)$/.test(value)
    || /(?:^|\s)[&*!]|<<\s*:|[|>]$/.test(value)) throw new Error('yaml_scalar_unsupported');
  return value;
}

export function parseWorkerSnapshotYaml(source) {
  if (typeof source !== 'string' || source.includes('\t') || source.includes('\0')) {
    throw new Error('yaml_input_invalid');
  }
  const root = {};
  const stack = [{ indent: -1, value: root }];
  const lines = source.replace(/^\uFEFF/, '').split(/\r?\n/);
  for (let index = 0; index < lines.length; index++) {
    const raw = lines[index];
    if (!raw.trim() || raw.trimStart().startsWith('#')) continue;
    const indent = raw.length - raw.trimStart().length;
    if (indent % 2 !== 0) throw new Error('yaml_indent_invalid');
    const text = raw.slice(indent);
    while (stack.at(-1).indent >= indent) stack.pop();
    const parent = stack.at(-1)?.value;
    if (!parent) throw new Error('yaml_parent_invalid');
    if (text.startsWith('- ')) {
      if (!Array.isArray(parent)) throw new Error('yaml_sequence_invalid');
      parent.push(scalar(text.slice(2)));
      continue;
    }
    const match = /^([^:#][^:]*):(?:\s*(.*))?$/.exec(text);
    if (!match || Array.isArray(parent)) throw new Error('yaml_mapping_invalid');
    const key = match[1].trim();
    if (!key || Object.hasOwn(parent, key)) throw new Error('yaml_key_invalid');
    const tail = match[2] ?? '';
    if (tail !== '') {
      parent[key] = scalar(tail);
      continue;
    }
    let next = '';
    for (let look = index + 1; look < lines.length; look++) {
      if (lines[look].trim() && !lines[look].trimStart().startsWith('#')) { next = lines[look]; break; }
    }
    const nextIndent = next ? next.length - next.trimStart().length : -1;
    const child = nextIndent > indent && next.slice(nextIndent).startsWith('- ') ? [] : {};
    parent[key] = child;
    stack.push({ indent, value: child });
  }
  return root;
}

function strings(value, predicate = item => typeof item === 'string' && item.length > 0) {
  return Array.isArray(value) && new Set(value).size === value.length && value.every(predicate);
}

function subset(values, limits) {
  const allowed = new Set(limits);
  return values.every(value => allowed.has(value));
}

function sameSet(left, right) {
  return left.length === right.length && subset(left, right);
}

function within(relative, boundary) {
  return relative === boundary || relative.startsWith(`${boundary}/`);
}

export function validWorkerSnapshot(snapshot) {
  return snapshot && typeof snapshot === 'object' && !Array.isArray(snapshot)
    && snapshot.schema_version === 2 && SLUG.test(snapshot.id) && snapshot.status === 'active'
    && strings(snapshot.tools)
    && snapshot.instructions && strings(snapshot.instructions.required) && snapshot.instructions.required.length > 0
    && strings(snapshot.instructions.optional)
    && !snapshot.instructions.required.some(ref => snapshot.instructions.optional.includes(ref))
    && snapshot.context && strings(snapshot.context.read_paths, safeRelative)
    && strings(snapshot.context.write_paths, safeRelative)
    && snapshot.permissions
    && strings(snapshot.permissions.autonomous, item => AUTONOMOUS.has(item))
    && strings(snapshot.permissions.conditional, item => CONDITIONAL.has(item));
}

export function packetWithinWorker(packet, snapshot) {
  if (!validWorkerSnapshot(snapshot) || snapshot.id !== packet.worker_id) return false;
  return subset(packet.tools, snapshot.tools)
    && subset(packet.permissions.autonomous, snapshot.permissions.autonomous)
    && subset(packet.permissions.conditional, snapshot.permissions.conditional)
    && sameSet(packet.instructions.required_refs, snapshot.instructions.required)
    && subset(packet.instructions.optional_refs, snapshot.instructions.optional)
    && packet.context_refs.every(ref => snapshot.context.read_paths.some(limit => within(ref.path, limit)))
    && [packet.result_path, packet.handoff_path]
      .every(output => snapshot.context.write_paths.some(limit => within(output, limit)));
}
