import {normaliseImage} from './imageNormalise';
import {AgentDroplet, activityState, type DropletState} from './AgentDroplet';
import {Popover} from '../../components/popover/Popover';
import {ModalPresence} from '../../components/modal/ModalFrame';
import {Fragment as ReactFragment, memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {SessionList} from './SessionList';
import {createPortal} from 'react-dom';
import {renderToStaticMarkup} from 'react-dom/server';
import {EditorContent, useEditor} from '@tiptap/react';
import type {Editor, JSONContent} from '@tiptap/core';
import {NodeSelection, TextSelection} from '@tiptap/pm/state';
import type {EditorState} from '@tiptap/pm/state';
import {DOMParser as ProseMirrorDOMParser, Fragment, Slice} from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import {
  ArrowUp,
  ArrowDown,
  CircleAlert,
  CircleCheck,
  CircleStop,
  Clock3,
  FilePenLine,
  FileSearch,
  Gauge,
  Image,
  SendHorizontal,
  ListFilter,
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  Folder,
  MessageSquarePlus,
  Plus,
  Pencil,
  Search,
  Settings2,
  Trash2,
  Square,
  PanelRight,
  Route,
  Terminal,
  TriangleAlert,
  Wrench,
  X,
  Zap,
  History,
  Sparkles,
  Loader2,
} from 'lucide-react';
import {ModelPriceBadge} from '../../components/ModelPriceBadge';
import {api, type AgentActivity, type AgentConversationReference, type AgentDefaultModels, type AgentEvent, type AgentMessage, type AgentModelConfig, type AgentModelDefaults, type AgentModelOption, type AgentProject, type AgentReferenceCandidate, type AgentRunStats, type AgentSession, type AgentThought, type BehaviorReview} from '../../lib/api';
import {modelConfigFor, modelLabelsKey, modelSettingsFor, readEnabledModels, readModelLabels, utilityModelFor, useAgentModelSettings} from '../../lib/agent-model-settings';
import {AgentSettingsModal} from './AgentSettingsModal';
import {ProjectModal, RenameSessionModal} from './AgentModals';
import {ReferenceModal, ReferencePicker} from './ReferencePicker';
import {ReviewListModal} from './ReviewListModal';
import {ReviewDetailModal} from './ReviewDetailModal';
import {ConversationReference, conversationReferenceType} from './ConversationReference';
import './agent.css';
import './agent-sidebar.css';
import {useConfirm} from '../../components/confirm/ConfirmProvider';

declare global { interface Window { xtPickFolder?: () => Promise<string>; xtOpenDirectory?: (kind: 'data' | 'install') => Promise<void>; } }

async function readEventStream(response: Response, onEvent: (event: AgentEvent) => void) {
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

function mergeHistory<T extends {id: string}>(older: T[] | null | undefined, newer: T[] | null | undefined): T[] {
  const merged = new Map<string, T>();
  for (const item of [...(older ?? []), ...(newer ?? [])]) {
    const previous = merged.get(item.id);
    const before = previous as (T & {role?: string; content?: string}) | undefined;
    const after = item as T & {role?: string; content?: string};
    // A history snapshot may predate an SSE fragment. Formal answers are monotonic.
    if (before?.role === 'assistant' && before.content && after.content !== undefined && before.content.startsWith(after.content)) continue;
    merged.set(item.id, item);
  }
  return Array.from(merged.values());
}

function mergeRunStats(older: AgentRunStats[] | null | undefined, newer: AgentRunStats[] | null | undefined): AgentRunStats[] {
  const merged = new Map<string, AgentRunStats>();
  for (const item of [...(older ?? []), ...(newer ?? [])]) {
    const previous = merged.get(item.runId);
    if (previous?.endedAt && !item.endedAt) continue;
    merged.set(item.runId, item);
  }
  return Array.from(merged.values()).sort((a, b) => a.startedAt.localeCompare(b.startedAt));
}

const thoughtKindLabel: Record<AgentThought['kind'], string> = {
  understanding: '理解',
  observation: '进展',
  decision: '决定',
  checkpoint: '阶段',
};

type ThoughtTraceRow = {
  id: string;
  time: string;
  status: 'running' | 'completed' | 'failed';
  text: string;
} & (
  {kind: 'thought'; thoughtKind: AgentThought['kind']}
  | {kind: 'activity'; activityKind: string; title: string; detail?: string; tool?: string; input?: string; output?: string; startedAt: string; endedAt?: string}
);

function traceSummary(row: ThoughtTraceRow) {
  const source = (() => {
    if (row.kind !== 'activity' || row.activityKind !== 'run_command') return row.text;
    const command = row.input ? (() => {
      try {
        const parsed = JSON.parse(row.input) as {command?: unknown};
        return typeof parsed.command === 'string' ? parsed.command : '';
      } catch {
        return '';
      }
    })() : '';
    return command || row.detail || row.text;
  })();
  return source.replace(/\s+/g, ' ').trim();
}

function thoughtTraceIcon(row: ThoughtTraceRow) {
  if (row.kind === 'activity') {
    if (row.activityKind === 'write_file') return FilePenLine;
    if (row.activityKind === 'read_file' || row.activityKind === 'list_files' || row.activityKind === 'search_files') return FileSearch;
    return Terminal;
  }
  if (row.thoughtKind === 'understanding') return Route;
  if (row.thoughtKind === 'observation') return FileSearch;
  if (row.thoughtKind === 'decision') return Zap;
  return CircleCheck;
}

function traceOffset(startedAt: string, time: string) {
  const start = new Date(startedAt).getTime();
  const current = new Date(time).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(current)) return '';
  return `+${Math.max(0, Math.round((current - start) / 1000))}s`;
}

function traceStatusLabel(status: ThoughtTraceRow['status']) {
  return status === 'running' ? '进行中' : status === 'failed' ? '失败' : '完成';
}

function runPhase(stats: AgentRunStats): NonNullable<AgentRunStats['phase']> {
  return stats.phase ?? (stats.endedAt ? 'completed' : 'working');
}

function applyEvent(session: AgentSession, event: AgentEvent): AgentSession {
  if (event.type === 'answer_end' && event.message) {
    const message = event.message;
    const existing = session.messages.find(item => item.id === message.id);
    // start is idempotent; deltas append only to the formal answer, never drafts.
    const next = existing && existing.content.startsWith(message.content) ? existing : message;
    return {...session,
      streamDrafts: (session.streamDrafts ?? []).filter(item => item.runId !== message.runId),
      messages: existing ? session.messages.map(item => item.id === message.id ? next : item) : [...session.messages, next],
    };
  }
  if (event.type === 'message_delta' && event.message) {
    if (session.messages.some(item => item.role === 'assistant' && item.runId === event.message!.runId)) return session;
    const current = session.streamDrafts ?? [];
    const existing = current.find(item => item.id === event.message!.id);
    const draft = existing ? {...existing, content: existing.content + event.message.content} : event.message;
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
  if (event.type === 'run_stats' && event.runStats) {
    return {...session, runStats: mergeRunStats(session.runStats, [event.runStats])};
  }
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

function conversationMessages(messages: AgentMessage[]): AgentMessage[] {
  const finalIds = new Map<string, string>();
  let legacyTurn = 0;
  for (const item of messages) {
    if (item.role === 'user') legacyTurn++;
    if (item.role === 'assistant' && !item.toolCalls?.length) finalIds.set(item.runId ?? `legacy:${legacyTurn}`, item.id);
  }
  const ids = new Set(finalIds.values());
  return messages.filter(item => item.role !== 'tool' && (item.role !== 'assistant' || ids.has(item.id)));
}

function conversationHasImages(messages: AgentMessage[], hasImages?: boolean) {
  return !!hasImages || messages.some(item => !!item.images?.length);
}

function runDuration(stats: AgentRunStats) {
  const start = new Date(stats.startedAt).getTime();
  const end = stats.endedAt ? new Date(stats.endedAt).getTime() : Date.now();
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

function runSpeed(stats: AgentRunStats) {
  if (!stats.outputSpeed || stats.outputSpeed <= 0) return '';
  return `${stats.outputSpeed.toFixed(1).replace(/\.0$/, '')} tok/s`;
}

// runElapsed is the wall clock time of the run, in whole seconds.
function runElapsed(stats: AgentRunStats, now: number) {
  const start = new Date(stats.startedAt).getTime();
  if (!Number.isFinite(start)) return 0;
  const end = stats.endedAt ? new Date(stats.endedAt).getTime() : now;
  return Math.max(0, Math.floor((end - start) / 1000));
}

function formatElapsed(seconds: number) {
  if (seconds < 60) return `${seconds} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

// useTicker advances once per second so the live duration keeps moving even
// when the backend has nothing new to report.
function useTicker(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);
  return now;
}

// liveOutput reads the streaming values first and falls back to the values
// reported by a finished request.
function liveOutput(stats: AgentRunStats) {
  const tokens = (stats.liveTokens ?? 0) > 0 ? stats.liveTokens! : (stats.outputTokens ?? 0);
  const speed = (stats.liveSpeed ?? 0) > 0 ? stats.liveSpeed! : (stats.outputSpeed ?? 0);
  return {tokens, speed};
}

function runSummaryItems(stats: AgentRunStats) {
  const items = [{key: 'duration', label: `共思考 ${runDuration(stats)}`, Icon: Clock3, value: runDuration(stats)}];
  if (stats.firstTokenMs != null && stats.firstTokenMs > 0) {
    const value = `${(stats.firstTokenMs / 1000).toFixed(1)} 秒`;
    items.push({key: 'firstToken', label: `首 token 延迟 ${value}`, Icon: Zap, value});
  }
  if (stats.filesRead > 0) items.push({key: 'read', label: `读取 ${stats.filesRead} 个文件`, Icon: FileSearch, value: String(stats.filesRead)});
  if (stats.filesWritten > 0) items.push({key: 'write', label: `写入 ${stats.filesWritten} 个文件`, Icon: FilePenLine, value: String(stats.filesWritten)});
  if (stats.toolCalls > 0) items.push({key: 'tools', label: `调用 ${stats.toolCalls} 次工具`, Icon: Wrench, value: String(stats.toolCalls)});
  if ((stats.modelRequests ?? 0) > 0) items.push({key: 'requests', label: `模型请求 ${stats.modelRequests} 次`, Icon: SendHorizontal, value: String(stats.modelRequests)});
  const speed = runSpeed(stats);
  if (speed) items.push({key: 'speed', label: `输出速度 ${speed}`, Icon: Gauge, value: speed.replace(' tok/s', '')});
  return items;
}

function RunSummary({stats}: {stats: AgentRunStats}) {
  const anchor = useRef<HTMLDivElement>(null);
  const [tip, setTip] = useState('');
  const [tipPosition, setTipPosition] = useState({left: 0, top: 0});
  const phase = runPhase(stats);
  const items = runSummaryItems(stats);
  const showTip = (event: React.FocusEvent<HTMLSpanElement> | React.MouseEvent<HTMLSpanElement>, label: string) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const width = Math.min(250, Math.max(140, label.length * 12 + 22));
    setTip(label);
    setTipPosition({
      left: Math.max(8, Math.min(window.innerWidth - width - 8, rect.left + rect.width / 2 - width / 2)),
      top: rect.top >= 44 ? rect.top - 36 : rect.bottom + 8,
    });
  };
  const hideTip = () => setTip('');
  return <div ref={anchor} className={'agent-final-meta is-' + phase}>
    {(phase === 'failed' || phase === 'canceled') && <span className="agent-final-state" data-state={phase}>{phase === 'failed' ? <TriangleAlert aria-hidden="true"/> : <CircleStop aria-hidden="true"/>}<span>{phase === 'failed' ? '任务失败' : '任务已停止'}</span></span>}
    {items.map(({key, label, Icon, value}) => <span key={key} className="agent-final-stat" tabIndex={0} aria-label={label} onMouseEnter={event => showTip(event, label)} onMouseLeave={hideTip} onFocus={event => showTip(event, label)} onBlur={hideTip}><Icon aria-hidden="true"/><span>{value}</span></span>)}
    {tip && createPortal(<span className="agent-stat-tooltip" role="tooltip" style={tipPosition}>{tip}</span>, document.body)}
  </div>;
}

function liveRunState(runId: string, thoughts: AgentThought[], activity: AgentActivity[]): DropletState {
  const latestActivity = activity.filter(item => item.runId === runId).sort((a, b) => a.startedAt.localeCompare(b.startedAt)).at(-1);
  const latestThought = thoughts.filter(item => item.runId === runId).sort((a, b) => a.createdAt.localeCompare(b.createdAt)).at(-1);
  if (!latestActivity || (latestThought && latestThought.createdAt >= latestActivity.startedAt)) return 'thinking';
  if (latestActivity.status === 'failed') return 'error';
  return activityState(latestActivity.kind);
}

function formatCompactTokens(value: number) {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  if (value >= 1_000) return (value / 1_000).toFixed(1).replace(/\.0$/, '') + 'K';
  return String(value);
}

// 最终回复有两种渲染形状：MessageRow 的 .agent-message.role-assistant 与 RunTimeline 的 .agent-run-bubble，
// 两者的 data-message-id 分别落在 article 和气泡上，正文统一带 .agent-message-content。
function quoteMessageContent(node: Node): HTMLElement | null {
  const element = node instanceof Element ? node : node.parentElement;
  const content = element?.closest<HTMLElement>('.agent-message-content');
  const owner = content?.closest<HTMLElement>('[data-message-id]');
  if (!content || !owner?.dataset.messageId) return null;
  if (owner.classList.contains('role-user')) return null;
  return content;
}

const MessageRow = memo(function MessageRow({item, followsProcess, stats}: {
  item: AgentMessage;
  followsProcess: boolean;
  stats?: AgentRunStats;
}) {
  const completed = !!stats?.endedAt;
  return <article data-message-id={item.id} className={'agent-message role-' + item.role + (followsProcess ? ' follows-process' : '')}>
    {item.role === 'assistant' && !followsProcess && <div className="agent-message-avatar"><AgentDroplet state={stats?.phase === 'failed' ? 'error' : completed ? 'completed' : 'output'}/></div>}
    <div className="agent-message-body">
      {item.role === 'user' && !!item.references?.length && <div className="agent-message-references" aria-label="已引用对话">
        {item.references.map(reference => <button type="button" key={reference.id} className={'agent-message-reference' + (reference.sourceAvailable === false ? ' is-unavailable' : '')} onClick={() => window.dispatchEvent(new CustomEvent('xiaotao:agent-reference', {detail: reference}))} title={reference.sourceAvailable === false ? '来源对话已删除，摘要仍保留' : `引用对话：${reference.title}`}>
          <MessageSquarePlus aria-hidden="true"/><span>{reference.title}</span>
          {reference.hasNewContent && <small>有新内容</small>}
          {reference.sourceAvailable === false && <small>来源不可用</small>}
        </button>)}
      </div>}
      {item.role === 'assistant' && completed && <RunSummary stats={stats}/>}
      {!!item.images?.length && <div className="agent-message-images">
        {item.images.map(image => <a key={image.id} href={image.url} target="_blank" rel="noreferrer" title="打开图片"><img src={image.url} alt="消息图片" width={image.width} height={image.height}/></a>)}
      </div>}
      <div className="agent-message-content"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{item.content}</Markdown></div>
    </div>
  </article>;
}, (previous, next) => previous.followsProcess === next.followsProcess && previous.item.id === next.item.id && previous.item.role === next.item.role && previous.item.content === next.item.content && previous.stats === next.stats && JSON.stringify(previous.item.references) === JSON.stringify(next.item.references) && JSON.stringify(previous.item.images) === JSON.stringify(next.item.images));

// LiveStats shows the four numbers that matter while a run is streaming:
// elapsed time, first-token latency, produced tokens and output speed.
function LiveStats({stats}: {stats: AgentRunStats}) {
  const now = useTicker(!stats.endedAt);
  const elapsed = runElapsed(stats, now);
  const {tokens, speed} = liveOutput(stats);
  const firstToken = stats.firstTokenMs != null && stats.firstTokenMs > 0 ? `${(stats.firstTokenMs / 1000).toFixed(1)}s` : '--';
  return <div className="agent-live-stats" aria-live="polite" aria-label="实时执行统计">
    <span title="已执行时长"><Clock3 aria-hidden="true"/><span>{formatElapsed(elapsed)}</span></span>
    <span title="首 token 延迟"><Zap aria-hidden="true"/><span>首 token {firstToken}</span></span>
    <span title="累计输入 token 数（已返回用量）"><ArrowUp aria-hidden="true"/><span>{stats.inputKnown ? formatCompactTokens(stats.inputTokens ?? 0) : '--'} tok</span></span>
    <span title="累计输出 token 数"><ArrowDown aria-hidden="true"/><span>{formatCompactTokens(tokens)} tok</span></span>
    <span title="实时输出速度"><span className="agent-live-speed-val">{speed > 0 ? speed.toFixed(1).replace(/\.0$/, '') : '--'}</span><span className="agent-live-speed-unit">tok/s</span></span>
  </div>;
}

function RunTimeline({runId, thoughts, activity, streamDraft, answer, stats, busy, onOpen}: {
  runId: string; thoughts: AgentThought[]; activity: AgentActivity[];
  streamDraft?: AgentMessage; answer?: AgentMessage; stats: AgentRunStats; busy: boolean; onOpen: () => void;
}) {
  const rows = useMemo(() => [
    ...thoughts.filter(item => item.runId === runId).map(item => ({id: item.id, time: item.createdAt, kind: 'thought', label: thoughtKindLabel[item.kind], text: item.text})),
    ...activity.filter(item => item.runId === runId).map(item => ({id: item.id, time: item.startedAt, kind: 'activity', label: '操作', text: item.title + (item.detail ? ' · ' + item.detail : '')})),
  ].sort((a, b) => a.time.localeCompare(b.time)).slice(-4), [runId, thoughts, activity]);
  const current = thoughts.filter(item => item.runId === runId).at(-1);
  const finished = !!stats.endedAt;
  const text = answer?.content || streamDraft?.content || current?.text || '正在理解你的需求…';
  const content = useRef<HTMLDivElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState<number>();
  useLayoutEffect(() => {
    const element = content.current;
    if (!element) return;
    const measure = () => setHeight(finished ? element.scrollHeight : Math.min(element.scrollHeight, parseFloat(getComputedStyle(element).lineHeight) * 2));
    measure();
    const observer = new ResizeObserver(measure); observer.observe(element);
    return () => observer.disconnect();
  }, [finished, text]);
  useLayoutEffect(() => {
    if (viewport.current) viewport.current.scrollTop = finished ? 0 : viewport.current.scrollHeight;
  }, [text, finished, height]);
  const hasEvents = rows.length > 0;
  const hasAnswer = !!answer;
  return <div className={'agent-run-timeline' + (finished ? ' is-finished' : '')} data-run-id={runId}>
    <div className="agent-message-avatar"><AgentDroplet state={finished ? stats.phase === 'failed' ? 'error' : 'completed' : liveRunState(runId, thoughts, activity)} running={busy && !finished}/></div>
    {(!finished || hasAnswer) && <div className={'agent-run-bubble' + (!finished && streamDraft ? ' is-streaming' : '')} data-message-id={answer?.id}>
      <div ref={viewport} className={'agent-result-viewport' + (finished ? ' is-expanded' : '')} style={{height}}><div ref={content} className="agent-run-bubble-text agent-message-content"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{text}</Markdown></div></div>
    </div>}
    <div className={'agent-process-collapse' + (finished || !hasEvents ? ' is-collapsed' : '')} inert={finished || !hasEvents} aria-hidden={finished || !hasEvents}><div>
      <div className="agent-run-trace">
        <div className="agent-run-scroll" aria-label="最新四条思考与操作"><ol>{rows.map(row => <li key={row.id} data-kind={row.kind}><span>{row.label}</span><p>{row.text}</p></li>)}</ol></div>
        {!finished && <button type="button" className="agent-run-open" onClick={onOpen} aria-label="查看思考过程" title="查看思考过程"><PanelRight aria-hidden="true"/></button>}
      </div>
      {!finished && <LiveStats stats={stats}/>}
    </div></div>
    {finished && <div className="agent-final-meta-wrap"><RunSummary stats={stats}/><button type="button" className="agent-run-open agent-final-open" onClick={onOpen} aria-label="查看思考过程" title="查看思考过程"><PanelRight aria-hidden="true"/></button></div>}
  </div>;
}

function ThoughtTraceEntry({row, startedAt, index, expanded, onToggle}: {
  row: ThoughtTraceRow; startedAt: string; index: number; expanded: boolean; onToggle: () => void;
}) {
  const Icon = thoughtTraceIcon(row);
  const summary = traceSummary(row);
  const detail = row.kind === 'activity'
    ? [row.detail, row.tool && `工具：${row.tool}`, row.input && `参数\n${row.input}`, row.output && `结果\n${row.output}`].filter(Boolean).join('\n\n')
    : row.text;
  const duration = row.kind === 'activity' && row.endedAt
    ? Math.max(0, Math.round((new Date(row.endedAt).getTime() - new Date(row.startedAt).getTime()) / 1000))
    : null;
  return <li className={'agent-thought-event' + (expanded ? ' is-expanded' : '')} data-kind={row.kind} data-status={row.status}>
    <span className="agent-thought-status" role="status">{traceStatusLabel(row.status)}</span>
    <span className="agent-thought-rail" aria-hidden="true"><i/></span>
    <button type="button" className="agent-thought-event-toggle" aria-expanded={expanded} aria-controls={`thought-event-${row.id}`} onClick={onToggle}>
      <span className="agent-thought-event-index">{String(index + 1).padStart(2, '0')}</span>
      <span className="agent-thought-event-icon"><Icon aria-hidden="true"/></span>
      <span className="agent-thought-event-copy">
        <span className="agent-thought-event-meta"><strong>{row.kind === 'thought' ? thoughtKindLabel[row.thoughtKind] : row.title}</strong><time dateTime={row.time}>{traceOffset(startedAt, row.time)}</time></span>
        <span className="agent-thought-event-text">{summary}</span>
      </span>
      <ChevronRight className="agent-thought-event-chevron" aria-hidden="true"/>
    </button>
    <div className="agent-thought-event-detail" id={`thought-event-${row.id}`} aria-hidden={!expanded} inert={!expanded}>
      <div className="agent-thought-event-detail-inner">
        <pre>{detail || summary || '暂无更多详情'}</pre>
        {row.kind === 'activity' && (row.tool || duration !== null) && <span className="agent-thought-event-detail-meta">
          {row.tool && <span>工具 {row.tool}</span>}
          {duration !== null && <span>耗时 {duration} 秒</span>}
        </span>}
      </div>
    </div>
  </li>;
}

function thoughtTraceRows(runId: string, thoughts: AgentThought[], activity: AgentActivity[]): ThoughtTraceRow[] {
  return [
    ...(thoughts ?? []).filter(item => item.runId === runId).map(item => ({
      id: item.id, time: item.createdAt,
      kind: 'thought' as const, thoughtKind: item.kind, text: item.text, status: item.status,
    })),
    ...(activity ?? []).filter(item => item.runId === runId).map(item => ({
      id: item.id, time: item.startedAt, kind: 'activity' as const,
      activityKind: item.kind, title: item.title, detail: item.detail, tool: item.tool, input: item.input, output: item.output,
      startedAt: item.startedAt, endedAt: item.endedAt, text: item.title + (item.detail ? ' · ' + item.detail : ''), status: item.status,
    })),
  ].sort((a, b) => a.time.localeCompare(b.time));
}

function ThoughtDrawer({runId, thoughts, activity, stats, open, onClose}: {
  runId: string; thoughts: AgentThought[]; activity: AgentActivity[]; stats: AgentRunStats; open: boolean; onClose: () => void;
}) {
  const close = useRef<HTMLButtonElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const [present, setPresent] = useState(open);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    if (open) {
      returnFocus.current = document.activeElement as HTMLElement | null;
      setPresent(true);
      return;
    }
    const timer = window.setTimeout(() => setPresent(false), 260);
    return () => window.clearTimeout(timer);
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const previous = returnFocus.current;
    close.current?.focus();
    const escape = (event: KeyboardEvent) => {if (event.key === 'Escape') {event.preventDefault(); onClose();}};
    document.addEventListener('keydown', escape);
    return () => {document.removeEventListener('keydown', escape); previous?.focus();};
  }, [open, onClose]);
  useEffect(() => {
    if (open) setExpanded(new Set());
  }, [open, runId]);
  const timeline = useMemo(() => thoughtTraceRows(runId, thoughts, activity), [runId, thoughts, activity]);
  const activities = timeline.filter(row => row.kind === 'activity');
  const thoughtCount = timeline.length - activities.length;
  const warnings = activities.filter(row => row.status === 'failed').length;
  const firstThought = timeline.find(row => row.kind === 'thought');
  const lastDecision = [...timeline].reverse().find(row => row.kind === 'thought' && row.thoughtKind === 'decision');
  const hasThought = timeline.some(row => row.kind === 'thought');
  if (!present) return null;
  return createPortal(<div className={'agent-drawer-backdrop' + (open ? ' is-open' : ' is-closing')} onPointerDown={event => {if (event.target === event.currentTarget) onClose();}} inert={!open}>
    <aside className="agent-thought-drawer agent-thought-map" role="dialog" aria-modal="true" aria-label="本轮思考过程">
      <header className="agent-thought-header">
        <div><span>思考路线</span><strong><Route aria-hidden="true"/>{runDuration(stats)}</strong></div>
        <button ref={close} type="button" onClick={onClose} aria-label="关闭思考过程" title="关闭"><X aria-hidden="true"/></button>
      </header>
      <div className="agent-thought-overview">
        <div className="agent-thought-overview-head">
          <div><span>整体判断</span><h2>{firstThought?.text || '这轮还没有留下可读的思考节点。'}</h2></div>
          <span className="agent-thought-count"><strong>{timeline.length}</strong> 个节点</span>
        </div>
        {!hasThought && <p className="agent-thought-caveat">本轮模型没有输出过程说明，下面只展示实际工具操作。</p>}
        <div className="agent-thought-narrative">
          <p><span>最终落点</span>{lastDecision?.text || (stats.filesWritten > 0 ? `本轮完成修改，涉及 ${stats.filesWritten} 个文件。` : activities.length ? `本轮以 ${activities.length} 次操作为主，主要是在收集和执行信息。` : '本轮以模型思考为主，没有额外工具操作。')}</p>
          <div className="agent-thought-facts">
            <span><FileSearch aria-hidden="true"/>{stats.filesRead} 个文件读取</span>
            <span><FilePenLine aria-hidden="true"/>{stats.filesWritten} 个文件写入</span>
            <span><Wrench aria-hidden="true"/>{stats.toolCalls} 次操作</span>
            {warnings > 0 && <span className="is-warning"><CircleAlert aria-hidden="true"/>{warnings} 次失败</span>}
          </div>
        </div>
      </div>
      <div className="agent-thought-timeline-head">
        <div><span>思考链</span><small>点击任意节点展开细节</small></div>
        <span>{thoughtCount} 思考 · {activities.length} 操作</span>
      </div>
      {timeline.length ? <ol className="agent-thought-timeline">
        {timeline.map((row, index) => <ThoughtTraceEntry key={row.id} row={row} startedAt={stats.startedAt} index={index} expanded={expanded.has(row.id)} onToggle={() => setExpanded(current => {
          const next = new Set(current);
          if (next.has(row.id)) next.delete(row.id); else next.add(row.id);
          return next;
        })}/>)}
      </ol> : <div className="agent-thought-empty"><Route aria-hidden="true"/><strong>没有可展开的过程</strong><p>这轮只有最终回答，没有留下思考节点或工具调用。</p></div>}
    </aside>
  </div>, document.body);
}

function inlineMarkdown(node: JSONContent): string {
  if (node.type === conversationReferenceType) return '@' + String(node.attrs?.title || '');
  if (node.type === 'text') {
    let text = node.text ?? '';
    for (const mark of node.marks ?? []) {
      if (mark.type === 'code') text = '`' + text + '`';
      else if (mark.type === 'bold') text = '**' + text + '**';
      else if (mark.type === 'italic') text = '*' + text + '*';
      else if (mark.type === 'strike') text = '~~' + text + '~~';
      else if (mark.type === 'underline') text = '<u>' + text + '</u>';
      else if (mark.type === 'link') text = '[' + text + '](' + (mark.attrs?.href ?? '') + ')';
    }
    return text;
  }
  if (node.type === 'hardBreak') return '  \n';
  const content = (node.content ?? []).map(inlineMarkdown).join('');
  return content;
}

function blocksMarkdown(nodes: JSONContent[], indent = 0): string {
  return nodes.map(node => blockMarkdown(node, indent)).filter(Boolean).join('\n\n');
}

function listMarkdown(node: JSONContent, indent: number): string {
  const ordered = node.type === 'orderedList';
  const start = Number(node.attrs?.start) || 1;
  return (node.content ?? []).map((item, index) => {
    const children = item.content ?? [];
    const first = children[0];
    const prefix = ' '.repeat(indent) + (ordered ? (start + index) + '. ' : '- ');
    const head = first ? inlineMarkdown(first).replace(/\n/g, '\n' + ' '.repeat(prefix.length)) : '';
    const rest = children.slice(1);
    const nested = rest.length ? '\n' + blocksMarkdown(rest, indent + 2).replace(/\n\n/g, '\n') : '';
    return prefix + head + nested;
  }).join('\n');
}

function blockMarkdown(node: JSONContent, indent: number): string {
  if (node.type === 'paragraph') return ' '.repeat(indent) + inlineMarkdown(node);
  if (node.type === 'heading') {
    const level = Math.min(6, Math.max(1, Number(node.attrs?.level) || 1));
    return ' '.repeat(indent) + '#'.repeat(level) + ' ' + inlineMarkdown(node);
  }
  if (node.type === 'bulletList' || node.type === 'orderedList') return listMarkdown(node, indent);
  if (node.type === 'blockquote') {
    return (node.content ?? []).map(child => blockMarkdown(child, 0).split('\n').map(line => ' '.repeat(indent) + '> ' + line).join('\n')).join('\n\n');
  }
  if (node.type === 'codeBlock') {
    const language = String(node.attrs?.language || '');
    return ' '.repeat(indent) + '```' + language + '\n' + (node.content ?? []).map(child => child.text ?? '').join('') + '\n' + ' '.repeat(indent) + '```';
  }
  if (node.type === 'horizontalRule') return ' '.repeat(indent) + '---';
  return ' '.repeat(indent) + inlineMarkdown(node);
}

function editorMarkdown(editor: Editor): string {
  return blocksMarkdown(editor.getJSON().content ?? []).trim();
}

function editorReferenceIds(editor: Editor): string[] {
  const ids: string[] = [];
  const visit = (node: JSONContent) => {
    if (node.type === conversationReferenceType && typeof node.attrs?.sessionId === 'string') ids.push(node.attrs.sessionId);
    for (const child of node.content ?? []) visit(child);
  };
  visit(editor.getJSON());
  return Array.from(new Set(ids.filter(Boolean)));
}

type ComposerImage = {
  id: string;
  data: string;
  mimeType: string;
  name: string;
  preview: string;
  size: number;
  width?: number;
  height?: number;
};

const composerImageTypes = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp']);

async function readImageFile(file: File): Promise<ComposerImage> {
  if (!composerImageTypes.has(file.type)) {
    throw new Error('仅支持 PNG、JPEG、GIF 和 WebP 图片');
  }
  if (file.size > 5 * 1024 * 1024) {
    throw new Error('单张图片不能超过 5 MiB');
  }
  // Scale and re-encode before upload: vision models price images by pixel
  // area, and a large screenshot costs far more to send than it does to read.
  const normalised = await normaliseImage(file);
  if (!normalised.width || !normalised.height || normalised.width * normalised.height > 40_000_000) {
    throw new Error('图片尺寸无效或过大');
  }
  return {
    id: crypto.randomUUID(),
    data: normalised.data,
    mimeType: normalised.mimeType,
    name: file.name || '粘贴的图片',
    preview: normalised.preview,
    size: normalised.size,
    width: normalised.width,
    height: normalised.height,
  };
}

function pastedImageFiles(data: DataTransfer | null): File[] {
  if (!data) return [];
  const files = Array.from(data.files).filter(file => file.type.startsWith('image/'));
  if (files.length) return files;
  return Array.from(data.items)
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile())
    .filter((file): file is File => !!file);
}

function referenceQueryAtCursor(state: EditorState): {from: number; query: string} | null {
  if (hasActiveNode(state, ['codeBlock'])) return null;
  const {from} = state.selection;
  const { $from } = state.selection;
  const paragraphStart = $from.start();
  const textBefore = state.doc.textBetween(paragraphStart, from, '\n', '\ufffc');
  const match = /(?:^|\s)@([^\s@]*)$/.exec(textBefore);
  if (!match) return null;
  return {from: from - match[1].length - 1, query: match[1]};
}

function hasActiveNode(state: EditorState, names: string[]): boolean {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if (names.includes($from.node(depth).type.name)) return true;
  }
  return false;
}

function isEmptyListItem(state: EditorState): boolean {
  const { $from, $to } = state.selection;
  return $from.parentOffset === 0 && $to.parentOffset === 0 &&
    $from.parent.content.size === 0 && hasActiveNode(state, ['listItem']);
}

function exitEmptyListItem(editor: Editor): boolean {
  const { state } = editor;
  const { $from } = state.selection;
  let listDepth = -1;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if ($from.node(depth).type.name === 'listItem') {
      listDepth = depth;
      break;
    }
  }
  if (listDepth < 0) return false;

  const list = $from.node(listDepth - 1);
  const listStart = $from.before(listDepth - 1);
  const itemIndex = $from.index(listDepth - 1);
  const item = list.child(itemIndex);
  if (itemIndex !== list.childCount - 1 || !item.firstChild?.isTextblock || item.firstChild.content.size) return false;

  const paragraph = state.schema.nodes.paragraph.create();
  const transaction = state.tr;
  if (list.childCount === 1) {
    transaction.replaceWith(listStart, listStart + list.nodeSize, paragraph);
  } else {
    transaction.delete($from.before(listDepth), $from.after(listDepth));
    const afterList = listStart + list.nodeSize - item.nodeSize - 1;
    transaction.insert(afterList, paragraph);
  }
  const after = listStart + list.nodeSize - item.nodeSize + 1;
  transaction.setSelection(TextSelection.near(transaction.doc.resolve(Math.min(after, transaction.doc.content.size))));
  editor.view.dispatch(transaction.scrollIntoView());
  return true;
}

export function AgentPage({visible}: {visible: boolean}) {
  const confirm = useConfirm();
  const [accessMode, setAccessMode] = useState<'safe' | 'full'>('safe');
  const [accessReady, setAccessReady] = useState(false);
  const [modeMenuOpen, setModeMenuOpen] = useState(false);
  const modeButtonRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const refresh = () => { void fetch('/api/agent/access-mode').then(response => {if (!response.ok) throw new Error('读取访问模式失败'); return response.json();}).then(value => {setAccessMode(value.mode === 'full' ? 'full' : 'safe'); setAccessReady(true);}).catch(() => setAccessReady(false)); };
    refresh(); window.addEventListener('focus', refresh); window.addEventListener('storage', refresh);
    return () => {window.removeEventListener('focus', refresh); window.removeEventListener('storage', refresh);};
  }, []);
  async function changeAccessMode(mode: 'safe' | 'full') {
    setModeMenuOpen(false);
    if (mode === accessMode) return;
    if (mode === 'full' && !await confirm({title: '切换到完全访问？', description: '小涛将以当前 Windows 用户权限访问和修改文件，并自主执行命令。该模式对所有会话生效。', confirmLabel: '切换'})) return;
    try {
      const response = await fetch('/api/agent/access-mode', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({mode})});
      if (!response.ok) throw new Error('切换访问模式失败');
      setAccessMode(mode);
      try { localStorage.setItem('xiaotao.agent.accessModeChanged', String(Date.now())); } catch { /* Other windows refresh on focus. */ }
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }

  const [sessions, setSessions] = useState<AgentSession[]>([]), [active, setActive] = useState<AgentSession | null>(null);
  const [view, setView] = useState<'project' | 'session'>(() => localStorage.getItem('xiaotao.agent.historyView') === 'session' ? 'session' : 'project');
  const [viewMenuOpen, setViewMenuOpen] = useState(false);
  const viewButtonRef = useRef<HTMLButtonElement>(null);
  const [deleting, setDeleting] = useState('');
  const [exitingSession, setExitingSession] = useState('');
  const [sessionMenu, setSessionMenu] = useState<{item: AgentSession; x: number; y: number} | null>(null);
  const [renaming, setRenaming] = useState<AgentSession | null>(null);
  const sessionMenuTrigger = useRef<HTMLElement | null>(null);
  const {enabledModels, modelConfigs, defaultModels, defaultModelSettings, persist: persistModelSettings} = useAgentModelSettings();
  const [models, setModels] = useState<AgentModelOption[]>(() => { const labels = readModelLabels(); return Array.from(new Map([...readEnabledModels().map(id => ({model: id, label: labels[id] || id}))].map(item => [item.model, item])).values()); });
  const [modelsLoading, setModelsLoading] = useState(false), [modelsError, setModelsError] = useState('');
  const [modelsAttempt, setModelsAttempt] = useState(0);
  const refreshModelsRequested = useRef(false);
  const [model, setModel] = useState(() => readEnabledModels()[0] || ''), [path, setPath] = useState(''), [projectName, setProjectName] = useState('');
  const [modelMenuOpen, setModelMenuOpen] = useState(false);
  const modelButtonRef = useRef<HTMLButtonElement>(null);
  const [prompt, setPrompt] = useState(''), [pending, setPending] = useState(false), [loading, setLoading] = useState(true);
  const [composerImages, setComposerImages] = useState<ComposerImage[]>([]);
  const composerImagesRef = useRef<ComposerImage[]>([]);
  composerImagesRef.current = composerImages;
  const [runningIds, setRunningIds] = useState<Set<string>>(new Set());
  const runningRef = useRef(new Set<string>());
  const sessionCache = useRef(new Map<string, AgentSession>());
  const busy = pending || !!active && (runningIds.has(active.id) || active.status === 'running');
  const drafts = useRef(new Map<string, {json: object; model: string; path: string; name: string; top: number; follow: boolean; quotes: typeof quotes; images: ComposerImage[]}>());
  const [quotes, setQuotes] = useState<Array<{id: string; messageId: string; start: number; end: number; sourceText: string; text: string; reply: string}>>([]);
  const [quoteSelection, setQuoteSelection] = useState<{text: string; messageId: string; start: number; end: number; left: number; top: number} | null>(null);
  const [editingQuote, setEditingQuote] = useState<string | null>(null);
  const [quoteMarks, setQuoteMarks] = useState<Array<{id: string; rects: Array<{left: number; top: number; width: number; height: number}>; left: number; top: number}>>([]);
  useEffect(() => {if (!visible || busy) setSessionMenu(null);}, [visible, busy]);
  useEffect(() => { if (busy) setModelMenuOpen(false); }, [busy]);
  const openSessionMenu = useCallback(function openSessionMenu(item: AgentSession, x: number, y: number, trigger: HTMLElement) {
    if (busy || deleting) return;
    sessionMenuTrigger.current = trigger;
    setSessionMenu({item, x: Math.max(8, Math.min(x, window.innerWidth - 168)), y: Math.max(8, Math.min(y, window.innerHeight - 92))});
  }, [busy, deleting]);
  const [error, setError] = useState('');
  const [openThoughtRun, setOpenThoughtRun] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false), [projectOpen, setProjectOpen] = useState(false);
  const [reviewListOpen, setReviewListOpen] = useState(false);
  const [reviewDetailOpen, setReviewDetailOpen] = useState<BehaviorReview | null>(null);
  const [reviewingSessionId, setReviewingSessionId] = useState<string | null>(null);
  const [reviewToast, setReviewToast] = useState<string | null>(null);

  useEffect(() => {
    if (!reviewToast) return;
    const timer = window.setTimeout(() => setReviewToast(null), 3000);
    return () => window.clearTimeout(timer);
  }, [reviewToast]);

  const hasCompletedRuns = useMemo(() => {
    if (!active) return false;
    if (active.runStats && active.runStats.some(r => !!r.endedAt)) return true;
    if (active.activity && active.activity.some(a => !!a.endedAt)) return true;
    if (active.status !== 'running' && active.messages && active.messages.some(m => m.role === 'assistant')) return true;
    return false;
  }, [active]);

  const canStartReview = !!active && !busy && hasCompletedRuns && reviewingSessionId !== active.id;

  const handleStartReview = useCallback(async () => {
    if (!active) return;
    if (busy) {
      setError('任务结束后可复盘');
      return;
    }
    if (!hasCompletedRuns) {
      setError('会话中暂无已完成的 Agent 执行记录，无法进行复盘');
      return;
    }
    if (reviewingSessionId === active.id) return;

    const targetSessionId = active.id;
    setReviewingSessionId(targetSessionId);
    setError('');

    try {
      const utility = utilityModelFor('review', defaultModels, defaultModelSettings, modelConfigs, models);
      const review = await api.createReview(targetSessionId, undefined, utility.model, utility.settings);
      const pollTimer = window.setInterval(async () => {
        try {
          const detail = await api.getReview(review.id);
          if (detail.status === 'succeeded') {
            window.clearInterval(pollTimer);
            setReviewingSessionId(curr => (curr === targetSessionId ? null : curr));
            if (activeIdRef.current === targetSessionId) {
              setReviewDetailOpen(detail);
            } else {
              setReviewToast('复盘已完成，可在复盘记录中查看');
            }
          } else if (detail.status === 'failed' || detail.status === 'canceled') {
            window.clearInterval(pollTimer);
            setReviewingSessionId(curr => (curr === targetSessionId ? null : curr));
            if (detail.status === 'failed') {
              setError(`复盘失败: ${detail.errorMessage || '未知错误'}`);
            }
          }
        } catch {
          // ignore transient poll error
        }
      }, 2000);
    } catch (err: unknown) {
      setReviewingSessionId(curr => (curr === targetSessionId ? null : curr));
      setError(err instanceof Error ? err.message : '发起复盘失败');
    }
  }, [active, busy, hasCompletedRuns, reviewingSessionId, defaultModels, defaultModelSettings, modelConfigs, models]);
  const [projectMenuOpen, setProjectMenuOpen] = useState(false), [projectQuery, setProjectQuery] = useState('');
  const projectTriggerRef = useRef<HTMLButtonElement>(null), projectSearchRef = useRef<HTMLInputElement>(null);
  useEffect(() => { if (!visible || busy || active) setProjectMenuOpen(false); }, [visible, busy, active]);
  const feed = useRef<HTMLDivElement>(null), composer = useRef<HTMLDivElement>(null), sendRef = useRef<() => void>(() => {});
  const followFeed = useRef(true);
  const captureQuoteSelection = () => {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || !selection.rangeCount) { setQuoteSelection(null); return; }
    const range = selection.getRangeAt(0);
    const start = quoteMessageContent(range.startContainer);
    const end = quoteMessageContent(range.endContainer);
    const raw = selection.toString();
    const text = raw.trim();
    const messageId = start?.closest<HTMLElement>('[data-message-id]')?.dataset.messageId;
    if (!text || !start || start !== end || !messageId || !feed.current?.contains(start)) { setQuoteSelection(null); return; }
    const before = range.cloneRange();
    before.selectNodeContents(start);
    before.setEnd(range.startContainer, range.startOffset);
    const offset = before.toString().length + (raw.length - raw.trimStart().length);
    const rect = range.getBoundingClientRect();
    setQuoteSelection({text, messageId, start: offset, end: offset + text.length, left: Math.max(8, Math.min(window.innerWidth - 80, rect.left + rect.width / 2 - 32)), top: Math.max(8, rect.top - 36)});
  };
  useEffect(() => {
    if (!quoteSelection) return;
    const dismiss = (event: PointerEvent) => { if (!(event.target instanceof Element) || !event.target.closest('.agent-quote-selection')) setQuoteSelection(null); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setQuoteSelection(null); };
    const hide = () => setQuoteSelection(null);
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', escape);
    document.addEventListener('scroll', hide, true);
    window.addEventListener('resize', hide);
    return () => { document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', escape); document.removeEventListener('scroll', hide, true); window.removeEventListener('resize', hide); };
  }, [quoteSelection]);

  useEffect(() => {
    if (!editingQuote) return;
    const escape = (event: KeyboardEvent) => {if (event.key === 'Escape') {event.preventDefault(); setEditingQuote(null);}};
    document.addEventListener('keydown', escape);
    return () => document.removeEventListener('keydown', escape);
  }, [editingQuote]);
  useEffect(() => {
    let frame = 0;
    const update = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const host = feed.current;
        if (!host || !visible) { setQuoteMarks([]); return; }
        const bounds = host.getBoundingClientRect();
        const marks: typeof quoteMarks = [];
        for (const quote of quotes) {
          const owner = host.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(quote.messageId)}"]`);
          const content = owner && !owner.classList.contains('role-user') ? owner.querySelector<HTMLElement>('.agent-message-content') : null;
          if (!content) continue;
          const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT);
          let node: Node | null, position = 0, first: {node: Node; offset: number} | undefined, last: {node: Node; offset: number} | undefined;
          while ((node = walker.nextNode())) {
            const length = node.textContent?.length ?? 0;
            if (!first && quote.start >= position && quote.start < position + length) first = {node, offset: quote.start - position};
            if (quote.end > position && quote.end <= position + length) { last = {node, offset: quote.end - position}; break; }
            position += length;
          }
          if (!first || !last) continue;
          const range = document.createRange(); range.setStart(first.node, first.offset); range.setEnd(last.node, last.offset);
          if (range.toString() !== quote.sourceText) continue;
          const rects = Array.from(range.getClientRects()).map(rect => ({left: Math.max(rect.left, bounds.left), top: Math.max(rect.top, bounds.top), width: Math.min(rect.right, bounds.right) - Math.max(rect.left, bounds.left), height: Math.min(rect.bottom, bounds.bottom) - Math.max(rect.top, bounds.top)})).filter(rect => rect.width > 0 && rect.height > 0);
          if (!rects.length) continue;
          const lastRect = rects[rects.length - 1];
          marks.push({id: quote.id, rects, left: Math.min(bounds.right - 20, lastRect.left + lastRect.width), top: Math.max(bounds.top, lastRect.top - 8)});
        }
        setQuoteMarks(marks);
      });
    };
    update(); document.addEventListener('scroll', update, true); window.addEventListener('resize', update);
    return () => {cancelAnimationFrame(frame); document.removeEventListener('scroll', update, true); window.removeEventListener('resize', update);};
  }, [quotes, active?.messages, visible]);

  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const updateScrollToBottom = useCallback(() => {
    const el = feed.current;
    setShowScrollToBottom(!!el && el.scrollHeight - el.scrollTop - el.clientHeight >= 80);
  }, []);
  function scrollToBottom() {
    const el = feed.current;
    if (!el) return;
    el.scrollTo({top: el.scrollHeight, behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth'});
  }
  const [historyBefore, setHistoryBefore] = useState('');
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const historyRequest = useRef(0);
  const historyInFlight = useRef(false);
  const activeSessionRef = useRef(active);
  activeSessionRef.current = active;
  const activeIdRef = useRef(active?.id);
  activeIdRef.current = active?.id;
  const prependAnchor = useRef<{id: string; height: number; top: number} | null>(null);
  useLayoutEffect(() => {
    const anchor = prependAnchor.current; const el = feed.current;
    if (!anchor || !el || anchor.id !== active?.id) return;
    el.scrollTop = anchor.top + el.scrollHeight - anchor.height;
    prependAnchor.current = null;
  }, [active?.id, active?.messages, active?.activity, historyLoading, historyError]);
  function preserveHistoryScroll(id: string) {
    const el = feed.current;
    if (el) prependAnchor.current = {id, height:el.scrollHeight, top:el.scrollTop};
  }
  async function loadOlder() {
    if (!active || !historyBefore || historyInFlight.current || busy) return;
    const id = active.id; const version = historyRequest.current;
    preserveHistoryScroll(id);
    followFeed.current = false;
    historyInFlight.current = true; setHistoryLoading(true); setHistoryError('');
    try {
      const page = await api.agentHistory(id, historyBefore);
      if (version !== historyRequest.current || activeIdRef.current !== id) return;
      preserveHistoryScroll(id);
      followFeed.current = false;
      setActive(current => current?.id === id ? {...current, messages:mergeHistory(page.session.messages, current.messages), activity:mergeHistory(page.session.activity, current.activity), thoughts:mergeHistory(page.session.thoughts, current.thoughts), runStats:mergeRunStats(page.session.runStats, current.runStats)} : current);
      setHistoryBefore(page.before);
    } catch (reason) {
      if (version === historyRequest.current) { preserveHistoryScroll(id); setHistoryError(reason instanceof Error ? reason.message : String(reason)); }
    } finally {
      if (version === historyRequest.current) { historyInFlight.current = false; setHistoryLoading(false); }
    }
  }
  const resetHistory = useCallback(() => {
    historyRequest.current++; historyInFlight.current = false;
    setHistoryBefore(''); setHistoryLoading(false); setHistoryError(''); prependAnchor.current = null;
  }, []);

  useEffect(() => {
    const element = composer.current;
    const scrollArea = feed.current;
    if (!element || !scrollArea) return;
    const observer = new ResizeObserver(() => {
      scrollArea.style.setProperty('--composer-height', `${element.offsetHeight}px`);
      if (followFeed.current) scrollArea.scrollTop = scrollArea.scrollHeight;
      updateScrollToBottom();
    });
    observer.observe(element);
    observer.observe(scrollArea);
    return () => observer.disconnect();
  }, [updateScrollToBottom]);
  const promptRef = useRef(''), canSendRef = useRef(false);
  const referenceQueryRef = useRef<{from: number; query: string} | null>(null);
  const referenceOpenRef = useRef(false);
  const closeReferencePickerRef = useRef<() => void>(() => {});
  const [referencePicker, setReferencePicker] = useState<{open: boolean; query: string; scope: 'project' | 'all'}>({open: false, query: '', scope: 'project'});
  const [referenceResults, setReferenceResults] = useState<AgentReferenceCandidate[]>([]);
  const [referenceIndex, setReferenceIndex] = useState(0);
  const [referenceLoading, setReferenceLoading] = useState(false);
  const [referenceError, setReferenceError] = useState('');
  const referenceRequest = useRef(0);
  const referenceComposerRef = useRef<HTMLDivElement>(null);
  const [referenceDetail, setReferenceDetail] = useState<AgentConversationReference | null>(null);
  const addComposerImages = useCallback((files: File[]) => {
    if (!files.length) return;
    const existing = composerImagesRef.current;
    const available = 4 - existing.length;
    if (available <= 0) { setError('每条消息最多添加 4 张图片'); return; }
    void Promise.all(files.slice(0, available).map(readImageFile))
      .then(images => {
        const current = composerImagesRef.current;
        const ids = new Set(current.map(image => image.id));
        const unique = images.filter(image => !ids.has(image.id));
        const next = [...current, ...unique].slice(0, 4);
        if (next.reduce((total, image) => total + image.size, 0) > 12 * 1024 * 1024) {
          setError('图片合计不能超过 12 MiB');
          return;
        }
        setComposerImages(next);
        setError('');
      })
      .catch(reason => setError(reason instanceof Error ? reason.message : String(reason)));
    if (files.length > available) setError('每条消息最多添加 4 张图片');
  }, []);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({trailingNode: false}),
      Placeholder.configure({placeholder: '描述你想完成的事…'}),
      ConversationReference,
    ],
    editorProps: {
      attributes: {'aria-label': '给小涛发消息', role: 'textbox', 'aria-multiline': 'true'},
      handlePaste: (view, event) => {
        const imageFiles = pastedImageFiles(event.clipboardData);
        if (imageFiles.length) {
          event.preventDefault();
          addComposerImages(imageFiles);
          return true;
        }
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        const text = event.clipboardData?.getData('text/plain').replace(/\r\n?/g, '\n');
        const match = text?.match(/^\s*(`{3,}|~{3,})(?:markdown|md)[ \t]*\n([\s\S]*?)\n\1[ \t]*\s*$/i);
        if (text && !match && !text.includes('\n') && /\\/.test(text) && !/^[a-z][a-z\d+.-]*:\/\//i.test(text)) {
          view.dispatch(view.state.tr.insertText(text).setMeta('preventAutolink', true).scrollIntoView());
          event.preventDefault();
          return true;
        }
        const html = event.clipboardData?.getData('text/html') ?? '';
        const source = new DOMParser().parseFromString(html, 'text/html');
        const codeContainer = source.body.querySelector('pre') || Array.from(source.body.querySelectorAll<HTMLElement>('[style]')).find(element => element.style.whiteSpace === 'pre');
        if (!match && text && codeContainer) {
          const nodes = text.split('\n').flatMap((line, index) => [
            ...(index ? [view.state.schema.nodes.hardBreak.create()] : []),
            ...(line ? [view.state.schema.text(line)] : []),
          ]);
          view.dispatch(view.state.tr.replaceSelection(new Slice(Fragment.fromArray(nodes), 0, 0)).scrollIntoView());
          event.preventDefault();
          return true;
        }
        if (!match) return false;
        const container = document.createElement('div');
        container.innerHTML = renderToStaticMarkup(<Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{match[2]}</Markdown>);
        const slice = ProseMirrorDOMParser.fromSchema(view.state.schema).parseSlice(container);
        view.dispatch(view.state.tr.replaceSelection(slice).scrollIntoView());
        event.preventDefault();
        return true;
      },
      handleTextInput: (view, from, to, text) => {
        if (text !== ' ' || from !== to || hasActiveNode(view.state, ['codeBlock'])) return false;
        const { $from } = view.state.selection;
        const before = $from.parent.textBetween(0, $from.parentOffset, '\n', '\ufffc');
        const match = /(^|[^`])`([^`\n]+)`$/.exec(before);
        if (!match || !view.state.schema.marks.code) return false;
        const start = from - match[2].length - 2;
        let formatted = false;
        view.state.doc.nodesBetween(start, from, node => { if (node.isText && node.marks.length) formatted = true; });
        if (formatted) return false;
        const code = view.state.schema.text(match[2], [view.state.schema.marks.code.create()]);
        const space = view.state.schema.text(' ');
        view.dispatch(view.state.tr.replaceWith(start, to, [code, space]).scrollIntoView());
        return true;
      },
      handleKeyDown: (view, event) => {
        if (referenceOpenRef.current && !event.isComposing) {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            setReferenceIndex(index => referenceResults.length ? (index + (event.key === 'ArrowDown' ? 1 : -1) + referenceResults.length) % referenceResults.length : 0);
            return true;
          }
          if (event.key === 'Enter' && referenceResults.length) {
            event.preventDefault();
            chooseReference(referenceResults[referenceIndex]);
            return true;
          }
          if (event.key === 'Enter') {
            event.preventDefault();
            return true;
          }
          if (event.key === 'Escape') {
            event.preventDefault();
            closeReferencePicker();
            return true;
          }
        }
        if (event.key !== 'Enter' || event.isComposing) return false;
        if (event.ctrlKey || event.metaKey) {
          if (!canSendRef.current) return false;
          event.preventDefault();
          sendRef.current();
          return true;
        }
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        if (hasActiveNode(view.state, ['listItem'])) {
          if (isEmptyListItem(view.state)) {
            if (editor && exitEmptyListItem(editor)) { event.preventDefault(); return true; }
          } else if (editor?.commands.splitListItem('listItem')) {
            event.preventDefault();
            return true;
          }
          return false;
        }
        if (event.shiftKey || !canSendRef.current) return false;

        if (hasActiveNode(view.state, ['heading', 'blockquote'])) return false;

        event.preventDefault();
        sendRef.current();
        return true;
      },
    },
    onUpdate: ({editor}) => {
      const next = editorMarkdown(editor);
      promptRef.current = next;
      setPrompt(next);
      if (editor.state.selection instanceof NodeSelection && editor.state.selection.node.type.name === conversationReferenceType) {
        closeReferencePickerRef.current();
        return;
      }
      const query = referenceQueryAtCursor(editor.state);
      if (!query) {
        closeReferencePickerRef.current();
        return;
      }
      referenceOpenRef.current = true;
      setReferencePicker(current => ({...current, open: true, query: query.query}));
      referenceQueryRef.current = query;
      setReferenceIndex(0);
    },
  });

  useEffect(() => {
    if (!referencePicker.open) return;
    const request = ++referenceRequest.current;
    setReferenceLoading(true);
    setReferenceError('');
    api.agentReferenceSessions(referencePicker.query, referencePicker.scope, active?.project.path || path, Array.from(new Set([active?.id, ...editorReferenceIds(editor!)].filter((id): id is string => !!id))))
      .then(result => {
        if (request !== referenceRequest.current) return;
        setReferenceResults(result.sessions);
        setReferenceIndex(0);
      })
      .catch(reason => {
        if (request === referenceRequest.current) setReferenceError(reason instanceof Error ? reason.message : String(reason));
      })
      .finally(() => {if (request === referenceRequest.current) setReferenceLoading(false);});
    return () => {};
  }, [referencePicker.open, referencePicker.query, referencePicker.scope, active?.id, active?.project.path, path, prompt, editor]);

  useEffect(() => {
    const show = (event: Event) => setReferenceDetail((event as CustomEvent<AgentConversationReference>).detail);
    window.addEventListener('xiaotao:agent-reference', show);
    return () => window.removeEventListener('xiaotao:agent-reference', show);
  }, []);

  function closeReferencePicker() {
    referenceQueryRef.current = null;
    referenceOpenRef.current = false;
    setReferencePicker(current => current.open ? {...current, open: false} : current);
    setReferenceResults([]);
    setReferenceError('');
  }
  closeReferencePickerRef.current = closeReferencePicker;

  function chooseReference(candidate: AgentReferenceCandidate) {
    const current = referenceQueryRef.current ?? (editor ? referenceQueryAtCursor(editor.state) : null);
    if (!editor || !current) return;
    const node = editor.schema.nodes[conversationReferenceType].create({
      sessionId: candidate.sessionId,
      title: candidate.title,
      projectName: candidate.project.name,
      updatedAt: candidate.updatedAt,
    });
    const tr = editor.state.tr.replaceWith(current.from, editor.state.selection.from, node);
    tr.insertText(' ', current.from + node.nodeSize);
    editor.view.dispatch(tr.scrollIntoView());
    editor.commands.focus();
    closeReferencePicker();
  }

  useEffect(() => {
    let live = true;
    api.agentSessions()
      .then(history => { if (live) setSessions(history.sessions); })
      .catch(reason => live && setError(String(reason)))
      .finally(() => live && setLoading(false));
    return () => {live = false;};
  }, []);

  useEffect(() => {
    if (!visible) return;
    const refresh = refreshModelsRequested.current;
    refreshModelsRequested.current = false;
    let live = true;
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), 30000);
    setModelsLoading(true);
    setModelsError('');
    api.agentModels(controller.signal, refresh)
      .then(available => {
        if (!live) return;
        if (available.errors?.length) setModelsError(available.errors.join('；'));
        setModels(current => {
          const selected = new Set(readEnabledModels());
          return Array.from(new Map([...current.filter(item => selected.has(item.model)), ...available.models].map(item => [item.model, item])).values());
        });
      })
      .catch(reason => live && setModelsError(controller.signal.aborted
        ? '获取模型列表超时，请检查账号状态后重试'
        : reason instanceof TypeError ? '无法连接小涛服务，请检查服务是否正在运行' : reason instanceof Error ? reason.message : String(reason)))
      .finally(() => { window.clearTimeout(timer); if (live) setModelsLoading(false); });
    return () => {live = false; window.clearTimeout(timer); controller.abort();};
  }, [visible, settingsOpen, modelMenuOpen, modelsAttempt]);

  useEffect(() => {
    if (!visible) { setSettingsOpen(false); setProjectOpen(false); setModelMenuOpen(false); setQuoteSelection(null); }
  }, [visible]);

  useEffect(() => {
    if (feed.current && followFeed.current) feed.current.scrollTop = feed.current.scrollHeight;
    updateScrollToBottom();
  }, [active?.messages, active?.activity.length, busy, visible, updateScrollToBottom]);

  const saveDraft = useCallback(() => {
    if (!editor) return;
    const current = activeSessionRef.current;
    if (current) {
      const cached = sessionCache.current.get(current.id);
      sessionCache.current.set(current.id, cached ? {...current, ...cached,
        messages: mergeHistory(current.messages, cached.messages), activity: mergeHistory(current.activity, cached.activity),
        thoughts: mergeHistory(current.thoughts, cached.thoughts), runStats: mergeRunStats(current.runStats, cached.runStats)} : current);
    }
    drafts.current.set(activeIdRef.current || '', {json: editor.getJSON(), model, path, name: projectName, top: feed.current?.scrollTop || 0, follow: followFeed.current, quotes, images: composerImagesRef.current});
  }, [editor, model, path, projectName, quotes]);
  const restoreDraft = useCallback((id: string, fallbackModel: string) => {
    const draft = drafts.current.get(id);
    setQuotes(draft?.quotes || []); setQuoteSelection(null); setEditingQuote(null);
    setComposerImages(draft?.images || []);
    editor?.commands.setContent(draft?.json || {type:'doc', content:[{type:'paragraph'}]});
    setModel(draft?.model || fallbackModel);
    if (!id) { setPath(draft?.path || ''); setProjectName(draft?.name || ''); }
    followFeed.current = draft?.follow ?? true;
    requestAnimationFrame(() => {if (feed.current && draft && !draft.follow) feed.current.scrollTop = draft.top;});
  }, [editor]);

  async function send(contentOverride?: string) {
    const text = (contentOverride ?? promptRef.current).trim();
    const submittedImages = composerImagesRef.current;
    const submittedQuotes = quotes;
    const quoteBlocks = submittedQuotes.map((item, index) => {
      const quoted = item.text.split('\n').map(line => '> ' + line.replace(/[\\`*_{}[\]<>#~|]/g, '\\$&')).join('\n');
      return `引用小涛的回复 ${index + 1}：\n\n${quoted}${item.reply.trim() ? `\n\n我的回复：${item.reply.trim()}` : ''}`;
    });
    const content = [...quoteBlocks, text].filter(Boolean).join('\n\n');
    if ((!content && !submittedImages.length) || quotes.some(item => !item.text.trim()) || busy || !editor || (!active && !model)) return;
    historyRequest.current++; historyInFlight.current = false; setHistoryLoading(false); prependAnchor.current = null;
    followFeed.current = true;
    const submittedContent = editorMarkdown(editor);
    const references = editorReferenceIds(editor);
    setPending(true);
    setError('');
    setActive(current => current ? {...current, error: ''} : current);
    let session = active;
    let streamID = '';
    let sending = true;
    let submitted = false;
    try {
      if (!session) {
        session = await api.createAgentSession(path, model, projectName, modelSettingsFor(modelConfigs, model, models));
        setActive(session);
        setSessions(current => [session!, ...current]);
      }
      const id = session.id;
      streamID = id;
      sessionCache.current.set(id, {...session, status:'running', error:''});
      runningRef.current.add(id);
      setRunningIds(new Set(runningRef.current));
      const response = await fetch('/api/agent/sessions/' + encodeURIComponent(id) + '/messages', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({content, model: model || session.model, modelSettings: modelSettingsFor(modelConfigs, model || session.model, models), references, images: submittedImages.map(image => ({data: image.data, mimeType: image.mimeType, width: image.width, height: image.height}))}),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => null);
        throw new Error(body?.error?.message || '发送失败（' + response.status + '）');
      }
      if (editorMarkdown(editor) === submittedContent) editor.commands.clearContent();
      setComposerImages(current => current.filter(image => !submittedImages.some(sent => sent.id === image.id)));
      if (!active) drafts.current.delete('');
      drafts.current.delete(id);
      setQuotes(current => current.filter(item => !submittedQuotes.some(sent => sent.id === item.id && sent.text === item.text && sent.reply === item.reply)));
      setEditingQuote(null); closeReferencePicker();
      setPending(false); sending = false;
      await readEventStream(response, event => {
        const cached = sessionCache.current.get(id);
        if (cached) {
          const updated = applyEvent(cached, event);
          sessionCache.current.set(id, updated);
          setSessions(current => [{...updated, messages:[], activity:[]}, ...current.filter(item => item.id !== id)]);
        }
        if (event.type === 'approval' && event.approval) {
          const approval = event.approval;
          void confirm({title:'确认高危操作？', description: `会话「${session?.title || id}」的小涛准备${approval.tool === 'run_command' ? '执行以下命令' : approval.tool === 'enable_mcp_tools' ? '启用浏览器与 MCP 工具' : '进行以下操作'}：\n\n${approval.detail}`, confirmLabel:'允许执行'}).then(async approved => {
            const response = await fetch('/api/agent/sessions/' + encodeURIComponent(id) + '/approval', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:approval.id, approved})});
            if (!response.ok) throw new Error('确认请求已失效');
          }).catch(reason => setError(reason instanceof Error ? reason.message : String(reason)));
        }
        if (event.type === 'session' && event.message) submitted = true;
        setActive(current => current?.id === id ? applyEvent(current, event) : current);
      });
      const page = await api.agentHistory(id);
      const latest = page.session;
      const cached = sessionCache.current.get(latest.id);
      const merged = cached ? {...latest, messages:mergeHistory(latest.messages, cached.messages), activity:mergeHistory(latest.activity, cached.activity), thoughts:mergeHistory(latest.thoughts, cached.thoughts), runStats:mergeRunStats(latest.runStats, cached.runStats)} : latest;
      sessionCache.current.set(latest.id, merged);
      setActive(current => current?.id === id ? {...merged, messages:mergeHistory(merged.messages, current.messages), activity:mergeHistory(merged.activity, current.activity), thoughts:mergeHistory(merged.thoughts, current.thoughts), runStats:mergeRunStats(current.runStats, merged.runStats)} : current);
      setSessions(current => [{...latest, messages:[], activity:[]}, ...current.filter(item => item.id !== id)]);
    } catch (reason) {
      if (activeIdRef.current === session?.id || sending) setError(reason instanceof Error ? reason.message : String(reason));
      if (submitted && session) {
        try {
          const page = await api.agentHistory(session.id);
          const latest = page.session;
          const cached = sessionCache.current.get(latest.id);
          const merged = cached ? {...latest, messages:mergeHistory(latest.messages, cached.messages), activity:mergeHistory(latest.activity, cached.activity), thoughts:mergeHistory(latest.thoughts, cached.thoughts), runStats:mergeRunStats(latest.runStats, cached.runStats)} : latest;
          sessionCache.current.set(latest.id, merged);
          setActive(current => current?.id === latest.id ? {...merged, messages:mergeHistory(merged.messages, current.messages), activity:mergeHistory(merged.activity, current.activity), thoughts:mergeHistory(merged.thoughts, current.thoughts), runStats:mergeRunStats(current.runStats, merged.runStats)} : current);
          setSessions(current => [{...latest, messages:[], activity:[]}, ...current.filter(item => item.id !== latest.id)]);
        } catch { /* retain visible state */ }
      }
    } finally {
      if (sending) setPending(false);
      if (streamID) {
        const cached = sessionCache.current.get(streamID);
        if (cached?.status === 'running') {
          const failed = applyEvent(cached, {type:'status', sessionId:streamID, status:'failed', error:'连接已结束，请查看任务状态'});
          sessionCache.current.set(streamID, failed);
          setSessions(current => current.map(item => item.id === streamID ? {...failed, messages:[], activity:[]} : item));
        }
        runningRef.current.delete(streamID);
        setRunningIds(new Set(runningRef.current));
        setActive(current => current?.id === streamID && current.status === 'running' ? applyEvent(current, {type:'status', sessionId:streamID, status:'failed', error:'连接已结束，请查看任务状态'}) : current);
      }
      if (activeIdRef.current === streamID) editor.commands.focus();
    }
  }
  sendRef.current = () => {void send();};

  async function openReferencedSession(id: string) {
    setReferenceDetail(null);
    const session = sessions.find(item => item.id === id);
    if (session) await selectSession(session);
    else {
      try { const page = await api.agentHistory(id); await selectSession(page.session); }
      catch (reason) { setError(String(reason)); }
    }
  }

  const [sidebarProjectMenu, setSidebarProjectMenu] = useState<{path: string; name: string; key: string} | null>(null);
  const [renamingProject, setRenamingProject] = useState<{path: string; name: string} | null>(null);
  const [savedProjects, setSavedProjects] = useState<AgentProject[]>([]);
  const sidebarProjectTrigger = useRef<HTMLElement | null>(null);
  useEffect(() => { if (!visible || busy) setSidebarProjectMenu(null); }, [visible, busy]);
  useEffect(() => {
    let live = true;
    api.agentProjects()
      .then(result => { if (live) setSavedProjects(result.projects.filter(item => item.saved)); })
      .catch(reason => live && setError(reason instanceof Error ? reason.message : String(reason)));
    return () => {live = false;};
  }, []);
  const newProjectChat = useCallback(async (projectPath: string, name: string) => {
    if (pending) return;
    saveDraft();
    setPending(true);
    try {
      const resolved = await api.resolveAgentProject(projectPath);
      resetHistory(); setActive(null); setQuotes([]); setComposerImages([]); setPath(resolved.path); setProjectName(name); setError('');
      editor?.commands.clearContent(); editor?.commands.focus();
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setPending(false);}
  }, [pending, editor, resetHistory, saveDraft]);
  const openSidebarProjectMenu = useCallback((path: string, name: string, trigger: HTMLElement, key: string) => {
    sidebarProjectTrigger.current = trigger;
    setSessionMenu(null);
    setSidebarProjectMenu({path, name, key});
  }, []);

  function newChat() {
    if (pending) return;
    saveDraft();
    resetHistory();
    setActive(null);
    setPath('');
    setProjectName('');
    setComposerImages([]);
    setError('');
    restoreDraft('', model);
    editor?.commands.focus();
  }

  function stop() {
    if (active) void api.cancelAgentSession(active.id).catch(reason => setError(String(reason)));
  }

  const selectSession = useCallback(async (session: AgentSession) => {
    if (pending || session.id === activeIdRef.current) return;
    saveDraft();
    setError('');
    setPending(true);
    try {
      historyRequest.current++; historyInFlight.current = false; setHistoryLoading(false); prependAnchor.current = null;
      const page = await api.agentHistory(session.id);
      const cached = sessionCache.current.get(session.id);
      const detail = runningRef.current.has(session.id) && cached ? cached : cached
        ? {...page.session, streamDrafts: cached.streamDrafts, messages:mergeHistory(page.session.messages, cached.messages), activity:mergeHistory(cached.activity, page.session.activity), thoughts:mergeHistory(cached.thoughts, page.session.thoughts), runStats:mergeRunStats(cached.runStats, page.session.runStats)} : page.session;
      followFeed.current = true;
      setHistoryBefore(page.before);
      setHistoryError('');
      sessionCache.current.set(detail.id, detail);
      setActive(detail);
      setModel(detail.model);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      return;
    } finally { setPending(false); }
    restoreDraft(session.id, session.model);
    editor?.commands.focus();
  }, [pending, editor, saveDraft, restoreDraft]);

  async function deleteSession(item: AgentSession) {
    if (busy || deleting) return;
    if (!await confirm({title: '删除会话？', description: '“' + item.title + '”的消息和执行记录将一并删除。', confirmLabel: '删除会话'})) return;
    setDeleting(item.id);
    try {
      await api.deleteAgentSession(item.id);
      if (item.project.path && !item.project.isDefault && !item.project.removed) {
        setSavedProjects(current => {
          const key = item.project.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '');
          return current.some(project => project.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '') === key)
            ? current : [...current, {...item.project, saved: true}];
        });
      }
      setExitingSession(item.id);
      const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      if (!reduceMotion) await new Promise(resolve => window.setTimeout(resolve, 180));
      setSessions(current => current.filter(session => session.id !== item.id));
      if (active?.id === item.id) newChat();
      setError('');
      if (!reduceMotion) await new Promise(resolve => window.setTimeout(resolve, 220));
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setDeleting(''); setExitingSession(''); }
  }

  const deleteProject = useCallback(async (item: {path: string; name: string}) => {
    if (busy) return;
    if (!await confirm({title: '删除项目？', description: `项目“${item.name}”会从项目列表移除，里面的会话回到“最新”，不会被删除。`, confirmLabel: '删除项目'})) return;
    const matches = (value: string) => value.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '') === item.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '');
    try {
      await api.deleteAgentProject(item.path);
      setSavedProjects(current => current.filter(project => !matches(project.path)));
      setSessions(current => current.map(session => matches(session.project.path) ? {...session, project: {...session.project, removed: true}} : session));
      setActive(current => current && matches(current.project.path) ? {...current, project: {...current.project, removed: true}} : current);
      setError('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }, [busy, confirm]);

  function saveAgentSettings(nextConfigs: AgentModelConfig[], nextDefaults: AgentDefaultModels, nextDefaultSettings: AgentModelDefaults) {
    const nextModels = nextConfigs.map(item => item.model);
    persistModelSettings(nextModels, nextConfigs, nextDefaults, nextDefaultSettings);
    localStorage.setItem(modelLabelsKey, JSON.stringify(Object.fromEntries(models.filter(item => nextModels.includes(item.model)).map(item => [item.model, item.label]))));
    if (!active && !nextModels.includes(model)) setModel(nextModels[0] || '');
    setSettingsOpen(false);
  }

  const activity = active?.activity ?? [];
  const thoughts = active?.thoughts ?? [];
  const runStats = useMemo(() => active?.runStats ?? [], [active?.runStats]);
  useLayoutEffect(() => {
    const area = feed.current;
    if (!area) return;
    const observer = new ResizeObserver(() => {
      if (followFeed.current) area.scrollTop = area.scrollHeight;
      updateScrollToBottom();
    });
    area.querySelectorAll('.agent-run-timeline').forEach(element => observer.observe(element));
    return () => observer.disconnect();
  }, [active?.id, active?.runStats?.length, updateScrollToBottom]);
  const contextRun = runStats.at(-1);
  const contextTokens = contextRun?.contextTokens ?? 0;
  const currentModel = model || active?.model || '';
  const contextWindow = contextRun?.contextWindow ?? modelConfigFor(modelConfigs, currentModel, models)?.contextWindow ?? models.find(item => item.model === currentModel)?.contextWindow ?? 0;
  const contextRatio = contextWindow > 0 ? Math.min(1, contextTokens / contextWindow) : 0;
  const hasReference = !!editor && editorReferenceIds(editor).length > 0;
  const currentModelOption = models.find(item => item.model === (model || active?.model));
  const currentModelSupportsImages = currentModelOption?.supportsImages === true;
  const historyHasImages = !!active && conversationHasImages(active.messages, active.hasImages);
  const isReviewingCurrent = !!active && reviewingSessionId === active.id;
  const canSend = !busy && !isReviewingCurrent && quotes.every(item => !!item.text.trim()) && (!!prompt.trim() || quotes.length > 0 || hasReference || composerImages.length > 0) && (!!active || !!model) && (!(composerImages.length || historyHasImages) || currentModelSupportsImages || currentModelOption === undefined);
  canSendRef.current = canSend;

  const {entries} = useMemo(() => {
    type Entry = {kind: 'message'; item: AgentMessage; stats?: AgentRunStats} | {kind: 'run'; runId: string; stats: AgentRunStats; phase: 'live' | 'done'};
    const timeline = conversationMessages(active?.messages ?? []);
    const entries: Entry[] = timeline.map(item => ({kind: 'message', item,
      stats: item.role === 'assistant' ? runStats.find(run => run.runId === item.runId) : undefined}));
    for (const run of runStats) {
      for (let i = entries.length - 1; i >= 0; i--) { const entry = entries[i]; if (entry.kind === 'message' && entry.item.role === 'assistant' && entry.item.runId === run.runId) entries.splice(i, 1); }
      entries.push({kind: 'run', runId: run.runId, stats: run,
        phase: runPhase(run) === 'working' ? 'live' : 'done'});
    }
    const time = (entry: Entry) => entry.kind === 'message' ? entry.item.createdAt : entry.stats.startedAt;
    entries.sort((a, b) => time(a).localeCompare(time(b)) || (a.kind === 'message' && a.item.role === 'user' ? -1 : 1));
    return {entries};
  }, [active?.messages, runStats]);
  const {avatarOwners} = useMemo(() => {
    const owners = new Set<number>();
    let response: number | undefined;
    const finishTurn = () => {
      const owner = response;
      if (owner !== undefined) owners.add(owner);
      return owner === undefined;
    };
    entries.forEach((entry, index) => {
      if (entry.kind === 'message' && entry.item.role === 'user') {
        finishTurn();
        response = undefined;
      } else if (entry.kind === 'message' && entry.item.role === 'assistant') {
        response ??= index;
      }
    });
    finishTurn();
    return {avatarOwners: owners};
  }, [entries]);
  const workingIndicator = <div className="agent-working-row" role="status"><div className="agent-message-avatar"><AgentDroplet running/></div><div className="agent-working"><span><strong>正在思考</strong><small>准备下一步…</small></span></div></div>;
  const activeProjectName = active ? active.project.name : projectName || (path ? path.split(/[\\/]/).filter(Boolean).pop() : '') || '默认工作目录';
  const activeProjectPath = active ? active.project.path : path;
  const availablePickerModels = models.filter(item => enabledModels.includes(item.model));
  if (model && !availablePickerModels.some(item => item.model === model)) availablePickerModels.push({model, label: models.find(item => item.model === model)?.label || model});
  if (active?.model && !availablePickerModels.some(item => item.model === active.model)) availablePickerModels.push({model: active.model, label: active.model});

  const projects = useMemo(() => Array.from(new Map([...savedProjects, ...([...sessions].sort((a,b) => b.updatedAt.localeCompare(a.updatedAt))).filter(item => item.project.path && !item.project.isDefault).map(item => item.project), ...(path ? [{path, name: activeProjectName, isDefault: false, saved: false}] : [])].map(item => [item.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, ''), item])).values()).filter(item => `${item.name} ${item.path}`.toLowerCase().includes(projectQuery.trim().toLowerCase())), [savedProjects, sessions, path, activeProjectName, projectQuery]);

  return <section className="agent-workspace" aria-label="小涛 Agent">
    <aside className="agent-sidebar">
      <button className="agent-new" type="button" onClick={newChat} disabled={pending}>
        <MessageSquarePlus aria-hidden="true"/><span>新会话</span>
      </button>
      <div className="agent-history-heading">
        <span>{view === 'project' ? '项目' : '会话'}</span>
        <button ref={viewButtonRef} type="button" className="agent-history-switch" aria-label="切换会话排列方式" aria-expanded={viewMenuOpen} aria-controls="agent-history-menu" title="切换排列方式" onClick={() => setViewMenuOpen(open => !open)}><ListFilter aria-hidden="true"/></button>
        <Popover open={viewMenuOpen} onClose={() => setViewMenuOpen(false)} anchorRef={viewButtonRef} align="end" id="agent-history-menu" className="agent-history-menu" role="group" aria-label="会话排列方式">
          {(['project', 'session'] as const).map(option => <button type="button" key={option} className={view === option ? 'is-active' : ''} aria-pressed={view === option} onClick={() => {setView(option); localStorage.setItem('xiaotao.agent.historyView', option); setViewMenuOpen(false); viewButtonRef.current?.focus();}}><span>{option === 'project' ? '按项目查看' : '按会话查看'}</span>{view === option && <Check aria-hidden="true"/>}</button>)}
        </Popover>
      </div>
      <SessionList runningIds={runningIds} sessions={sessions} savedProjects={savedProjects} activeId={active?.id} view={view} loading={loading} busy={pending} deleting={deleting} exitingId={exitingSession} menuId={sidebarProjectMenu?.key || sessionMenu?.item.id} onNewProjectChat={newProjectChat} onProjectMenu={openSidebarProjectMenu} onDeleteProject={item => void deleteProject(item)} onSelect={selectSession} onMenu={openSessionMenu}/>
      <div className="agent-sidebar-foot">
        <span className="agent-status-dot"/>
        <span>本地工作区</span>
      </div>
    </aside>

    <div className={'agent-main-panel ' + (active ? 'has-session' : 'is-empty')} role="main">
      <header className="agent-context-bar">
        <div className="agent-context-title">
          <AgentDroplet running={busy}/>
          <div><strong>{active ? active.title : '新会话'}</strong><small>{busy ? '小涛正在处理' : active ? 'Agent 工作台' : '准备开始'}</small></div>
        </div>
        <div className="agent-context-actions">
          {active && (
            <button
              type="button"
              className={`agent-context-btn ${reviewingSessionId === active.id ? 'is-loading' : ''}`}
              disabled={!canStartReview}
              onClick={() => void handleStartReview()}
              title={
                busy
                  ? '任务结束后可复盘'
                  : !hasCompletedRuns
                  ? '会话中暂无已完成的 Agent 执行记录，无法进行复盘'
                  : '检查当前会话中小涛的执行行为，生成独立记录；会调用当前模型并产生正常模型用量'
              }
              aria-label="复盘当前会话"
            >
              {reviewingSessionId === active.id ? (
                <>
                  <Loader2 size={13} className="spin" aria-hidden="true"/>
                  <span>复盘中…</span>
                </>
              ) : (
                <>
                  <Sparkles size={13} aria-hidden="true"/>
                  <span>复盘</span>
                </>
              )}
            </button>
          )}

          <button
            type="button"
            className="agent-context-btn"
            onClick={() => setReviewListOpen(true)}
            title="查看复盘记录"
            aria-label="打开复盘记录"
          >
            <History size={13} aria-hidden="true"/>
            <span>复盘记录</span>
          </button>

          <button type="button" className="agent-settings-trigger" onClick={() => setSettingsOpen(true)} title="Agent 设置" aria-label="打开 Agent 设置">
            <Settings2 aria-hidden="true"/><span>设置</span>
          </button>
        </div>
      </header>

      <div className="agent-feed" ref={feed} onMouseUp={captureQuoteSelection} onKeyUp={event => {if (event.shiftKey || event.key === 'Shift' || ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a')) captureQuoteSelection();}} onScroll={event => { const el = event.currentTarget; followFeed.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80; updateScrollToBottom(); if (el.scrollTop < 120 && !historyError) void loadOlder(); }}>
        {active && historyBefore && <div className="agent-history-load"><button type="button" disabled={historyLoading || busy} onClick={() => void loadOlder()}>{historyLoading ? '正在加载历史…' : historyError ? '加载失败，点击重试' : '加载更早的消息'}</button>{historyError && <span role="status">{historyError}</span>}</div>}
        {!active && <div className="agent-empty">
          <div className="agent-empty-mark"><img src="/agent/mascot-hero.png" alt=""/></div>
          <h1>今天想做什么？</h1>
        </div>}

        {entries.map((entry, index) => {
          if (entry.kind === 'run') {
            const running = entry.phase === 'live';
            return <ReactFragment key={entry.runId}>
              <RunTimeline runId={entry.runId} thoughts={thoughts} activity={activity} streamDraft={active?.streamDrafts?.find(item => item.runId === entry.runId)} stats={entry.stats} busy={running} answer={conversationMessages(active?.messages ?? []).find(item => item.role === 'assistant' && item.runId === entry.runId)} onOpen={() => setOpenThoughtRun(entry.runId)}/>
            </ReactFragment>;
          }
          const followsProcess = entry.item.role === 'assistant' && !avatarOwners.has(index);
          return <MessageRow key={entry.item.id} item={entry.item} followsProcess={followsProcess} stats={entry.stats}/>;
        })}
        {busy && !entries.some(entry => entry.kind === 'run' || (entry.kind === 'message' && entry.stats && !entry.stats.endedAt)) && workingIndicator}
        {(error || (!busy && active?.error)) && <div className="agent-error" role="alert"><Circle aria-hidden="true"/><span>{error || active?.error}</span></div>}
      </div>

      <div className="agent-composer-wrap" ref={composer}>
        <button type="button" className={'agent-scroll-bottom' + (showScrollToBottom ? ' is-visible' : '')} onClick={scrollToBottom} aria-label="回到底部" title="回到底部" aria-hidden={!showScrollToBottom} tabIndex={showScrollToBottom ? 0 : -1} inert={!showScrollToBottom}><ArrowDown aria-hidden="true"/></button>
        <div className="agent-project-row">
          <div className={'agent-project-picker' + (path && !active ? ' has-project' : '')}>
          <Popover open={projectMenuOpen} onClose={() => setProjectMenuOpen(false)} anchorRef={projectTriggerRef} initialFocusRef={projectSearchRef} side="top" className="agent-project-menu" role="dialog" aria-label="选择项目">
            <label className="agent-project-search"><Search aria-hidden="true"/><input ref={projectSearchRef} value={projectQuery} onChange={event => setProjectQuery(event.target.value)} placeholder="搜索项目" aria-label="搜索项目"/></label>
            <div className="agent-project-menu-list">
              {projects.map(project => <button type="button" key={project.path} title={project.path} onClick={() => {void api.resolveAgentProject(project.path).then(resolved => {setPath(resolved.path); setProjectName(project.name); setError(''); setProjectMenuOpen(false); setProjectQuery(''); projectTriggerRef.current?.focus();}).catch(reason => {setError(reason instanceof Error ? reason.message : String(reason)); setProjectMenuOpen(false);});}}><Folder aria-hidden="true"/><span>{project.name}</span>{path.toLowerCase() === project.path.toLowerCase() && <Check aria-label="当前项目"/>}</button>)}
              {!projects.length && <p>{projectQuery ? '没有匹配的项目' : '暂无项目，选择文件夹开始'}</p>}
            </div>
            <div className="agent-project-menu-actions"><button type="button" onClick={() => {setProjectMenuOpen(false); setProjectQuery(''); setProjectOpen(true);}}><Plus aria-hidden="true"/>选择新项目</button><button type="button" onClick={() => {setPath(''); setProjectName(''); setProjectMenuOpen(false); setProjectQuery(''); projectTriggerRef.current?.focus();}}><X aria-hidden="true"/>使用默认工作目录{!path && <Check aria-label="当前选择"/>}</button></div>
          </Popover>
          <button
            ref={projectTriggerRef}
            type="button"
            className="agent-project-trigger"
            onClick={() => setProjectMenuOpen(open => !open)}
            aria-expanded={projectMenuOpen}
            disabled={busy || !!active}
            title={active?.project.path || path || '使用默认工作目录'}
          >
            <Folder aria-hidden="true"/>
            <span>{activeProjectName}</span>
          </button>
          {!active && path && <button type="button" className="agent-project-clear" disabled={busy} onClick={() => {setPath(''); setProjectName('');}} aria-label="清空工作区目录" title="清空工作区目录"><X aria-hidden="true"/></button>}
          </div>
        </div>
        <div className="agent-composer" ref={referenceComposerRef}>
          {!!quotes.length && <div className="agent-quote-drafts" aria-label="引用的文字">{quotes.map((item, index) => <div className="agent-quote-chip" key={item.id}><button type="button" className="agent-quote-chip-open" title={item.text} onClick={() => setEditingQuote(item.id)} aria-label={`编辑引用 ${index + 1}`}><span>{index + 1}</span><span>{item.text}</span></button><button type="button" className="agent-quote-chip-remove" aria-label={`移除引用 ${index + 1}`} onClick={() => {setQuotes(current => current.filter(quote => quote.id !== item.id)); setEditingQuote(current => current === item.id ? null : current);}}><X aria-hidden="true"/></button></div>)}</div>}
          {!!composerImages.length && <div className="agent-composer-images" aria-label="待发送图片">
            {composerImages.map((image, index) => <div className="agent-composer-image" key={image.id}>
              <img src={image.preview} alt={image.name} title={image.name}/>
              <button type="button" aria-label={`移除图片 ${index + 1}`} title="移除图片" onClick={() => setComposerImages(current => current.filter(item => item.id !== image.id))}><X aria-hidden="true"/></button>
            </div>)}
          </div>}
          <EditorContent editor={editor} className="agent-editor"/>
          {referencePicker.open && <ReferencePicker anchor={referenceComposerRef} editor={editor} anchorAt={referenceQueryRef.current?.from ?? editor?.state.selection.from ?? 0} query={referencePicker.query} scope={referencePicker.scope} results={referenceResults} index={referenceIndex} loading={referenceLoading} error={referenceError} onScope={scope => {setReferencePicker(current => ({...current, scope})); setReferenceIndex(0);}} onChoose={chooseReference} onClose={() => {closeReferencePicker(); editor?.commands.focus();}}/>}
          <div className="agent-toolbar">
            <div className="agent-composer-meta">
            <div className="agent-mode-picker">
              <button ref={modeButtonRef} type="button" className="agent-option" aria-label="访问模式" aria-expanded={modeMenuOpen} disabled={!accessReady} onClick={() => setModeMenuOpen(open => !open)}>{accessMode === 'safe' ? '安全模式' : '完全访问'}<ChevronDown aria-hidden="true"/></button>
              <Popover open={modeMenuOpen} onClose={() => setModeMenuOpen(false)} anchorRef={modeButtonRef} side="top" className="agent-model-menu" aria-label="访问模式">
                <button type="button" role="menuitemradio" aria-checked={accessMode === 'safe'} onClick={() => void changeAccessMode('safe')}><span>安全模式 · 高危操作需确认</span>{accessMode === 'safe' && <Check aria-hidden="true"/>}</button>
                <button type="button" role="menuitemradio" aria-checked={accessMode === 'full'} onClick={() => void changeAccessMode('full')}><span>完全访问 · 自主执行</span>{accessMode === 'full' && <Check aria-hidden="true"/>}</button>
              </Popover>
            </div>
            {contextTokens > 0 && <button type="button" className="agent-context-meter" aria-label={contextWindow > 0 ? `上下文用量 ${Math.round(contextRatio * 100)}%` : `上下文用量，约 ${contextTokens} tokens，上限未知`} title={contextWindow > 0 ? `已用约 ${contextTokens} / ${contextWindow} tokens` : '已用约 ' + contextTokens + ' tokens · 模型上下文上限未知'}>
              <span className={contextWindow > 0 ? 'agent-context-ring' : 'agent-context-dot'} aria-hidden="true" style={{'--context-ratio': contextRatio} as React.CSSProperties}><i/></span>
              <span>{contextWindow > 0 ? `ctx ${Math.round(contextRatio * 100)}%` : `ctx ${formatCompactTokens(contextTokens)}`}</span>
            </button>}
            </div>
            <div className="agent-composer-actions">
              <div className="agent-model-picker">
                <button ref={modelButtonRef} type="button" className="agent-option agent-model" aria-label="选择模型" aria-haspopup="menu" aria-expanded={modelMenuOpen} disabled={busy || active?.status === 'running' || !availablePickerModels.length} onClick={() => setModelMenuOpen(open => !open)}>
                  <span>{availablePickerModels.find(item => item.model === (model || active?.model))?.label || '先在设置中选择模型'}</span><ChevronDown aria-hidden="true"/>
                </button>
                <Popover open={modelMenuOpen} onClose={() => setModelMenuOpen(false)} anchorRef={modelButtonRef} side="top" align="end" className="agent-model-menu" aria-label="选择模型">
                  {availablePickerModels.map(item => <button type="button" role="menuitemradio" aria-checked={item.model === model} key={item.model} onClick={() => {setModel(item.model); setModelMenuOpen(false); modelButtonRef.current?.focus();}}><span>{item.label}<ModelPriceBadge {...item}/></span>{item.model === model && <Check aria-hidden="true"/>}</button>)}
                </Popover>
              </div>
              {!!composerImages.length && currentModelSupportsImages && <span className="agent-image-model-hint"><Image aria-hidden="true"/>图片将随消息发送</span>}
              {(!!composerImages.length || historyHasImages) && !currentModelSupportsImages && <span className="agent-image-model-warning" role="status">{historyHasImages ? '会话包含图片，当前模型不支持' : '当前模型不支持图片'}</span>}
              {busy
                ? <button type="button" className="agent-send is-stop" onClick={stop} aria-label="停止任务" title="停止任务"><Square aria-hidden="true"/></button>
                : <button type="button" className="agent-send" disabled={!canSendRef.current} onClick={() => void send()} aria-label="发送消息" title="发送消息"><ArrowUp aria-hidden="true"/></button>}
            </div>
          </div>
        </div>
        <div className="agent-hint">
          <span className="agent-project-context" title={activeProjectPath || '小涛数据目录中的默认工作区'}>{activeProjectPath ? `工作目录：${activeProjectPath}` : '工作目录：小涛数据目录中的默认工作区'}</span>
          {!active && !model && <button type="button" onClick={() => setSettingsOpen(true)}>先选择模型，才能发送</button>}
        </div>
      </div>
    </div>

    {openThoughtRun && (() => {
      const stats = runStats.find(item => item.runId === openThoughtRun);
      return stats ? <ThoughtDrawer runId={openThoughtRun} thoughts={thoughts} activity={activity} stats={stats} open={!!openThoughtRun} onClose={() => setOpenThoughtRun('')}/> : null;
    })()}
    <Popover open={!!sidebarProjectMenu} onClose={() => setSidebarProjectMenu(null)} anchorRef={sidebarProjectTrigger} align="end" className="agent-session-menu" aria-label="项目操作"><button type="button" role="menuitem" onClick={() => {setRenamingProject(sidebarProjectMenu); setSidebarProjectMenu(null);}}><Pencil aria-hidden="true"/>重命名项目</button><button type="button" role="menuitem" onClick={() => {const project = sidebarProjectMenu; setSidebarProjectMenu(null); if (project) void api.openAgentProject(project.path).catch(reason => setError(reason instanceof Error ? reason.message : String(reason)));}}><Folder aria-hidden="true"/>在资源管理器中打开</button></Popover>
    <ModalPresence>{renamingProject && <RenameSessionModal project title={renamingProject.name} onClose={() => setRenamingProject(null)} onSave={async name => {const target = renamingProject.path; await api.renameAgentProject(target, name); const matches = (value: string) => value.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '') === target.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, ''); setSessions(current => current.map(item => matches(item.project.path) ? {...item, project:{...item.project, name}} : item)); setActive(current => current && matches(current.project.path) ? {...current, project:{...current.project, name}} : current); setSavedProjects(current => current.map(item => matches(item.path) ? {...item, name} : item)); if (matches(path)) setProjectName(name); setRenamingProject(null);}}/>}</ModalPresence>
    {visible && createPortal(<>{quoteMarks.map(mark => <ReactFragment key={mark.id}>{mark.rects.map((rect, index) => <span key={index} className="agent-quote-highlight" style={rect} aria-hidden="true"/>)}<button type="button" className="agent-quote-marker" style={{left: mark.left, top: mark.top}} aria-label={`编辑引用 ${quotes.findIndex(item => item.id === mark.id) + 1}`} title="编辑引用" onClick={() => setEditingQuote(mark.id)}>{quotes.findIndex(item => item.id === mark.id) + 1}</button></ReactFragment>)}{editingQuote && (() => {const item = quotes.find(quote => quote.id === editingQuote); if (!item) return null; const mark = quoteMarks.find(value => value.id === item.id); const left = mark ? Math.max(8, Math.min(window.innerWidth - 296, mark.left - 120)) : Math.max(8, (window.innerWidth - 288) / 2); const top = mark ? Math.max(8, Math.min(window.innerHeight - 270, mark.top + 24)) : Math.max(8, (window.innerHeight - 260) / 2); return <div className="agent-quote-editor" role="dialog" aria-label="编辑引用" style={{left, top}}><div className="agent-quote-editor-head"><strong>编辑引用 {quotes.findIndex(quote => quote.id === item.id) + 1}</strong><button type="button" aria-label="关闭引用编辑" onClick={() => setEditingQuote(null)}><X aria-hidden="true"/></button></div><label>引用文字<textarea aria-label="引用文字" value={item.text} onChange={event => setQuotes(current => current.map(quote => quote.id === item.id ? {...quote, text:event.target.value} : quote))}/></label><label>回复<textarea aria-label="回复" placeholder="写下对这段文字的回复（可选）" value={item.reply} onChange={event => setQuotes(current => current.map(quote => quote.id === item.id ? {...quote, reply:event.target.value} : quote))}/></label><div className="agent-quote-editor-actions"><button type="button" onClick={() => {setQuotes(current => current.filter(quote => quote.id !== item.id)); setEditingQuote(null);}}>移除引用</button><button type="button" disabled={!item.text.trim()} onClick={() => setEditingQuote(null)}>完成</button></div></div>;})()}</>, document.body)}
    {quoteSelection && createPortal(<button type="button" className="agent-quote-selection" style={{left: quoteSelection.left, top: quoteSelection.top}} onMouseDown={event => event.preventDefault()} onClick={() => {const selected = quoteSelection; const existing = quotes.find(item => item.messageId === selected.messageId && item.start === selected.start && item.end === selected.end); if (existing) {setEditingQuote(existing.id);} else {setQuotes(current => [...current, {id: crypto.randomUUID(), messageId: selected.messageId, start: selected.start, end: selected.end, sourceText: selected.text, text: selected.text, reply: ''}]);} setQuoteSelection(null); window.getSelection()?.removeAllRanges(); editor?.commands.focus('end');}}>引用</button>, document.body)}
    <Popover open={!!sessionMenu} onClose={() => setSessionMenu(null)} anchorRef={sessionMenuTrigger} point={sessionMenu ? {x: sessionMenu.x, y: sessionMenu.y} : undefined} className="agent-session-menu" aria-label="会话操作">
      <button type="button" role="menuitem" onClick={() => {if (sessionMenu) { selectSession(sessionMenu.item); setReviewListOpen(true); } setSessionMenu(null);}}><History aria-hidden="true"/>复盘记录</button>
      <button type="button" role="menuitem" onClick={() => {if (sessionMenu) setRenaming(sessionMenu.item); setSessionMenu(null);}}><Pencil aria-hidden="true"/>重命名</button>
      <button type="button" role="menuitem" className="is-danger" onClick={() => {if (!sessionMenu) return; const item = sessionMenu.item; setSessionMenu(null); void deleteSession(item);}}><Trash2 aria-hidden="true"/>删除</button>
    </Popover>
    <ModalPresence>{referenceDetail && <ReferenceModal reference={referenceDetail} onOpenSource={id => void openReferencedSession(id)} onClose={() => setReferenceDetail(null)}/>}</ModalPresence>
    <ModalPresence>{renaming && <RenameSessionModal title={renaming.title} origin={(() => {const rect = sessionMenuTrigger.current?.getBoundingClientRect(); return rect ? {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2} : undefined;})()} onGenerateTitle={signal => {const utility = utilityModelFor('title', defaultModels, defaultModelSettings, modelConfigs, models); return api.generateAgentSessionTitle(renaming.id, signal, utility.model, utility.settings).then(result => result.title);}} onClose={() => {setRenaming(null); sessionMenuTrigger.current?.focus();}} onSave={async title => {const updated = await api.renameAgentSession(renaming.id, title); setSessions(current => current.map(item => item.id === updated.id ? {...item, title: updated.title} : item)); setActive(current => current?.id === updated.id ? {...current, title: updated.title} : current); setRenaming(null); sessionMenuTrigger.current?.focus();}}/>}</ModalPresence>
    <ModalPresence>{reviewListOpen && <ReviewListModal currentSessionId={active?.id} onClose={() => setReviewListOpen(false)} />}</ModalPresence>
    <ModalPresence>{reviewDetailOpen && <ReviewDetailModal review={reviewDetailOpen} onClose={() => setReviewDetailOpen(null)} />}</ModalPresence>
    {reviewToast && createPortal(
      <div className="agent-review-toast" role="status">
        <Sparkles size={14} aria-hidden="true"/>
        <span>{reviewToast}</span>
      </div>,
      document.body
    )}
    <ModalPresence>{settingsOpen && <AgentSettingsModal
      models={models}
      selected={enabledModels}
      modelConfigs={modelConfigs}
      defaultModels={defaultModels}
      defaultModelSettings={defaultModelSettings}
      loading={modelsLoading}
      error={modelsError}
      onRetry={() => { refreshModelsRequested.current = true; setModelsAttempt(value => value + 1); }}
      onAccounts={() => { setSettingsOpen(false); window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail: 'accounts'})); }}
      onSave={saveAgentSettings}
      onClose={() => setSettingsOpen(false)}
    />}</ModalPresence>
    <ModalPresence>{projectOpen && <ProjectModal
      name={projectName}
      path={path}
      onApply={(nextPath, nextName) => {
        setPath(nextPath);
        setProjectName(nextName);
        setProjectOpen(false);
        if (nextPath) setSavedProjects(current => {
          const key = nextPath.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '');
          return [...current.filter(project => project.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '') !== key), {path: nextPath, name: nextName, saved: true}];
        });
        setError('');
      }}
      onClose={() => setProjectOpen(false)}
    />}</ModalPresence>
  </section>;
}

