import {memo, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {ArrowDown, ArrowUp, CircleStop, Clock3, MessageSquarePlus, PanelRight, TriangleAlert, Zap} from 'lucide-react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import {AgentDroplet} from '../AgentDroplet';
import type {AgentActivity, AgentMessage, AgentRunStats, AgentThought} from '../../../lib/api';
import {MessageActions, MessageCopyButton} from './MessageActions';
import {formatCompactTokens, formatElapsed, liveOutput, liveRunState, runElapsed, runPhase, runSummaryItems, thoughtKindLabel, useTicker} from './runStats';

export function RunSummary({stats}: {stats: AgentRunStats}) {
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
    {items.map(({key, label, Icon, value}) => <span key={key} className="agent-final-stat" tabIndex={0} aria-label={label} onMouseEnter={event => showTip(event, label)} onMouseLeave={hideTip} onFocus={event => showTip(event, label)} onBlur={hideTip}><Icon aria-hidden="true"/><span>{value}</span></span>)}
    {tip && createPortal(<span className="agent-stat-tooltip" role="tooltip" style={tipPosition}>{tip}</span>, document.body)}
  </div>;
}

export const MessageRow = memo(function MessageRow({item, followsProcess, stats, onBranch, branchDisabled}: {
  item: AgentMessage;
  followsProcess: boolean;
  stats?: AgentRunStats;
  onBranch?: (messageId: string) => void;
  branchDisabled?: boolean;
}) {
  const completed = !!stats?.endedAt;
  const canBranch = item.role === 'assistant' && (stats ? completed : true);
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
      {item.role === 'user'
        ? <MessageCopyButton content={item.content}/>
        : <MessageActions messageId={item.id} content={item.content} canBranch={canBranch} branchDisabled={branchDisabled} onBranch={onBranch}/>}
    </div>
  </article>;
}, (previous, next) => previous.followsProcess === next.followsProcess && previous.item.id === next.item.id && previous.item.role === next.item.role && previous.item.content === next.item.content && previous.stats === next.stats && previous.branchDisabled === next.branchDisabled && previous.onBranch === next.onBranch && JSON.stringify(previous.item.references) === JSON.stringify(next.item.references) && JSON.stringify(previous.item.images) === JSON.stringify(next.item.images));

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

export function RunTimeline({runId, thoughts, activity, streamDraft, answer, stats, busy, onOpen, onBranch, branchDisabled, notice}: {
  runId: string; thoughts: AgentThought[]; activity: AgentActivity[];
  streamDraft?: AgentMessage; answer?: AgentMessage; stats: AgentRunStats; busy: boolean; onOpen: () => void;
  onBranch?: (messageId: string) => void; branchDisabled?: boolean; notice?: string;
}) {
  const rows = useMemo(() => [
    ...thoughts.filter(item => item.runId === runId).map(item => ({id: item.id, time: item.createdAt, kind: 'thought', label: thoughtKindLabel[item.kind], text: item.text})),
    ...activity.filter(item => item.runId === runId).map(item => ({id: item.id, time: item.startedAt, kind: 'activity', label: '操作', text: item.title + (item.detail ? ' · ' + item.detail : '')})),
  ].sort((a, b) => a.time.localeCompare(b.time)).slice(-4), [runId, thoughts, activity]);
  const current = thoughts.filter(item => item.runId === runId).at(-1);
  const finished = !!stats.endedAt;
  const phase = runPhase(stats);
  const terminal = finished && (phase === 'failed' || phase === 'canceled');
  const noticeText = phase === 'canceled' ? '任务已停止'
    : notice ? '任务失败：' + notice : '任务失败';
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
    {(!finished || hasAnswer || terminal) && <div className={'agent-run-bubble' + (!finished && streamDraft ? ' is-streaming' : '') + (terminal ? ' is-terminal' : '')} data-phase={terminal ? phase : undefined} data-message-id={answer?.id}>
      {(!terminal || hasAnswer) && <div ref={viewport} className={'agent-result-viewport' + (finished ? ' is-expanded' : '')} style={{height}}><div ref={content} className="agent-run-bubble-text agent-message-content"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{text}</Markdown></div></div>}
      {terminal && <div className="agent-run-notice" role="status">{phase === 'canceled' ? <CircleStop aria-hidden="true"/> : <TriangleAlert aria-hidden="true"/>}<span>{noticeText}</span></div>}
    </div>}
    <div className={'agent-process-collapse' + (finished || !hasEvents ? ' is-collapsed' : '')} inert={finished || !hasEvents} aria-hidden={finished || !hasEvents}><div>
      <div className="agent-run-trace">
        <div className="agent-run-scroll" aria-label="最新四条思考与操作"><ol>{rows.map(row => <li key={row.id} data-kind={row.kind}><span>{row.label}</span><p>{row.text}</p></li>)}</ol></div>
        {!finished && <button type="button" className="agent-run-open" onClick={onOpen} aria-label="查看思考过程" title="查看思考过程"><PanelRight aria-hidden="true"/></button>}
      </div>
      {!finished && <LiveStats stats={stats}/>}
    </div></div>
    {finished && <div className="agent-final-meta-wrap">
      <RunSummary stats={stats}/>
      <MessageActions messageId={answer?.id} content={answer?.content || (terminal ? noticeText : undefined)} canBranch={hasAnswer} branchDisabled={branchDisabled} onBranch={onBranch} onOpenProcess={onOpen}/>
    </div>}
  </div>;
}