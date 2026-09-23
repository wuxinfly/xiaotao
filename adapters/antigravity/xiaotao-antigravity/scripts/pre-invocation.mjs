import { readdir, readFile, realpath, stat } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const MAX_DIRECTORY_ENTRIES = 128;
const MAX_METADATA_BYTES = 16 * 1024;
const MAX_TOTAL_BYTES = 128 * 1024;
const MAX_ACTIVE_TASKS = 64;

export class ScanBudgetError extends Error {
  constructor() {
    super('scan_budget_exceeded');
    this.code = 'scan_budget_exceeded';
  }
}

export function scanBudget() {
  return { bytes: 0 };
}

async function boundedEntries(directory) {
  const entries = await readdir(directory);
  if (entries.length > MAX_DIRECTORY_ENTRIES) throw new ScanBudgetError();
  return entries;
}

async function boundedText(file, budget) {
  const info = await stat(file);
  if (!info.isFile() || info.size > MAX_METADATA_BYTES || budget.bytes + info.size > MAX_TOTAL_BYTES) {
    throw new ScanBudgetError();
  }
  budget.bytes += info.size;
  return readFile(file, 'utf8');
}

function yamlScalar(source, key) {
  const match = new RegExp(`^${key}:\\s*["']?([^"'\\r\\n#]+)`, 'm').exec(source);
  return match ? match[1].trim() : null;
}

function inside(root, target) {
  const relative = path.relative(root, target);
  return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
}

