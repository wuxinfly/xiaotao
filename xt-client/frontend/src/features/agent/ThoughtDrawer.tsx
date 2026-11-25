import {useEffect, useMemo, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {ChevronRight, CircleAlert, CircleCheck, FilePenLine, FileSearch, Route, Terminal, Wrench, X, Zap} from 'lucide-react';
import type {AgentActivity, AgentRunStats, AgentThought} from '../../lib/api';
import {runDuration, thoughtKindLabel} from './messages/runStats';

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
    if (row.activityKind === 'write_file' || row.activityKind === 'str_replace') return FilePenLine;
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

// The drawer mounts already open, so shown flips one frame late: the closed
// styles paint first and the CSS transition gets a start value to animate from.
// Closing keeps present mounted until the exit transition has finished.
function useDrawerPresence(open: boolean) {
  const [present, setPresent] = useState(open);
  const [shown, setShown] = useState(false);
  useEffect(() => {
    if (!open) {
      setShown(false);
      const timer = window.setTimeout(() => setPresent(false), 260);
      return () => window.clearTimeout(timer);
    }
    setPresent(true);
    const frame = window.requestAnimationFrame(() => setShown(true));
    return () => window.cancelAnimationFrame(frame);
  }, [open]);
  return {present, shown};
}

export function ThoughtDrawer({runId, thoughts, activity, stats, open, onClose}: {
  runId: string; thoughts: AgentThought[]; activity: AgentActivity[]; stats: AgentRunStats; open: boolean; onClose: () => void;
}) {
  const close = useRef<HTMLButtonElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const {present, shown} = useDrawerPresence(open);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    if (open) returnFocus.current = document.activeElement as HTMLElement | null;
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
  return createPortal(<div className={'agent-drawer-backdrop' + (open && shown ? ' is-open' : ' is-closing')} onPointerDown={event => {if (event.target === event.currentTarget) onClose();}} inert={!open}>
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

