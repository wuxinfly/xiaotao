import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Activity, Settings, X } from 'lucide-react';
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
          <Activity className="request-hud-icon" aria-hidden="true" />
          <strong>小涛</strong>
          <span className="request-hud-connection" title={connectionText}>{connectionText}</span>
          <button type="button" className="request-hud-button" aria-label="设置（暂不可用）" title="设置（暂不可用）" disabled>
            <Settings aria-hidden="true" />
          </button>
          <button type="button" className="request-hud-button request-hud-close" onClick={onClose} aria-label="关闭请求浮窗" title="关闭请求浮窗">
            <X aria-hidden="true" />
          </button>
        </header>

        <div className="request-hud-stats">
          <div className="request-hud-total">
            <span>今日用量 · {snap ? `${formatCompactNumber(snap.today.totalRequests)} 次请求` : '等待数据'}</span>
            <strong>{snap ? formatCompactNumber(todayTokens) : '—'}</strong>
          </div>
          <div className="request-hud-direction">
            <span>输入 / 输出 Token</span>
            <div><span className="is-up">↑ {snap ? formatCompactNumber(snap.today.promptTokens) : '—'}</span><span className="is-down">↓ {snap ? formatCompactNumber(snap.today.completionTokens) : '—'}</span></div>
          </div>
        </div>
        <div className="request-hud-list-heading"><span>最近请求</span><span>{requests.length} 条</span></div>
        <div className="request-hud-body">
          {!snap ? (
            <div className="request-hud-state">{error || '正在读取请求状态…'}</div>
          ) : requests.length > 0 ? (
            <div className="hud-request-list" role="list" aria-label="最近三条请求">
              {requests.map((request, index) => {
                const account = accountName(request.accountId, accounts);
                const speed = request.speedTokPerSec;
                const tier = speed == null || speed <= 0 ? 'idle' : speed < 10 ? 'slow' : speed < 50 ? 'normal' : 'fast';
                const elapsed = Math.max(0, ((request.endedAt || nowMs) - request.startedAt) / 1000);
                const details = `${account} · ${request.model} · ${requestStatus(request, nowMs)} · ${elapsed.toFixed(1)}s`;
                return <div className="hud-request-item" role="listitem" key={request.id} title={details}>
                  <span className="hud-request-index">{index + 1}</span>
                  <span className="hud-request-name"><span className="hud-request-account">{account} · </span>{request.model}</span>
                  <span className={`hud-request-speed is-${tier}`} title="输出速率">{speedText(request)}</span>
                  <span className="hud-request-tokens" title="Token 总数">{request.tokens.total == null ? '—' : formatCompactNumber(request.tokens.total)}</span>
                </div>;
              })}
            </div>
          ) : (
            <div className="request-hud-idle">暂无进行中的请求</div>
          )}
        </div>
      </aside>
    </>,
    document.body,
  );
}
