import {ChevronDown, Gauge, Layers3} from 'lucide-react';
import {useId, useState} from 'react';
import type {DashboardSnapshot} from '../../lib/api';
import type {ContextParts, DashboardConversation} from '../../lib/dashboard-conversation';
import {RecentRequests} from './RecentRequests';
import {formatNumber} from './UsageOverview';
import './conversation-context.css';

const parts: Array<{key: keyof ContextParts; label: string; color: string}> = [
  {key: 'messages', label: '对话消息', color: 'var(--context-blue)'},
  {key: 'system', label: '系统提示', color: 'var(--context-purple)'},
  {key: 'tools', label: '工具定义', color: 'var(--context-teal)'},
  {key: 'toolCalls', label: '工具调用与结果', color: 'var(--context-orange)'},
  {key: 'references', label: '对话引用', color: 'var(--context-pink)'},
  {key: 'images', label: '图片', color: 'var(--context-lime)'},
];
const compact = new Intl.NumberFormat('en', {notation: 'compact', maximumFractionDigits: 1});
const recentConversationCount = 3;

function ContextBar({context, ratio}: {context: DashboardConversation['context']; ratio: number | null}) {
  const sections = parts.filter(part => context.parts[part.key] > 0);
  const total = sections.reduce((sum, part) => sum + context.parts[part.key], 0);
  return <span className="context-bar" data-unknown={ratio === null} aria-hidden="true">
    {ratio !== null && <span className="context-bar-fill" style={{width: `${Math.min(ratio * 100, 100)}%`}}>
      {sections.map(part => <span key={part.key} style={{width: `${context.parts[part.key] / total * 100}%`, background: part.color}}/>)}
    </span>}
  </span>;
}

function ContextDetails({session, nowMs, usage}: {session: DashboardConversation; nowMs: number; usage: string}) {
  const {context, requests, totals} = session;
  const [selected, setSelected] = useState<keyof ContextParts>();
  const sections = parts.filter(part => context.parts[part.key] > 0);
  const current = sections.find(part => part.key === selected) ?? sections[0];
  const detail = current ? context.items?.[current.key] : undefined;
  return <div className="context-panel">
    <p className="context-usage-detail">{usage}</p>
    {current && <>
      <div className="context-legend" aria-label="上下文构成估算">
        {sections.map(part => <button type="button" key={part.key} aria-pressed={current.key === part.key} onClick={() => setSelected(part.key)}>
          <i aria-hidden="true" style={{background: part.color}}/>
          <span>{part.label}</span><strong>{formatNumber(context.parts[part.key])}</strong>
        </button>)}
      </div>
      <section className="context-part-detail" aria-label="分块明细">
        <header><strong>{current.label}</strong><span>约 {formatNumber(context.parts[current.key])} tokens</span></header>
        {!!detail?.length && <ul>{detail.map((item, index) => <li key={index}><span>{item.label}</span><span>{formatNumber(item.tokens)}</span></li>)}</ul>}
      </section>
    </>}
    <details className="context-requests">
      <summary>最近请求 <span>已确认累计输入 {formatNumber(totals.prompt)} · 输出 {formatNumber(totals.completion)} tokens</span></summary>
      <RecentRequests requests={requests} nowMs={nowMs}/>
    </details>
  </div>;
}

function ConversationRow({session, open, onToggle, nowMs}: {
  session: DashboardConversation; open: boolean; onToggle: () => void; nowMs: number;
}) {
  const panel = useId();
  const {context, requests} = session;
  const latest = requests.find(request => request.id === session.latestRequestId) ?? requests[0];
  const active = session.active > 0;
  const speed = latest?.speedTokPerSec;
  const measured = latest?.tokens.prompt;
  const used = measured ?? (context.estimated > 0 ? context.estimated : null);
  const window = context.window;
  const ratio = window > 0 && used !== null ? used / window : null;
  const percentage = ratio === null ? '—' : `${ratio > 0 && ratio < .01 ? '<1' : Math.round(ratio * 100)}%`;
  const qualifier = used === null ? '输入未知' : measured == null ? '估算输入' : '上游输入用量';
  const source = [session.source, latest?.model].filter(Boolean).join(' · ');
  const tokens = `${used == null ? '—' : `${measured == null ? '≈' : ''}${compact.format(used)}`} / ${window > 0 ? compact.format(window) : '—'}`;
  const exact = `${qualifier} ${formatNumber(used)} tokens · ${window > 0 ? `窗口 ${formatNumber(window)} tokens` : '窗口未知'}`;

  return <li className="context-entry" data-active={active}>
    <button type="button" className="context-row" aria-expanded={open} aria-controls={panel} onClick={onToggle}>
      <span className="context-identity">
        <span className="context-name">
          <strong title={session.title}>{session.title}</strong>
          <small title={source}>{source}</small>
        </span>
      </span>
      <span className="context-status">{active ? '进行中' : '已结束'}</span>
      <span className="context-capacity">
        <span className="context-tokens" title={exact}><span aria-hidden="true">{tokens}</span><span className="sr-only">{exact}</span></span>
        <strong className="context-percent" data-near-limit={ratio !== null && ratio >= .8}>{percentage}</strong>
        <ContextBar context={context} ratio={ratio}/>
      </span>
      <span className="context-speed" title="最近一次主请求的输出速度">
        <Gauge size={14} aria-hidden="true"/>
        <span><strong>{speed == null ? '—' : speed.toFixed(1)}</strong> <small>tok/s</small></span>
      </span>
      <ChevronDown className="context-chevron" size={15} aria-hidden="true"/>
    </button>
    <div id={panel} className="context-disclosure" data-open={open} inert={!open} aria-hidden={!open}>
      <div><ContextDetails session={session} nowMs={nowMs} usage={exact}/></div>
    </div>
  </li>;
}

export function ConversationContext({snapshot, nowMs}: {snapshot: DashboardSnapshot; nowMs: number}) {
  const [selected, setSelected] = useState<string | null>(null);
  const conversations = snapshot.conversations ?? [];
  const active = conversations.filter(session => session.active > 0);
  const recent = conversations.filter(session => session.active === 0).slice(0, recentConversationCount);
  const sessions = [...active, ...recent];
  return <section className="requests-section conversation-context" aria-label="会话上下文">
    <div className="requests-header context-heading">
      <h2><Layers3 size={17} aria-hidden="true"/>会话上下文</h2>
      {sessions.length > 0 && <span className="context-counts">
        <span data-active={active.length > 0}>{active.length} 个进行中</span>
        <span>最近结束 {recent.length} 个</span>
      </span>}
    </div>
    {sessions.length ? <div className="context-card">
      <div className="context-columns" aria-hidden="true"><span>会话 / 模型</span><span>状态</span><span>上下文占用</span><span>输出速度</span></div>
      <ul className="context-list" aria-label="会话列表">
        {sessions.map(session => <ConversationRow key={session.id} session={session} nowMs={nowMs} open={selected === session.id}
          onToggle={() => setSelected(current => current === session.id ? null : session.id)}/>)}
      </ul>
    </div> : <div className="empty-requests-card">暂无会话请求</div>}
  </section>;
}
