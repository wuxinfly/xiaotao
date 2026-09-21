export const HOSTS = Object.freeze({
  codex: Object.freeze({
    id: 'codex',
    name: 'Codex / shared Agent Skills',
    skillDir: '.agents/skills/xiaotao',
    detectionPaths: ['.agents/skills', '.codex'],
  }),
  claude: Object.freeze({
    id: 'claude',
    name: 'Claude Code',
    skillDir: '.claude/skills/xiaotao',
    detectionPaths: ['.claude'],
  }),
  opencode: Object.freeze({
    id: 'opencode',
    name: 'OpenCode',
    skillDir: '.opencode/skills/xiaotao',
    detectionPaths: ['.opencode'],
  }),
});

export function parseToolList(value) {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error('AI tool list cannot be empty');
  }

  const normalized = value.trim().toLowerCase();
  if (normalized === 'all') return Object.keys(HOSTS);
  if (normalized === 'none') return [];

  const selected = [];
  for (const tool of normalized.split(',').map((item) => item.trim()).filter(Boolean)) {
    if (!HOSTS[tool]) throw new Error(`Unknown AI tool: ${tool}`);
    if (!selected.includes(tool)) selected.push(tool);
  }
  return selected;
}

export async function detectHosts(exists) {
  const detected = [];
  for (const host of Object.values(HOSTS)) {
    for (const candidate of host.detectionPaths) {
      if (await exists(candidate)) {
        detected.push(host.id);
        break;
      }
    }
  }
  return detected;
}
