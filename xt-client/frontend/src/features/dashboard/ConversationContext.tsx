import {Layers3} from 'lucide-react';
import type {DashboardSnapshot, RequestSummary} from '../../lib/api';
import type {DashboardConversation} from '../../lib/dashboard-conversation';
import './conversation-context.css';

const recentConversationCount = 3;

function RequestRow({request}: {request: RequestSummary}) {
  const firstToken = request.firstTokenDurationMs;
  const speed = request.speedTokPerSec;
  const waiting = request.status === 'pending';
  const latency = firstToken == null
    ? waiting ? '等待首 Token' : '—'
    : firstToken < 1000 ? `${firstToken} ms` : `${(firstToken / 1000).toFixed(2)} s`;

  return <li className="context-request" data-status={request.status}>
    <span className="context-model" title={request.model || undefined}>
      <strong>{request.model || '模型未知'}</strong>
      <small>{request.statusText}</small>
    </span>
    <span className="context-metric context-latency" data-unavailable={firstToken == null}>
      <span className="context-metric-label">首 Token 耗时</span>
      <span>{latency}</span>
    </span>
    <span className="context-metric context-speed" data-unavailable={speed == null}>
      <span className="context-metric-label">输出速率</span>
      <span>{speed == null ? '—' : <>{speed.toFixed(1)} <small>tok/s</small></>}</span>
    </span>
  </li>;
}

function ConversationRow({session}: {session: DashboardConversation}) {
  const active = session.active > 0;
  const latest = session.requests.find(request => request.id === session.latestRequestId) ?? session.requests[0];
  const requests = active
    ? session.requests.filter(request => request.status === 'pending' || request.status === 'streaming')
    : latest ? [latest] : [];

  return <li className="context-entry" data-active={active}>
    <span className="context-identity">
      <strong title={session.title}>{session.title}</strong>
      <small>{session.source}</small>
    </span>
    <ul className="context-request-list" aria-label={`${session.title}的模型请求`}>
      {requests.map(request => <RequestRow key={request.id} request={request}/>)}
    </ul>
  </li>;
}

export function ConversationContext({snapshot}: {snapshot: DashboardSnapshot}) {
  const conversations = snapshot.conversations ?? [];
  const active = conversations.filter(session => session.active > 0);
  const recent = conversations.filter(session => session.active === 0).slice(0, recentConversationCount);
  const sessions = [...active, ...recent];

  return <section className="requests-section conversation-context" aria-label="会话上下文">
    <div className="requests-header context-heading">
      <h2><Layers3 size={17} aria-hidden="true"/>会话上下文</h2>
      {active.length > 0 && <span className="context-active-count">{active.length} 个会话进行中</span>}
    </div>
    {sessions.length > 0 ? <div className="context-card">
      <div className="context-columns" aria-hidden="true">
        <span>会话</span><span>请求模型</span><span>首 Token 耗时</span><span>输出速率</span>
      </div>
      <ul className="context-list" aria-label="会话列表">
        {sessions.map(session => <ConversationRow key={session.id} session={session}/>)}
      </ul>
    </div> : <div className="empty-requests-card">暂无会话请求</div>}
  </section>;
}
