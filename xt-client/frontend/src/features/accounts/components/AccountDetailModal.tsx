import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import type { Account, ProviderInfo } from '../../../lib/api';
import { ProviderIcon } from './ProviderIcon';
import {
  calculateQuotaSummary,
  getDetailItems,
  formatQuotaItemLine,
  formatRefreshTime,
} from '../quotaUtils';

interface AccountDetailModalProps {
  account: Account | null;
  providerInfo?: ProviderInfo;
  isBusy: boolean;
  onClose: () => void;
  onToggleEnable: (account: Account) => Promise<void>;
  onRefreshQuota: (account: Account) => Promise<void>;
  onDeleteAccount: (account: Account) => Promise<void>;
  onRelogin?: (account: Account) => void;
}

export const AccountDetailModal: React.FC<AccountDetailModalProps> = ({
  account,
  providerInfo,
  isBusy,
  onClose,
  onToggleEnable,
  onRefreshQuota,
  onDeleteAccount,
  onRelogin,
}) => {
  const modalRef = useRef<HTMLDivElement>(null);

  // Keyboard handling (Escape to close)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  if (!account) return null;

  const displayName = account.alias || account.nickname || `用户 ${account.uid}`;
  const providerName = providerInfo?.displayName || account.provider.toUpperCase();
  const summary = calculateQuotaSummary(account.quota);
  const detailItems = getDetailItems(account.quota?.items || []);
  const refreshTimeStr = formatRefreshTime(account.quota?.fetchedAt);
  const isAuthValid = account.authState === 'valid';
  const isQuotaError = account.quota?.status === 'error';

  return createPortal(
    <div
      className="modal-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-account-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="account-detail-dialog" ref={modalRef}>
        {/* Dialog Header */}
        <div className="dialog-header">
          <h3 id="modal-account-title" className="dialog-title">账号详情</h3>
          <button
            type="button"
            className="dialog-close-btn"
            onClick={onClose}
            title="关闭 (Esc)"
            aria-label="关闭详情弹窗"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        {/* Dialog Body */}
        <div className="dialog-scroll-body">
          {/* Identity & Status */}
          <div className="detail-profile-row">
            <div className="profile-icon-col">
              <ProviderIcon provider={account.provider} size={48} />
            </div>
            <div className="profile-info-col">
              <h4 className="profile-name" title={displayName}>{displayName}</h4>
              <span className="profile-source">来自 {providerName}{account.provider === 'qoder' ? (account.region === 'global' ? ' · 海外版' : ' · 国内版') : ''}</span>
              <div className="profile-status-capsules">
                <span className={`status-capsule ${isAuthValid ? 'auth-ok' : 'auth-warn'}`}>
                  <span className="capsule-dot" />
                  {isAuthValid ? '登录有效' : '登录失效'}
                </span>

                {/* iOS switch for enabled */}
                <label className="capsule-switch-pill" title={account.enabled ? '点击停用' : '点击启用'}>
                  <input
                    type="checkbox"
                    checked={account.enabled}
                    disabled={isBusy}
                    onChange={() => onToggleEnable(account)}
                  />
                  <span className="pill-knob" />
                  <span className="pill-text">{account.enabled ? '已启用' : '已停用'}</span>
                </label>

              </div>
            </div>
          </div>

          {/* Total Quota Panel */}
          <div className="detail-total-quota-card">
            <div className="total-quota-header">
              <span className="total-title">剩余额度</span>
              <span className="total-ratio">{isQuotaError ? '刷新失败' : summary.ratioText}</span>
            </div>
            <div className="total-digits-row">
              <span className="big-rem-num">{summary.remainingText}</span>
              <span className="slash">/</span>
              <span className="limit-num">{summary.limitText}</span>
            </div>
            <div className="total-progress-track">
              <div
                className={`total-progress-fill ${isQuotaError ? 'is-warn' : ''}`}
                style={{ width: `${summary.percent}%` }}
              />
            </div>
          </div>

          {/* Quota Details Sub-items */}
          {detailItems.length > 0 && (
            <div className="detail-sub-items-section">
              <h5 className="sub-items-title">配额明细</h5>
              <div className="sub-items-list">
                {detailItems.map((item, idx) => {
                  const formatted = formatQuotaItemLine(item);
                  return (
                    <div key={idx} className="sub-item-row">
                      <span className="sub-item-label">{item.label}{item.expireAt ? <small> · {new Date(item.expireAt * 1000).toLocaleDateString('zh-CN')} 到期</small> : null}</span>
                      <div className="sub-item-value">
                        <span className="sub-val-cur">{formatted.usedText}</span>
                        <span className="sub-val-slash">/</span>
                        <span className="sub-val-limit">{formatted.limitText}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* Last refreshed time */}
          <div className="detail-meta-footer">
            <span>最近刷新：{refreshTimeStr}</span>
          </div>
        </div>

        {/* Dialog Actions Footer */}
        <div className="dialog-footer">
          <div className="footer-left">
            <button
              type="button"
              className="dialog-btn text-danger"
              disabled={isBusy}
              onClick={() => onDeleteAccount(account)}
            >
              移除账号
            </button>
          </div>

          <div className="footer-right">
            <button
              type="button"
              className="dialog-btn secondary"
              disabled={isBusy}
              onClick={() => onRefreshQuota(account)}
            >
              <svg
                width="14"
                height="14"
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
              <span>刷新配额</span>
            </button>
            {onRelogin && <button type="button" className="dialog-btn" disabled={isBusy} onClick={() => onRelogin(account)}>重新登录</button>}

            <button
              type="button"
              className="dialog-btn primary-finish"
              onClick={onClose}
            >
              完成
            </button>
          </div>
        </div>
      </div>
    </div>, document.body
  );
};
