import {ChevronDown} from 'lucide-react';
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
  const measured = latest?.tokens.prompt;
  const used = measured ?? (context.estimated > 0 ? context.estimated : null);
  const window = context.window;
  const ratio = window > 0 && used !== null ? used / window : null;
  const percentage = ratio === null ? '—' : `${ratio > 0 && ratio < .01 ? '<1' : Math.round(ratio * 100)}%`;
  const qualifier = used === null ? '输入未知' : measured == null ? '估算输入' : '上游输入用量';
  const source = [session.source, latest?.model].filter(Boolean).join(' · ');
  const tokens = `${used == null ? '—' : `${measured == null ? '≈' : ''}${compact.format(used)}`} / ${window > 0 ? compact.format(window) : '—'}`;
  const exact = `${qualifier} ${formatNumber(used)} tokens · ${window > 0 ? `窗口 ${formatNumber(window)} tokens` : '窗口未知'}`;

  return <li className="context-entry">
    <button type="button" className="context-row" aria-expanded={open} aria-controls={panel} onClick={onToggle}>
      <span className="context-identity">
        <span className="status-dot" title="有请求进行中" aria-hidden="true"/>
        <span className="context-name">
          <strong title={session.title}>{session.title}</strong>
          <small title={source}>{source}</small>
        </span>
      </span>
      <ContextBar context={context} ratio={ratio}/>
      <span className="context-tokens" title={exact}><span aria-hidden="true">{tokens}</span><span className="sr-only">{exact}</span></span>
      <strong className="context-percent" data-near-limit={ratio !== null && ratio >= .8}>{percentage}</strong>
      <ChevronDown className="context-chevron" size={15} aria-hidden="true"/>
    </button>
    <div id={panel} hidden={!open}>{open && <ContextDetails session={session} nowMs={nowMs} usage={exact}/>}</div>
  </li>;
}

export function ConversationContext({snapshot, nowMs}: {snapshot: DashboardSnapshot; nowMs: number}) {
  const [selected, setSelected] = useState<string | null>(null);
  const sessions = snapshot.conversations?.filter(session => session.active > 0) ?? [];
  return <section className="requests-section" aria-label="会话上下文">
    <div className="requests-header">
      <h2>会话上下文</h2>
      {sessions.length > 0 && <span className="context-active-count">{sessions.length} 个进行中</span>}
    </div>
    {sessions.length ? <ul className="context-card" aria-label="活跃会话">
      {sessions.map(session => <ConversationRow key={session.id} session={session} nowMs={nowMs} open={selected === session.id}
        onToggle={() => setSelected(current => current === session.id ? null : session.id)}/>)}
    </ul> : <div className="empty-requests-card">暂无进行中的会话</div>}
  </section>;
}
