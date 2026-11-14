import type {AgentEvent, AgentMessage, AgentRunStats, AgentSession} from '../../../lib/api';
import {runPhase} from './runStats';

export async function readEventStream(response: Response, onEvent: (event: AgentEvent) => void) {
  if (!response.body) throw new Error('当前环境不支持流式消息');
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '', data: string[] = [];
  const dispatch = () => {
    if (data.length) {
      try { onEvent(JSON.parse(data.join('\n')) as AgentEvent); } catch { /* malformed frame */ }
    }
    data = [];
  };
  for (;;) {
    const {done, value} = await reader.read();
    buffer += decoder.decode(value, {stream: !done});
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (!line) dispatch();
      else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
    }
    if (done) {
      if (buffer.startsWith('data:')) data.push(buffer.slice(5).trimStart());
      dispatch();
      return;
    }
  }
}

export function mergeHistory<T extends {id: string}>(older: T[] | null | undefined, newer: T[] | null | undefined): T[] {
  const merged = new Map<string, T>();
  for (const item of [...(older ?? []), ...(newer ?? [])]) {
    const previous = merged.get(item.id);
    const before = previous as (T & {role?: string; content?: string}) | undefined;
    const after = item as T & {role?: string; content?: string};
    if (before?.role === 'assistant' && before.content && after.content !== undefined && before.content.startsWith(after.content)) continue;
    merged.set(item.id, item);
  }
  return Array.from(merged.values());
}

export function mergeRunStats(older: AgentRunStats[] | null | undefined, newer: AgentRunStats[] | null | undefined): AgentRunStats[] {
  const merged = new Map<string, AgentRunStats>();
  for (const item of [...(older ?? []), ...(newer ?? [])]) {
    const previous = merged.get(item.runId);
    if (previous?.endedAt && !item.endedAt) continue;
    merged.set(item.runId, item);
  }
  return Array.from(merged.values()).sort((a, b) => a.startedAt.localeCompare(b.startedAt));
}

export function applyEvent(session: AgentSession, event: AgentEvent): AgentSession {
  if (event.type === 'answer_end' && event.message) {
    const message = event.message;
    const existing = session.messages.find(item => item.id === message.id);
    const next = existing && existing.content.startsWith(message.content) ? existing : message;
    return {...session,
      streamDrafts: (session.streamDrafts ?? []).filter(item => item.runId !== message.runId),
      messages: existing ? session.messages.map(item => item.id === message.id ? next : item) : [...session.messages, next],
    };
  }
  if (event.type === 'message_delta' && event.message) {
    const message = event.message;
    if (session.messages.some(item => item.role === 'assistant' && item.runId === message.runId)) return session;
    const current = session.streamDrafts ?? [];
    const existing = current.find(item => item.id === message.id);
    const draft = existing ? {...existing, content: existing.content + message.content} : message;
    return {...session, streamDrafts: existing ? current.map(item => item.id === draft.id ? draft : item) : [...current, draft]};
  }
  if ((event.type === 'message' || event.type === 'session') && event.message) {
    const message = event.message;
    const existing = session.messages.find(item => item.id === message.id);
    const streamDrafts = event.type === 'message' && message.runId ? (session.streamDrafts ?? []).filter(item => item.runId !== message.runId) : session.streamDrafts;
    return {...session, streamDrafts, hasImages: session.hasImages || !!message.images?.length, status: event.status || session.status, error: event.type === 'session' ? '' : session.error, messages: existing ? session.messages.map(item => item.id === message.id ? message : item) : [...session.messages, message]};
  }
  if (event.type === 'activity' && event.activity) {
    const activity = (session.activity ?? []).filter(item => item.id !== event.activity!.id);
    const runId = event.activity.runId;
    const streamDrafts = runId ? (session.streamDrafts ?? []).filter(item => item.runId !== runId) : session.streamDrafts;
    return {...session, streamDrafts, activity: [...activity, event.activity]};
  }
  if (event.type === 'thought' && event.thought) {
    const thoughts = (session.thoughts ?? []).filter(item => item.id !== event.thought!.id);
    return {...session, thoughts: [...thoughts, event.thought]};
  }
  if (event.type === 'run_stats' && event.runStats) return {...session, runStats: mergeRunStats(session.runStats, [event.runStats])};
  if (event.type === 'status' || event.type === 'error') {
    if (event.type === 'error' && ['completed', 'failed', 'canceled'].includes(session.status)) return {...session, error: session.error || event.error};
    if (event.type === 'status' && event.status === 'running') return {...session, status: event.status, error: ''};
    const status = event.status || (event.type === 'error' ? 'failed' : session.status);
    return {...session, status, error: event.error || '', runStats: (session.runStats ?? []).map(item =>
      (!event.runId || item.runId === event.runId) && !item.endedAt && status !== 'running' && status !== 'idle'
        ? {...item, phase: status, endedAt: new Date().toISOString()} : item)};
  }
  return session;
}

export function conversationMessages(messages: AgentMessage[]): AgentMessage[] {
  const finalIds = new Map<string, string>();
  let legacyTurn = 0;
  for (const item of messages) {
    if (item.role === 'user') legacyTurn++;
    if (item.role === 'assistant' && !item.toolCalls?.length) finalIds.set(item.runId ?? `legacy:${legacyTurn}`, item.id);
  }
  const ids = new Set(finalIds.values());
  return messages.filter(item => item.role !== 'tool' && (item.role !== 'assistant' || ids.has(item.id)));
}

export function conversationHasImages(messages: AgentMessage[], hasImages?: boolean) {
  return !!hasImages || messages.some(item => !!item.images?.length);
}

export type ConversationEntry = {kind: 'message'; item: AgentMessage; stats?: AgentRunStats} | {kind: 'run'; runId: string; stats: AgentRunStats; phase: 'live' | 'done'};

export function conversationEntries(session: AgentSession | null | undefined): ConversationEntry[] {
  const runStats = session?.runStats ?? [];
  const timeline = conversationMessages(session?.messages ?? []);
  const entries: ConversationEntry[] = timeline.map(item => ({kind: 'message', item,
    stats: item.role === 'assistant' ? runStats.find(run => run.runId === item.runId) : undefined}));
  for (const run of runStats) {
    for (let index = entries.length - 1; index >= 0; index--) {
      const entry = entries[index];
      if (entry.kind === 'message' && entry.item.role === 'assistant' && entry.item.runId === run.runId) entries.splice(index, 1);
    }
    entries.push({kind: 'run', runId: run.runId, stats: run, phase: runPhase(run) === 'working' ? 'live' : 'done'});
  }
  const time = (entry: ConversationEntry) => entry.kind === 'message' ? entry.item.createdAt : entry.stats.startedAt;
  return entries.sort((a, b) => time(a).localeCompare(time(b)) || (a.kind === 'message' && a.item.role === 'user' ? -1 : 1));
}

export function avatarOwners(entries: ConversationEntry[]) {
  const owners = new Set<number>();
  let response: number | undefined;
  const finishTurn = () => {
    const owner = response;
    if (owner !== undefined) owners.add(owner);
  };
  entries.forEach((entry, index) => {
    if (entry.kind === 'message' && entry.item.role === 'user') {
      finishTurn();
      response = undefined;
    } else if (entry.kind === 'message' && entry.item.role === 'assistant') response ??= index;
  });
  finishTurn();
  return owners;
}
