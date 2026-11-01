import React from 'react';
import type { ProviderInfo } from '../../../lib/api';
import { ProviderIcon } from './ProviderIcon';

interface AddAccountModalProps {
  providers: ProviderInfo[];
  onSelectProvider: (providerId: string) => void;
  onClose: () => void;
}

export const AddAccountModal: React.FC<AddAccountModalProps> = ({
  providers,
  onSelectProvider,
  onClose,
}) => {
  // Available providers from registry (Trae, etc.)
  const available = providers.length > 0 ? providers : [
    {
      id: 'trae',
      displayName: 'Trae',
      loginType: 'browser',
      regions: ['cn'],
      billing: 'credits',
    },
  ];

  return (
    <div
      className="modal-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-add-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="add-account-dialog">
        <div className="dialog-header">
          <h3 id="modal-add-title" className="dialog-title">选择反代接入来源</h3>
          <button
            type="button"
            className="dialog-close-btn"
            onClick={onClose}
            title="关闭 (Esc)"
            aria-label="关闭选择来源弹窗"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="add-provider-list">
          {available.map((p) => (
            <button
              key={p.id}
              type="button"
              className="add-provider-item"
              onClick={() => onSelectProvider(p.id)}
            >
              <div className="provider-item-icon">
                <ProviderIcon provider={p.id} size={36} />
              </div>
              <div className="provider-item-info">
                <strong className="provider-item-name">{p.displayName}</strong>
                <span className="provider-item-desc">
                  {p.id === 'trae'
                    ? '国内版账号授权登录，支持模型代理与配额同步'
                    : p.id === 'workbuddy'
                      ? '国内版 CodeBuddy 插件协议授权，支持模型代理与真实额度'
                      : p.id === 'antigravity'
                        ? 'Google OAuth 授权登录，Gemini / Claude / GPT 免费额度'
                        : '反代服务接入'}
                </span>
              </div>
              <div className="provider-item-arrow">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
};
