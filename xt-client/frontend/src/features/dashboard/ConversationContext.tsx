import {useId, useState} from 'react';
import type {DashboardSnapshot} from '../../lib/api';
import type {ContextParts, DashboardConversation} from '../../lib/dashboard-conversation';
import {RecentRequests} from './RecentRequests';
import './conversation-context.css';

const parts: Array<{key: keyof ContextParts; label: string; color: string}> = [
  {key: 'messages', label: '对话消息', color: 'var(--context-blue)'},
  {key: 'system', label: '系统提示', color: 'var(--context-purple)'},
  {key: 'tools', label: '工具定义', color: 'var(--context-teal)'},
  {key: 'toolCalls', label: '工具调用与结果', color: 'var(--context-orange)'},
  {key: 'references', label: '对话引用', color: 'var(--context-pink)'},
  {key: 'images', label: '图片', color: 'var(--context-lime)'},
];

const count = (value?: number | null) => value == null ? '—' : value.toLocaleString('zh-CN');

function ContextDetails({session, nowMs}: {session: DashboardConversation; nowMs: number}) {
  const {context, requests, totals} = session;
  const latest = requests.find(request => request.id === session.latestRequestId) ?? requests[0];
  const measured = latest.tokens.prompt;
  const used = measured ?? context.estimated;
  const window = context.window;
  const percent = window > 0 ? Math.round(used / window * 100) : null;
  const sections = parts.filter(part => context.parts[part.key] > 0);
  const filled = Math.min(100, Math.max(0, percent ?? 100));
  return <>
    <div className="context-panel-heading">
      <div>
        <p className="context-model">{session.source} · {latest.model}</p>
        <h3>{measured == null ? '约 ' : ''}{count(used)}{window > 0 && <small> / {count(window)}</small>} <small>tokens</small></h3>
      </div>
      <span>{percent != null ? `${percent}%` : '窗口未知'}</span>
    </div>
    <div className="context-label">{measured == null ? '最新请求 · 估算输入' : '最新请求 · 上游输入用量'}</div>
    {sections.length > 0 && <div className="context-meter" aria-label="估算上下文组成">
      <div className="context-meter-fill" style={{width: `${filled}%`}}>
        {sections.map(part => <span key={part.key} style={{flexGrow: context.parts[part.key], background: part.color}}/>)}
      </div>
    </div>}
    {sections.length > 0 && <ul className="context-legend" aria-label="估算分项">
      {sections.map(part => <li key={part.key}>
        <i aria-hidden="true" style={{background: part.color}}/>
        <span>{part.label}</span><strong>{count(context.parts[part.key])}</strong>
      </li>)}
    </ul>}
    <details className="context-requests">
      <summary>最近请求 <span>已确认累计输入 {count(totals.prompt)} · 输出 {count(totals.completion)} tokens</span></summary>
      <RecentRequests requests={requests} nowMs={nowMs}/>
    </details>
  </>;
}

export function ConversationContext({snapshot, nowMs}: {snapshot: DashboardSnapshot; nowMs: number}) {
  const [selected, setSelected] = useState('');
  const panel = useId();
  const sessions = snapshot.conversations ?? [];
  const session = sessions.find(item => item.id === selected) ?? sessions[0];
  if (session && session.id !== selected) setSelected(session.id);
  return <section className="requests-section" aria-label="会话上下文">
    <div className="requests-header"><h2>会话上下文</h2></div>
    {session ? <div className="context-card">
      <div className="context-tabs" role="tablist" aria-label="会话" onKeyDown={event => {
        const index = sessions.indexOf(session);
        const next = {ArrowLeft: (index + sessions.length - 1) % sessions.length, ArrowRight: (index + 1) % sessions.length, Home: 0, End: sessions.length - 1}[event.key];
        if (next == null) return;
        event.preventDefault();
        setSelected(sessions[next].id);
        document.getElementById(`${panel}-${sessions[next].id}`)?.focus();
      }}>
        {sessions.map(item => <button key={item.id} type="button" role="tab" id={`${panel}-${item.id}`}
          tabIndex={item.id === session.id ? 0 : -1} aria-selected={item.id === session.id} aria-controls={panel}
          onClick={() => setSelected(item.id)} title={`${item.source} · ${item.title}`}>
          {item.active > 0 && <span className="status-dot" aria-label="有请求进行中"/>}
          <span>{item.title}</span>
          {(item.source === '按历史关联' || item.source === '归属待确认') && <small>{item.source === '按历史关联' ? '推测' : '待确认'}</small>}
        </button>)}
      </div>
      <div key={session.id} className="context-panel" id={panel} role="tabpanel" aria-labelledby={`${panel}-${session.id}`}>
        <ContextDetails session={session} nowMs={nowMs}/>
      </div>
    </div> : <div className="empty-requests-card">暂无会话。小涛或外部工具发起模型请求后，会在这里显示上下文。</div>}
    {!session && snapshot.recentRequests.length > 0 && <details className="context-requests">
      <summary>最近请求</summary>
      <RecentRequests requests={snapshot.recentRequests} nowMs={nowMs}/>
    </details>}
  </section>;
}
