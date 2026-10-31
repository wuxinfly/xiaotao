// Read-only declaration audit. Symbol presence is NOT runtime capability detection.
import { readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';

const root = new URL('../', import.meta.url);
const specifications = [
  ['dsh-tools', 'lib/types/index.d.ts', ['register(definition: ToolDefinition): () => void;']],
  ['dsh-session-persistence', 'lib/types/index.d.ts', [
    'abstract readFrom(id: SessionId, fromSeq: number, signal?: AbortSignal): Promise<{',
    'abstract inspect(id: SessionId, signal?: AbortSignal): Promise<SessionInspection>;',
  ]],
  ['dsh-session', 'lib/types/index.d.ts', [
    'get events(): readonly SessionEvent[];',
    'get seq(): number;',
    'flush(session: Session): Promise<boolean>;',
  ]],
  ['dsh-agent', 'lib/types/runtime-types.d.ts', [
    "'agent/session-start'(", "'agent/pre-step'(", "'agent/turn-stopping'(",
    'inject(message: UserMessage): void;',
  ]],
  ['dsh-agent', 'lib/types/index.d.ts', [
    'createAgent(ownerCtx: Context, options: CreateAgentOptions): Promise<AgentHandle>;',
  ]],
  ['dsh-system-prompt', 'lib/types/index.d.ts', [
    'export interface PromptSection {', 'export interface PromptContext {',
  ]],
];

async function readJson(relative) {
  return JSON.parse(await readFile(new URL(relative, root), 'utf8'));
}

async function audit() {
  const lock = await readJson('package-lock.json');
  const evidence = [];
  for (const [name, declaration, symbols] of specifications) {
    const packagePath = `node_modules/@deepseek-ai/${name}`;
    const file = `${packagePath}/${declaration}`;
    try {
      const metadata = await readJson(`${packagePath}/package.json`);
      const bytes = await readFile(new URL(file, root));
      const lines = bytes.toString('utf8').split(/\r?\n/);
      const lockedVersion = lock.packages?.[packagePath]?.version ?? null;
      evidence.push({
        package: metadata.name,
        version: metadata.version,
        lockedVersion,
        versionMatchesLock: lockedVersion !== null && metadata.version === lockedVersion,
        file,
        sha256: createHash('sha256').update(bytes).digest('hex'),
        symbols: symbols.map((symbol) => ({
          symbol,
          line: lines.findIndex((line) => line.trim().startsWith(symbol)) + 1 || null,
        })),
      });
    } catch (error) {
      evidence.push({ file, error: error.code ?? error.name });
    }
  }
  const complete = evidence.every((entry) => !entry.error
    && entry.versionMatchesLock && entry.symbols.every((symbol) => symbol.line !== null));
  console.log(JSON.stringify({
    scope: 'installed declarations only; no live host or user state inspected',
    runtimeAvailability: 'unverified',
    checkpointActivation: 'not assessed by this script',
    complete,
    evidence,
  }, null, 2));
  if (!complete) process.exitCode = 1;
}

audit().catch((error) => {
  console.error(`Checkpoint declaration audit failed: ${error.code ?? error.name}`);
  process.exitCode = 1;
});
