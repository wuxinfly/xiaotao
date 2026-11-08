/**
 * 格式化模型上下文窗口大小。
 *
 * 实施要求：
 * - 有有效值时，以易读格式展示，例如 "上下文 128K tokens"；格式化时不要造成明显精度损失，必要时显示完整数字。
 * - 字段缺失、为零或无效时显示 "上下文未知"，不要使用推测值。
 * - 该值是目录提供的最大输入 token 数，不要表述成已经验证的实际可用容量。
 */
export function formatTokens(tokens: number): string {
  // 1. 标准整 M（十进制优先，再二进制，避免混淆）
  if (tokens >= 1_000_000 && tokens % 1_000_000 === 0) {
    return `${tokens / 1_000_000}M tokens`;
  }
  if (tokens >= 1_048_576 && tokens % 1_048_576 === 0) {
    return `${tokens / 1_048_576}M tokens`;
  }
  // 2. 带小数的 M（如 1.5M）
  if (tokens >= 1_000_000 && tokens % 100_000 === 0) {
    return `${(tokens / 1_000_000).toFixed(1)}M tokens`;
  }
  // 3. 标准整 K（十进制 1000 优先如 128000/200000/272000，再二进制 1024 如 131072/32768/8192）
  if (tokens >= 1000 && tokens % 1000 === 0) {
    return `${tokens / 1000}K tokens`;
  }
  if (tokens >= 1024 && tokens % 1024 === 0) {
    return `${tokens / 1024}K tokens`;
  }
  // 4. 带小数的 K（如 12.5K）
  if (tokens >= 1000 && tokens % 100 === 0) {
    return `${(tokens / 1000).toFixed(1)}K tokens`;
  }
  // 5. 其余非规则值，直接完整显示避免精度损失
  return `${tokens.toLocaleString('en-US')} tokens`;
}

export function formatContextWindow(tokens?: number | null): string {
  if (tokens === undefined || tokens === null || typeof tokens !== 'number' || !Number.isFinite(tokens) || tokens <= 0) {
    return '上下文未知';
  }
  return `上下文 ${formatTokens(tokens)}`;
}

export function getContextWindowTitle(tokens?: number | null): string {
  if (tokens === undefined || tokens === null || typeof tokens !== 'number' || !Number.isFinite(tokens) || tokens <= 0) {
    return '目录未提供最大输入上下文';
  }
  return '目录声明的最大输入上下文，以实际可用为准';
}
