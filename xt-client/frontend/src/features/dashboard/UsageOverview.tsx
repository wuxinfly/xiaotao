import type { DashboardSnapshot } from '../../lib/api';

interface Props {
  snap: DashboardSnapshot;
  uptimeSeconds: number;
}

export function formatNumber(val: number | null | undefined): string {
  if (val == null || isNaN(val)) return '—';
  return val.toLocaleString('en-US');
}

export function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}秒`;
  const totalMinutes = Math.floor(seconds / 60);
  if (totalMinutes < 60) return `${totalMinutes}分`;
  const hours = Math.floor(totalMinutes / 60);
  const mins = totalMinutes % 60;
  return `${hours}小时${mins}分`;
}

export function formatStartTime(unixSec: number): string {
  const d = new Date(unixSec * 1000);
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  return `${h}:${m} 启动`;
}

export function UsageOverview({ snap, uptimeSeconds }: Props) {
  const session = snap.currentSession;
  const today = snap.today;
  const cumulative = snap.cumulative;
  const todayTotal = today.promptTokens + today.completionTokens;

  return (
    <div className="overview-card" role="region" aria-label="本次启动用量统计">
      <div className="overview-left">
        <div className="overview-header">
          <strong>本次启动</strong>
          <span className="status-dot" aria-hidden="true" />
          <span>已运行 {formatUptime(uptimeSeconds)} · {formatStartTime(snap.launch.startedAt)}</span>
        </div>

        <div className="overview-hero">
          <span className="hero-tokens">{formatNumber(session.totalTokens)}</span>
          <span className="hero-label">总 Token</span>
        </div>

        <div className="overview-submetrics">
          <div className="submetric-col">
            <span className="submetric-label">输入 Token</span>
            <span className="submetric-val">{formatNumber(session.promptTokens)}</span>
          </div>

          <div className="submetric-col">
            <span className="submetric-label">输出 Token</span>
            <span className="submetric-val">{formatNumber(session.completionTokens)}</span>
          </div>

          <div className="submetric-col">
            <span className="submetric-label">
              缓存命中 Token
              <span className="info-tooltip-wrap" tabIndex={0} role="button" aria-label="说明：缓存命中包含在输入 Token 中">
                <svg className="info-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <circle cx="8" cy="8" r="7" stroke="currentColor" strokeWidth="1.4" />
                  <path d="M8 5V5.5M8 7.5V11.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
                </svg>
                <span className="info-tooltip-content">缓存命中包含在输入 Token 中</span>
              </span>
            </span>
            <span className="submetric-val">{formatNumber(session.cachedTokens)}</span>
            <span className="submetric-hint">缓存命中包含在输入 Token 中</span>
          </div>
        </div>
      </div>

      <div className="overview-right">
        <span className="right-today-title">今日用量</span>
        <span className="right-today-val">{formatNumber(todayTotal)}</span>
        <span className="right-cumulative">
          累计使用 {formatNumber(cumulative.totalTokens)} Token
        </span>
      </div>
    </div>
  );
}
