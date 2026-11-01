import { useEffect, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Activity, Cpu, Gauge, Minus, UserRound } from 'lucide-react';
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
  const current = snap?.activeRequests[0] ?? null;
  const displayStatus = current
    ? '实时请求'
    : snap
      ? '当前空闲'
      : connected
        ? '等待请求'
        : '连接中';
  const todayTokens = snap ? snap.today.promptTokens + snap.today.completionTokens : 0;
  const accountLabel = useMemo(
    () => (current ? accountName(current.accountId, accounts) : '—'),
    [accounts, current],
  );

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
          ) : current ? (
            <div className="request-hud-current">
              <div className="request-hud-status">
                <span className={`request-hud-status-dot ${current.status}`} aria-hidden="true" />
                <span>{requestStatus(current, nowMs)}</span>
                {snap.activeCount > 1 && <small>共 {snap.activeCount} 个</small>}
              </div>

              <div className="request-hud-row" title={`账号：${accountLabel}`}>
                <UserRound aria-hidden="true" />
                <span>账号</span>
                <strong>{accountLabel}</strong>
              </div>
              <div className="request-hud-row" title={`模型：${current.model}`}>
                <Cpu aria-hidden="true" />
                <span>模型</span>
                <strong className="is-code">{current.model}</strong>
              </div>
              <div className="request-hud-row is-speed" title={`输出速度：${speedText(current)}`}>
                <Gauge aria-hidden="true" />
                <span>速度</span>
                <strong>{speedText(current)}</strong>
              </div>
            </div>
          ) : (
            <div className="request-hud-idle">
              <Activity aria-hidden="true" />
              <strong>当前空闲</strong>
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
