const SLUG = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
const REF = /^[a-z][a-z0-9-]*:[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
const HASH = /^[a-f0-9]{64}$/;
const AUTONOMOUS = new Set([
  'inspect-project', 'run-nondestructive-checks', 'write-xiaotao-state', 'write-work-artifacts',
]);
const CONDITIONAL = new Set([
  'edit-project-files', 'external-action', 'destructive-action', 'secrets-operation',
  'permissions-change', 'scope-expansion',
]);

export function safeRelative(value) {
  return typeof value === 'string' && value.length > 0 && !value.includes('\\')
    && !value.startsWith('/') && !/^[A-Za-z]:/.test(value)
    && !value.split('/').some(part => part === '' || part === '.' || part === '..');
}

function uniqueStrings(values, predicate = value => typeof value === 'string' && value.length > 0) {
  return Array.isArray(values) && new Set(values).size === values.length && values.every(predicate);
}

export function validDelegationEnvelope(value) {
  return value && typeof value === 'object' && !Array.isArray(value)
    && Object.keys(value).length === 3
    && value.schema_version === 1
    && typeof value.session_id === 'string' && value.session_id.length > 0
    && safeRelative(value.delegation_path);
}

export function validDelegationPacket(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const required = [
    'schema_version', 'worker_id', 'objective', 'completion_condition', 'instructions',
    'context_refs', 'tools', 'permissions', 'host_adapter', 'result_path', 'handoff_path',
  ];
  const allowed = new Set([...required, 'worker_snapshot_path']);
  if (required.some(key => !(key in value)) || Object.keys(value).some(key => !allowed.has(key))) return false;
  if (value.schema_version !== 1 || !SLUG.test(value.worker_id)
    || typeof value.objective !== 'string' || value.objective.length === 0
    || typeof value.completion_condition !== 'string' || value.completion_condition.length === 0
    || !safeRelative(value.result_path) || !safeRelative(value.handoff_path)
    || (value.worker_snapshot_path !== undefined && !safeRelative(value.worker_snapshot_path))) return false;

  const instructions = value.instructions;
  if (!instructions || typeof instructions !== 'object' || Array.isArray(instructions)
    || Object.keys(instructions).length !== 3
    || !uniqueStrings(instructions.required_refs, ref => REF.test(ref)) || instructions.required_refs.length === 0
    || !uniqueStrings(instructions.optional_refs, ref => REF.test(ref))
    || instructions.required_refs.some(ref => instructions.optional_refs.includes(ref))
    || !Array.isArray(instructions.resolved)) return false;
  const knownRefs = new Set([...instructions.required_refs, ...instructions.optional_refs]);
  const resolvedRefs = new Set();
  for (const item of instructions.resolved) {
    if (!item || typeof item !== 'object' || Array.isArray(item)
      || Object.keys(item).length !== 4 || !knownRefs.has(item.ref) || resolvedRefs.has(item.ref)
      || !['core', 'project'].includes(item.source_scope)
      || !uniqueStrings(item.source_paths, safeRelative) || item.source_paths.length === 0
      || !HASH.test(item.sha256)) return false;
    resolvedRefs.add(item.ref);
  }
  if (instructions.required_refs.some(ref => !resolvedRefs.has(ref))) return false;

  if (!Array.isArray(value.context_refs) || !value.context_refs.every(item => item
    && typeof item === 'object' && !Array.isArray(item) && Object.keys(item).length === 2
    && ['current-task', 'current-temporary', 'relevant-evidence', 'memory', 'current-state', 'worker-spec', 'other'].includes(item.kind)
    && safeRelative(item.path))) return false;
  if (!uniqueStrings(value.tools)) return false;
  const permissions = value.permissions;
  if (!permissions || typeof permissions !== 'object' || Array.isArray(permissions)
    || Object.keys(permissions).length !== 2
    || !uniqueStrings(permissions.autonomous, item => AUTONOMOUS.has(item))
    || !uniqueStrings(permissions.conditional, item => CONDITIONAL.has(item))) return false;
  const adapter = value.host_adapter;
  return adapter && typeof adapter === 'object' && !Array.isArray(adapter)
    && Object.keys(adapter).length === 3 && adapter.id === 'codex' && adapter.status === 'degraded'
    && uniqueStrings(adapter.unsupported_requirements)
    && adapter.unsupported_requirements.length === 1
    && adapter.unsupported_requirements[0] === 'tool-isolation';
}