async function exists(target) {
  try {
    await stat(target);
    return true;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

async function isXiaoTaoProject(root) {
  return (await exists(path.join(root, '.xiaotao'))) || (await exists(path.join(root, '.xiaotao/installation.json')));
}

export async function loadBoundedRuntimeContext(root, budget = scanBudget()) {
  const context = {
    activeTasks: [],
    recentHandoffs: [],
    recoverableCheckpoints: [],
    pendingFollowUps: [],
  };

  // 1. Scan tasks
  const tasksDir = path.join(root, '.xiaotao/tasks');
  if (await exists(tasksDir)) {
    try {
      const taskEntries = await boundedEntries(tasksDir);
      for (const entry of taskEntries.slice(0, MAX_ACTIVE_TASKS)) {
        const taskDir = path.join(tasksDir, entry);
        const taskYaml = path.join(taskDir, 'task.yaml');
        if (await exists(taskYaml)) {
          const content = await boundedText(taskYaml, budget);
          const status = yamlScalar(content, 'status');
          const id = yamlScalar(content, 'id') || entry;
          const objective = yamlScalar(content, 'objective') || '';
          if (status === 'active') {
            context.activeTasks.push({ id, objective });

            // Check recent handoffs within active task
            const handoffsDir = path.join(taskDir, 'handoffs');
            if (await exists(handoffsDir)) {
              const hEntries = await boundedEntries(handoffsDir);
              for (const h of hEntries.filter((f) => f.endsWith('.json')).slice(-2)) {
                try {
                  const hData = JSON.parse(await boundedText(path.join(handoffsDir, h), budget));
                  if (hData && typeof hData === 'object' && hData.status) {
                    context.recentHandoffs.push({
                      taskId: id,
                      status: hData.status,
                      summary: hData.detailed_result?.summary || hData.blockage?.reason || '',
                    });
                  }
                } catch (error) {
                  if (error instanceof ScanBudgetError) throw error;
                }
              }
            }
          }
        }
      }
    } catch (error) {
      if (error instanceof ScanBudgetError) throw error;
    }
  }

  // 2. Scan recoverable checkpoints
  const chkDir = path.join(root, '.xiaotao/checkpoints');
  if (await exists(chkDir)) {
    try {
      const files = await boundedEntries(chkDir);
      for (const f of files.filter((n) => n.endsWith('.json') && !n.includes('.failed-') && !n.includes('.committed.'))) {
        try {
          const data = JSON.parse(await boundedText(path.join(chkDir, f), budget));
          if (data?.request_id && (data.status === 'active' || data.status === 'saved')) {
            context.recoverableCheckpoints.push({
              requestId: data.request_id,
              targetId: data.target_id || '',
              kind: data.kind || 'checkpoint',
            });
          }
        } catch (error) {
          if (error instanceof ScanBudgetError) throw error;
        }
      }
    } catch (error) {
      if (error instanceof ScanBudgetError) throw error;
    }
  }

  // 3. Scan pending follow-ups
  const fuDir = path.join(root, '.xiaotao/memory/followups/pending');
  if (await exists(fuDir)) {
    try {
      const files = await boundedEntries(fuDir);
      for (const f of files.filter((n) => n.endsWith('.yaml') || n.endsWith('.yml'))) {
        try {
          const content = await boundedText(path.join(fuDir, f), budget);
          const id = yamlScalar(content, 'id') || f.replace(/\.ya?ml$/, '');
          const status = yamlScalar(content, 'status');
          if (status === 'pending' || !status) {
            context.pendingFollowUps.push({ id });
          }
        } catch (error) {
          if (error instanceof ScanBudgetError) throw error;
        }
      }
    } catch (error) {
      if (error instanceof ScanBudgetError) throw error;
    }
  }

  return context;
}

export function buildEphemeralReminder(context) {
  const lines = [
    '【小涛 (XiaoTao) 运行时锚点与 Core Guard】',
    '- 身份：你是小涛，唯一预置且直接面向用户的角色，始终用简洁的大白话中文先汇报结果。',
    '- 派工边界：单会话明确小改动默认轻任务直通；需恢复或多执行单元时升级 Task；确需工具隔离、缺失能力或用户明确委派时才使用原生子代理。',
    '- 工具隔离：调查或 Memory 提议等只读 Worker 必须设 enable_write_tools: false 落实物理隔离。',
    '- 记忆约束：Memory Worker 仅限输出候选提案（CREATE/UPDATE/MERGE/SKIP），绝对禁止自我批准或直接修改正式记忆。',
  ];

  const stateItems = [];
  if (context.activeTasks.length > 0) {
    const taskSummaries = context.activeTasks.slice(0, 3).map((t) => `${t.id}${t.objective ? `(${t.objective})` : ''}`);
    stateItems.push(`进行中任务: ${taskSummaries.join(', ')}`);
  }
  if (context.recentHandoffs.length > 0) {
    const handoffSummaries = context.recentHandoffs.slice(0, 2).map((h) => `${h.taskId}[${h.status}]${h.summary ? `: ${h.summary}` : ''}`);
    stateItems.push(`最新交接: ${handoffSummaries.join('; ')}`);
  }
  if (context.recoverableCheckpoints.length > 0) {
    stateItems.push(`可恢复检查点: ${context.recoverableCheckpoints.length} 个`);
  }
  if (context.pendingFollowUps.length > 0) {
    stateItems.push(`待跟进事项: ${context.pendingFollowUps.length} 个`);
  }

  if (stateItems.length > 0) {
    lines.push(`- 项目已有状态（请主动感知推进，不要回答“目前没有具体任务”）：${stateItems.join(' | ')}`);
  }

  return lines.join('\n');
}

export async function run(input, output, errorOutput = process.stderr) {
  let raw = '';
  for await (const chunk of input) raw += chunk;
  if (!raw.trim()) {
    output.write('{}\n');
    return;
  }

  let payload;
  try {
    payload = JSON.parse(raw);
  } catch {
    output.write('{}\n');
    return;
  }

  // PreInvocation fires on every turn. Only inject runtime context on invocationNum === 1
  if (payload.invocationNum !== undefined && Number(payload.invocationNum) > 1) {
    output.write('{}\n');
    return;
  }

  const workspaceRoot = Array.isArray(payload.workspacePaths) && payload.workspacePaths.length > 0 ? payload.workspacePaths[0] : null;

  if (!workspaceRoot) {
    output.write('{}\n');
    return;
  }

  let root;
  try {
    root = await realpath(path.resolve(workspaceRoot));
  } catch {
    output.write('{}\n');
    return;
  }

  if (!(await isXiaoTaoProject(root))) {
    output.write('{}\n');
    return;
  }

  try {
    const budget = scanBudget();
    const context = await loadBoundedRuntimeContext(root, budget);
    const reminder = buildEphemeralReminder(context);

    output.write(
      `${JSON.stringify({
        injectSteps: [
          {
            ephemeralMessage: reminder,
          },
        ],
      })}\n`
    );
  } catch (error) {
    if (error instanceof ScanBudgetError) {
      output.write(
        `${JSON.stringify({
          injectSteps: [
            {
              ephemeralMessage:
                '【小涛 (XiaoTao) 运行时警告】项目权威元数据超出扫描预算，已安全降级为 Bare Core 模式。请按需查阅 .xiaotao/ 状态。',
            },
          ],
        })}\n`
      );
      return;
    }
    errorOutput.write(`[xiaotao-antigravity] Unexpected scan error: ${error.message}\n`);
    output.write('{}\n');
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  run(process.stdin, process.stdout, process.stderr).catch(() => {
    process.stdout.write('{}\n');
  });
}
