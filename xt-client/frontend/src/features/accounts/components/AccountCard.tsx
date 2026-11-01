import React from 'react';
import { GripVertical } from 'lucide-react';
import type { Account } from '../../../lib/api';
import { calculateQuotaSummary } from '../quotaUtils';

interface AccountCardProps {
  account: Account;
  isBusy: boolean;
  canReorder: boolean;
  canMoveBackward: boolean;
  canMoveForward: boolean;
  onDragStart: (event: React.DragEvent<HTMLButtonElement>) => void;
  onMove: (direction: -1 | 1) => void;
  onToggleEnable: (account: Account) => Promise<void>;
  onRefreshQuota: (account: Account) => Promise<void>;
  onCheckin?: (account: Account) => Promise<void>;
  onOpenDetail: (account: Account) => void;
}

export const AccountCard: React.FC<AccountCardProps> = ({
  account,
  isBusy,
  canReorder,
  canMoveBackward,
  canMoveForward,
  onDragStart,
  onMove,
  onToggleEnable,
  onRefreshQuota,
  onCheckin,
  onOpenDetail,
}) => {
  const summary = calculateQuotaSummary(account.quota);
  const isAuthValid = account.authState === 'valid';
  const isAuthExpired = account.authState === 'expired' || account.authState === 'invalid';
  const isQuotaError = account.quota?.status === 'error';

  const isTraeCN = account.provider === 'trae' && (account.region === 'cn' || !account.region);
  const isWorkbuddy = account.provider === 'workbuddy';
  const supportsCheckin = isTraeCN || isWorkbuddy;
  const isCheckedIn = !!account.checkin?.checkedIn;

  const displayName = account.alias || account.nickname || `用户 ${account.uid}`;

  return (
    <div className={`modern-account-card ${account.enabled ? '' : 'is-disabled'}`}>
      {/* Card Header */}
      <div className="card-top-row">
        <div className="card-primary">
          <div className="card-identity">
            <button
              type="button"
              className="card-drag-handle"
              draggable={canReorder}
              disabled={!canReorder}
              onDragStart={onDragStart}
              onKeyDown={event => {
                if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
                  if (!canMoveBackward) return;
                  event.preventDefault();
                  onMove(-1);
                } else if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
                  if (!canMoveForward) return;
                  event.preventDefault();
                  onMove(1);
                }
              }}
              title="拖动调整请求顺序，聚焦后可按方向键调整"
              aria-label={`调整 ${displayName} 的请求顺序`}
            >
              <GripVertical size={14} aria-hidden="true" />
            </button>
            <h4 className="card-account-name" title={displayName}>
              {displayName}
            </h4>
            <div className="card-badges">
              {account.provider === 'qoder' && <span className="badge-pill">{account.region === 'global' ? '海外版' : '国内版'}</span>}
              {isAuthValid && (
                <span className="badge-pill badge-auth-valid">
                  <span className="dot" aria-hidden="true" />
                  登录有效
                </span>
              )}
              {isAuthExpired && (
                <span className="badge-pill badge-auth-expired">
                  <span className="dot" aria-hidden="true" />
                  登录失效
                </span>
              )}
              {supportsCheckin && isCheckedIn && (
                <span className="badge-pill badge-checkin-done" title={account.checkin?.streakDays ? `已连签 ${account.checkin.streakDays} 天` : '今日已签到'}>
                  已签到
                </span>
              )}
              {supportsCheckin && !isCheckedIn && account.checkin?.active !== false && (
                <span className="badge-pill badge-checkin-todo" title="今日可签到打卡">
                  待签到
                </span>
              )}
            </div>
          </div>

          <div className="card-quota-body">
            <div className="quota-metric-line">
              <div className="metric-left">
                <span className="metric-tag">剩余</span>
                <span className="metric-primary-value">{summary.remainingText}</span>
                <span className="metric-divider">/</span>
                <span className="metric-limit-value">{summary.limitText}</span>
              </div>
              <div className="metric-right">
                {isQuotaError ? (
                  <span className="quota-status-warn">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                      <circle cx="12" cy="12" r="10" />
                      <line x1="12" y1="8" x2="12" y2="12" />
                      <line x1="12" y1="16" x2="12.01" y2="16" />
                    </svg>
                    刷新失败
                  </span>
                ) : (
                  <span className="metric-ratio">{summary.ratioText}</span>
                )}
              </div>
            </div>
            <div className="quota-progress-track" role="progressbar" aria-valuenow={summary.percent} aria-valuemin={0} aria-valuemax={100}>
              <div
                className={`quota-progress-fill ${isQuotaError ? 'is-warn' : ''}`}
                style={{ width: `${summary.percent}%` }}
              />
            </div>
          </div>
        </div>

        <div className="card-side">
          {/* Enable / Disable Switch */}
          <div className="card-toggle-group">
            <label className="ios-switch" title={account.enabled ? '点击停用' : '点击启用'}>
              <input
                type="checkbox"
                checked={account.enabled}
                disabled={isBusy}
                onChange={() => onToggleEnable(account)}
                aria-label={`${displayName} 启用状态`}
              />
              <span className="switch-slider" />
            </label>
            <span className="switch-label">{account.enabled ? '已启用' : '已停用'}</span>
          </div>

          {/* Actions */}
          <div className="card-actions-row">
            {supportsCheckin && onCheckin && (
              <button
                type="button"
                className={`card-action-btn checkin-btn ${isCheckedIn ? 'is-checked-in' : ''}`}
                disabled={isBusy}
                onClick={() => onCheckin(account)}
                title={isCheckedIn ? (account.checkin?.streakDays ? `今日已签到 (连续 ${account.checkin.streakDays} 天)` : '今日已签到') : '每日签到打卡'}
                aria-label="每日签到打卡"
              >
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </button>
            )}

            <button
              type="button"
              className="card-action-btn refresh-btn"
              disabled={isBusy}
              onClick={() => onRefreshQuota(account)}
              title="刷新此账号配额"
              aria-label="刷新配额"
            >
              <svg
                width="13"
                height="13"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className={isBusy ? 'spin' : ''}
                aria-hidden="true"
              >
                <polyline points="23 4 23 10 17 10" />
                <polyline points="1 20 1 14 7 14" />
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
              </svg>
            </button>

            <button
              type="button"
              className="card-action-btn detail-btn"
              onClick={() => onOpenDetail(account)}
              title="查看详情"
              aria-label="查看账号详情"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
