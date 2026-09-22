const WRITE_TOOLS = new Set([
  'write_to_file',
  'replace_file_content',
  'run_command',
]);

export function sanitizeSubagentName(id) {
  if (!id || typeof id !== 'string') return 'xiaotao_worker';
  return id.replace(/[^a-zA-Z0-9_.-]/g, '_').toLowerCase();
}

export function isReadOnlyToolList(tools = []) {
  if (!Array.isArray(tools) || tools.length === 0) return true;
  return !tools.some((t) => WRITE_TOOLS.has(t));
}

export function mapDelegationToSubagent(packet, workerSpec = {}) {
  if (!packet || typeof packet !== 'object') {
    throw new Error('Invalid delegation packet');
  }

  const workerId = packet.worker_id || workerSpec.id || 'worker';
  const displayName = workerSpec.display_name || workerId;
  const objective = packet.objective || '';

  const tools = Array.isArray(packet.effective_tools)
    ? packet.effective_tools
    : Array.isArray(workerSpec.tools)
      ? workerSpec.tools
      : [];

  const readOnly = isReadOnlyToolList(tools);

  const instructions = Array.isArray(packet.effective_instructions)
    ? packet.effective_instructions
    : [];

  const systemPromptLines = [
    `你是 XiaoTao Worker：【${displayName}】`,
    `目标：${objective}`,
  ];

  if (instructions.length > 0) {
    systemPromptLines.push('遵循指令：');
    for (const inst of instructions) {
      systemPromptLines.push(`- ${inst.id || inst.path || JSON.stringify(inst)}`);
    }
  }

  if (readOnly) {
    systemPromptLines.push('【约束】你处于只读审查/调查模式，宿主已关闭写权限，仅使用分析与查找工具。');
  }

  return {
    defineSubagentArgs: {
      name: sanitizeSubagentName(workerId),
      description: displayName,
      system_prompt: systemPromptLines.join('\n'),
      enable_write_tools: !readOnly,
      enable_subagent_tools: false,
      enable_mcp_tools: false,
    },
    invokeSubagentArgs: {
      TypeName: sanitizeSubagentName(workerId),
      Role: displayName,
      Prompt: objective,
    },
    toolIsolationEnforced: readOnly,
  };
}
