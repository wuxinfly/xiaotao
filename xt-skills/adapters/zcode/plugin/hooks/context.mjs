// Protocol-only adapter: never writes project memory or invents save receipts.
import { lstat, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const core = fileURLToPath(new URL('../skills/xiaotao/', import.meta.url));
const rules = `使用 XiaoTao 工作流。先读取 ${path.join(core, 'SKILL.md')}，严格遵循其中的保存与授权规则。项目状态只存项目的 .xiaotao，不能存插件目录。Worker 派工前按 Core 准备 Delegation Packet，限定目标、必要上下文、可用工具、写入范围和输出契约；host_adapter.id=zcode，未实测的隔离能力必须标注 degraded/unsupported，不得仅凭提示词宣称强制隔离。Worker 用原生 Agent 工具及独立上下文，只支持一层派工；子 Worker 需要协作时返回主 Agent 安排，不能假装嵌套派工。Explore 仅用于只读探索。阶段完成及时保存，结束前检查是否有新进展：有则按 Core 调用 save_progress.py 写入当日时间线，并在有关联任务时更新进度；闲聊、重复内容不制造记录，长期知识走独立评审。保存失败必须如实说明，Hook 提醒不代表保存成功。没有 PreCompact，不承诺强退或压缩前自动保存。`;
export async function handle(event) {
  if (!event || typeof event.cwd !== 'string' || !path.isAbsolute(event.cwd)) return {};
  // Do not enroll arbitrary workspaces merely because the user enabled a global plugin.
  try {
    const dir = path.join(event.cwd, '.xiaotao');
    const info = await lstat(dir);
    const file = path.join(dir, 'installation.json');
    const stat = await lstat(file);
    if (info.isSymbolicLink() || !info.isDirectory() || stat.isSymbolicLink() || !stat.isFile() || stat.size > 32768) return {};
    const config = JSON.parse((await readFile(file, 'utf8')).replace(/^\uFEFF/, ''));
    if ((config.package !== '@wxfly/xiaotao' && config.package !== 'xiaotao-ai-workflow') || config.schema_version !== 2 || !config.scenes?.includes('zcode')) return {};
  } catch { return {}; }
  const name = event.hook_event_name ?? event.hookEventName;
  if (name === 'Stop') {
    if (event.stop_hook_active === true || event.stopHookActive === true) return {};
    return { decision: 'block', reason: 'XiaoTao 收尾检查（只补查一次）：检查本轮是否有尚未保存的新进展。有则按 Core 保存到当日时间线，关联任务同步进度并核对脚本返回结果；无新进展则直接结束，不制造空记录。失败如实报告，不反复重试。不需要在回复里重复描述本检查。' };
  }
  if (!['SessionStart', 'UserPromptSubmit'].includes(name)) return {};
  return { hookSpecificOutput: { hookEventName: name, additionalContext: rules } };
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.stdin.setEncoding('utf8');
    let raw = '';
    for await (const chunk of process.stdin) { raw += chunk; if (Buffer.byteLength(raw) > 1024 * 1024) throw new Error('input too large'); }
    process.stdout.write(JSON.stringify(await handle(JSON.parse(raw))));
  } catch (error) { process.stderr.write(`xiaotao-zcode: ${error.message}\n`); process.stdout.write('{}'); }
}
