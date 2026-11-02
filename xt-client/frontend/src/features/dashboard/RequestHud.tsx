import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Activity, Minus } from 'lucide-react';
import type { Account, RequestSummary } from '../../lib/api';
import { useDashboardData } from './DashboardData';
import './request-hud.css';

interface RequestHudProps {
  enabled: boolean;
  agentPage: boolean;
  dashboardPage: boolean;
  onOpen: () => void;
  onClose: () => void;
}

function formatCompactNumber(value: number): string {
  if (value < 10_000) return value.toLocaleString('zh-CN');
  if (value < 100_000_000) {
    const scaled = value / 10_000;
    return `${scaled >= 100 ? scaled.toFixed(0) : scaled.toFixed(1)}万`;
  }
  return `${(value / 100_000_000).toFixed(1)}亿`;
}

function accountName(accountId: string, accounts: Account[]): string {
  if (!accountId) return '未指定账号';
  const account = accounts.find((item) => item.id === accountId);
  if (account) return account.alias || account.nickname || account.uid || account.id;
  const suffix = accountId.split(':').filter(Boolean).pop();
  return suffix ? `账号 ${suffix.slice(0, 10)}` : accountId;
}

function requestStatus(request: RequestSummary, nowMs: number): string {
  const elapsedSec = Math.max(0, (nowMs - request.startedAt) / 1000);
  if (request.status === 'pending') return `等待响应 · ${elapsedSec.toFixed(0)}s`;
  if (request.status === 'streaming') return `正在输出 · ${elapsedSec.toFixed(1)}s`;
  return request.statusText;
}

function speedText(request: RequestSummary): string {
  if (request.status === 'pending') return '等待首包';
  if (request.speedTokPerSec == null) return '—';
  return `${request.speedTokPerSec.toFixed(1)} tok/s`;
}

export function RequestHud({ enabled, agentPage, dashboardPage, onOpen, onClose }: RequestHudProps) {
  const { snap, accounts, connected, loading, error, nowMs } = useDashboardData();
  const restoreRef = useRef<HTMLButtonElement>(null);
  const requests = snap ? [...new Map(
    [...snap.activeRequests, ...snap.recentRequests].map(request => [request.id, request]),
  ).values()].sort((a, b) => {
    const active = (request: RequestSummary) => request.status === 'pending' || request.status === 'streaming' ? 1 : 0;
    return active(b) - active(a) || b.startedAt - a.startedAt;
  }).slice(0, 3) : [];
  const displayStatus = '最近请求';
  const todayTokens = snap ? snap.today.promptTokens + snap.today.completionTokens : 0;
  useEffect(() => {
    if (!enabled) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose();
        window.requestAnimationFrame(() => restoreRef.current?.focus());
      }
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [enabled, onClose]);

  const connectionText = snap
    ? connected
      ? '实时连接'
      : '正在重连'
    : loading
      ? '正在连接'
      : error
        ? '暂时离线'
        : '实时连接';

  return createPortal(
    <>
      {!enabled && (
        <button
          ref={restoreRef}
          type="button"
          className="request-hud-toggle"
          aria-pressed="false"
          onClick={onOpen}
          title="开启实时请求浮窗"
        >
          <Activity aria-hidden="true" />
          <span>请求浮窗</span>
        </button>
      )}
      <aside
        className={`request-hud${agentPage ? ' is-agent' : ''}${dashboardPage ? ' is-dashboard' : ''}`}
        aria-label="实时请求浮窗"
        aria-hidden={!enabled}
        inert={!enabled}
      >
        <header className="request-hud-header">
          <span className={`request-hud-pulse${connected ? ' is-live' : ''}`} aria-hidden="true" />
          <strong title={displayStatus}>{displayStatus}</strong>
          <span className={`request-hud-connection${connected ? ' is-live' : ''}`}>
            {connectionText}
          </span>
          <button
            type="button"
            className="request-hud-minimize"
            onClick={onClose}
            aria-label="收起请求浮窗"
            title="收起请求浮窗"
          >
            <Minus aria-hidden="true" />
          </button>
        </header>

        <div className="request-hud-body">
          {!snap ? (
            <div className="request-hud-state">
              <Activity aria-hidden="true" />
              <span>{error || '正在读取请求状态…'}</span>
            </div>
          ) : requests.length > 0 ? (
            <div className="hud-request-list" role="list" aria-label="最近三条请求">
              {requests.map(request => {
                const elapsed = Math.max(0, ((request.endedAt || nowMs) - request.startedAt) / 1000);
                const account = accountName(request.accountId, accounts);
                return <div className={`hud-request-item is-${request.status}`} role="listitem" key={request.id}>
                  <div className="hud-request-line">
                    <i className={`request-hud-status-dot ${request.status}`} aria-hidden="true" />
                    <strong title={request.model}>{request.model}</strong>
                    <span className="hud-request-speed">{speedText(request)}</span>
                  </div>
                  <div className="hud-request-meta">
                    <span title={account}>{account}</span>
                    <span title={requestStatus(request, nowMs)}>{request.status === 'pending' ? '等待响应' : request.status === 'streaming' ? '正在输出' : request.statusText} · {elapsed.toFixed(1)}s</span>
                  </div>
                </div>;
              })}
            </div>          ) : (
            <div className="request-hud-idle">
              <Activity aria-hidden="true" />
              <strong>暂无请求</strong>
              <span>下一条请求会自动显示在这里</span>
            </div>
          )}
        </div>

        <footer className="request-hud-today">
          <span>今日用量</span>
          <strong>{snap ? `${formatCompactNumber(todayTokens)} Token` : '—'}</strong>
          <small>{snap ? `${formatCompactNumber(snap.today.totalRequests)} 次请求` : '等待数据'}</small>
        </footer>
      </aside>
    </>,
    document.body,
  );
}
