import type { ToolDefinition } from '@deepseek-ai/dsh-tools'
import type { FileSystem } from '@deepseek-ai/dsh-fs'
import path from 'node:path'
import os from 'node:os'
import { CheckpointError, CheckpointWriter, type CheckpointConfig, type CheckpointFs, type CheckpointInput } from './checkpoint'
import { XiaoTaoSchemaValidator } from './validate'

export interface CheckpointToolConfig {
  /** Omit to bind to the trusted calling session on every invocation. */
  projectRoot?: string
  /** Exact legacy archive root with projectRoot; archive base in automatic mode. */
  recoveryRoot?: string
}

/** Native ToolDefinition: execution stays inside DSH's approval/cancellation pipeline. */
export function checkpointTool(fs: CheckpointFs, validator: XiaoTaoSchemaValidator,
  config: CheckpointToolConfig = {}): ToolDefinition {
  return {
    name: 'xiaotao_checkpoint',
    description: '用户明确要求 XiaoTao 保存/交接，或已启用的可信 Adapter 发出 xiaotao-auto-checkpoint 生命周期提醒时：先 inspect 一个现有的活动 Temporary/Task，再使用其 revision/hash 保存有界事实快照。save 必须包含 request_id、base_revision、base_hash、snapshot。snapshot 的数组可以为空，source_refs 必须是项目内相对路径。出错后使用相同 request_id 执行 status/retry，不要随意换 ID 重复 save。快照不是 transcript 备份。不得自动创建 Task，也不得继承历史授权。',
    parameters: {
      type: 'object', additionalProperties: false, required: ['operation', 'kind', 'target_id'],
      properties: {
        operation: { type: 'string', enum: ['inspect', 'save', 'status', 'retry'] },
        kind: { type: 'string', enum: ['temporary', 'task'] }, target_id: { type: 'string' },
        request_id: { type: 'string' }, base_revision: { type: 'integer' }, base_hash: { type: 'string' },
        snapshot: { type: 'object', additionalProperties: false,
          required: ['objective', 'confirmed', 'rejected', 'in_progress', 'next', 'open_questions', 'source_refs'],
          properties: Object.fromEntries(['objective', 'confirmed', 'rejected', 'in_progress', 'next', 'open_questions',
            'source_refs'].map((key) => [key, key === 'objective' ? { type: 'string' }
              : { type: 'array', items: { type: 'string' } }])) },
      },
    },
    output: { schema: {}, render: (_args, value) => [{ type: 'text', text: JSON.stringify(value) }] },
    async execute(raw, exec) {
      const cwd = exec.agent?.session.header.cwd
      if (!cwd || !exec.agent) return { status: 'failed', code: 'caller_session_required', recovery: 'none' }
      let writer: CheckpointWriter | undefined
      try {
        exec.signal.throwIfAborted()
        if (!path.isAbsolute(cwd)) throw new CheckpointError('absolute_project_required')
        if (!raw || typeof raw !== 'object' || Array.isArray(raw)
          || Buffer.byteLength(JSON.stringify(raw)) > 32768) throw new CheckpointError('invalid_arguments')
        const args = raw as CheckpointInput & { operation: string }
        const keys = args.operation === 'save'
          ? ['operation', 'kind', 'target_id', 'request_id', 'base_revision', 'base_hash', 'snapshot']
          : args.operation === 'inspect' ? ['operation', 'kind', 'target_id']
            : ['operation', 'kind', 'target_id', 'request_id']
        if (Object.keys(raw).length !== keys.length || keys.some((key) => !(key in raw))) {
          return { status: 'failed', code: 'invalid_arguments', recovery: 'none',
            missing_fields: keys.filter((key) => !(key in raw)),
            unexpected_fields: Object.keys(raw).filter((key) => !keys.includes(key)) }
        }
        let resolved: CheckpointConfig
        if (config.projectRoot !== undefined) {
          resolved = { projectRoot: config.projectRoot, recoveryRoot: config.recoveryRoot }
        } else {
          const root = await fs.resolve(cwd, { signal: exec.signal })
          const base = config.recoveryRoot ?? path.join(process.env.DSH_HOME || path.join(os.homedir(), '.dsh'), 'xiaotao-recovery')
          if (!path.isAbsolute(base)) throw new CheckpointError('absolute_recovery_root_required')
          const baseTarget = await fs.resolve(base, { signal: exec.signal })
          if (fs.contains(root, baseTarget) || fs.contains(baseTarget, root)) {
            throw new CheckpointError('recovery_root_overlaps_project')
          }
          // CheckpointWriter owns project partitioning so configured and default
          // recovery roots share one canonical on-disk layout.
          resolved = { projectRoot: cwd, recoveryRoot: base,
            optionalRecovery: config.recoveryRoot === undefined }
        }
        const session = exec.agent.session
        const policyService = exec.agent.ctx?.get('sandboxPolicy') as {
          resolve(request: { session: typeof session }): Parameters<FileSystem['writeText']>[4]
        } | undefined
        const policy = policyService?.resolve({ session: exec.agent.session })
        const callFs: CheckpointFs = {
          resolve: (p, opts) => fs.resolve(p, opts), stat: (t, signal) => fs.stat(t, signal),
          readText: (t, signal) => fs.readText(t, signal), contains: (a, b) => fs.contains(a, b),
          listDir: (t, signal) => fs.listDir(t, signal),
          writeText: (t, content, expected, signal) =>
            (fs as FileSystem).writeText(t, content, expected, signal, policy),
        }
        writer = new CheckpointWriter(callFs, validator, resolved, String(exec.agent.id), exec.signal)
        await writer.initialize(cwd)
        switch (args.operation) {
          case 'inspect': return await writer.inspect(args.kind, args.target_id)
          case 'save': return await writer.save(args)
          case 'status': return await writer.status(args.kind, args.target_id, args.request_id)
          case 'retry': return await writer.retry(args.kind, args.target_id, args.request_id)
          default: throw new CheckpointError('invalid_operation')
        }
      } catch (error) {
        const request = raw as { request_id?: unknown; operation?: unknown } | null
        const failure = !writer
          ? { status: 'failed', code: error instanceof CheckpointError ? error.code : 'checkpoint_initialization_failed', recovery: 'none' }
          : request?.operation === 'save' || request?.operation === 'retry'
          ? await writer.reportFailure(error) : writer.failure(error)
        return { ...failure, ...(typeof request?.request_id === 'string'
          && /^[a-z0-9][a-z0-9_-]{0,63}$/.test(request.request_id) ? { request_id: request.request_id } : {}) }
      }
    },
  }
}
