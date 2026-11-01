import React from 'react';
import type { Account } from '../../../lib/api';
import { calculateQuotaSummary } from '../quotaUtils';

interface AccountCardProps {
  account: Account;
  isDefault: boolean;
  isBusy: boolean;
  onToggleEnable: (account: Account) => Promise<void>;
  onRefreshQuota: (account: Account) => Promise<void>;
  onOpenDetail: (account: Account) => void;
}

export const AccountCard: React.FC<AccountCardProps> = ({
  account,
  isDefault,
  isBusy,
  onToggleEnable,
  onRefreshQuota,
  onOpenDetail,
}) => {
  const summary = calculateQuotaSummary(account.quota);
  const isAuthValid = account.authState === 'valid';
  const isAuthExpired = account.authState === 'expired' || account.authState === 'invalid';
  const isQuotaError = account.quota?.status === 'error';

  const displayName = account.alias || account.nickname || `用户 ${account.uid}`;

  return (
    <div className={`modern-account-card ${account.enabled ? '' : 'is-disabled'}`}>
      {/* Card Header */}
      <div className="card-top-row">
        <div className="card-identity">
          <h4 className="card-account-name" title={displayName}>
            {displayName}
          </h4>
          <div className="card-badges">
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
            {isDefault && <span className="badge-pill badge-default">默认</span>}
          </div>
        </div>

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
      </div>

      {/* Quota Row */}
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

        {/* Progress Bar */}
        <div className="quota-progress-track" role="progressbar" aria-valuenow={summary.percent} aria-valuemin={0} aria-valuemax={100}>
          <div
            className={`quota-progress-fill ${isQuotaError ? 'is-warn' : ''}`}
            style={{ width: `${summary.percent}%` }}
          />
        </div>
      </div>

      {/* Actions */}
      <div className="card-actions-row">
        <button
          type="button"
          className="card-action-btn refresh-btn"
          disabled={isBusy}
          onClick={() => onRefreshQuota(account)}
          title="刷新此账号配额"
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
          <span>刷新</span>
        </button>

        <button
          type="button"
          className="card-action-btn detail-btn"
          onClick={() => onOpenDetail(account)}
        >
          <span>查看详情</span>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <polyline points="9 18 15 12 9 6" />
          </svg>
        </button>
      </div>
    </div>
  );
};
