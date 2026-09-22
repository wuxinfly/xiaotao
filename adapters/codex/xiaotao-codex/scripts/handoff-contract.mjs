const CAPABILITY = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;

export function isValidHandoffShape(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const required = ['status', 'summary', 'result_path', 'worker_state_path', 'needs_user_input', 'recommended_next'];
  const allowed = new Set([...required, 'questions']);
  if (required.some(key => !(key in value)) || Object.keys(value).some(key => !allowed.has(key))) return false;
  if (!['completed', 'blocked', 'failed', 'cancelled'].includes(value.status)
    || typeof value.summary !== 'string' || value.summary.length === 0
    || typeof value.result_path !== 'string' || value.result_path.length === 0
    || typeof value.worker_state_path !== 'string' || value.worker_state_path.length === 0
    || typeof value.needs_user_input !== 'boolean' || !Array.isArray(value.recommended_next)) return false;
  if (!value.recommended_next.every(item => item && typeof item === 'object' && !Array.isArray(item)
    && Object.keys(item).length === 2 && Array.isArray(item.capabilities) && item.capabilities.length > 0
    && new Set(item.capabilities).size === item.capabilities.length
    && item.capabilities.every(capability => typeof capability === 'string' && CAPABILITY.test(capability))
    && typeof item.reason === 'string' && item.reason.length > 0)) return false;
  if (value.needs_user_input) {
    return value.status === 'blocked' && Array.isArray(value.questions) && value.questions.length > 0
      && value.questions.every(item => item && typeof item === 'object' && !Array.isArray(item)
        && Object.keys(item).length === 2 && typeof item.question === 'string' && item.question.length > 0
        && typeof item.reason === 'string' && item.reason.length > 0);
  }
  return value.questions === undefined || (Array.isArray(value.questions) && value.questions.length === 0);
}
