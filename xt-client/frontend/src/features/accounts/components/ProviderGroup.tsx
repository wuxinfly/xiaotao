import React, { useState } from 'react';
import type { Account, ProviderInfo } from '../../../lib/api';
import { ProviderIcon } from './ProviderIcon';
import { AccountCard } from './AccountCard';
import { ModelCatalogPanel } from './ModelCatalogPanel';

interface ProviderGroupProps {
  providerId: string;
  providerInfo?: ProviderInfo;
  accounts: Account[];
  busyId: string | null;
  onAddAccount: (providerId: string) => void;
  onToggleEnable: (account: Account) => Promise<void>;
  onRefreshQuota: (account: Account) => Promise<void>;
  onOpenDetail: (account: Account) => void;
}

export const ProviderGroup: React.FC<ProviderGroupProps> = ({
  providerId,
  providerInfo,
  accounts,
  busyId,
  onAddAccount,
  onToggleEnable,
  onRefreshQuota,
  onOpenDetail,
}) => {
  const [collapsed, setCollapsed] = useState(true);
  const [showModels, setShowModels] = useState(false);
  const [modelsVisited, setModelsVisited] = useState(false);

  const displayName = providerInfo?.displayName || providerId.toUpperCase();
  const count = accounts.length;
  const toggleCollapsed = () => setCollapsed(value => !value);

  return (
    <div className={`provider-group-section ${collapsed ? 'is-collapsed' : ''}`}>
      <div className="provider-group-header">
        <div className="group-left">
          <button
            type="button"
            className="group-collapse-btn"
            onClick={toggleCollapsed}
            aria-expanded={!collapsed}
            aria-controls={`accounts-${providerId}`}
            title={collapsed ? '展开分组' : '收起分组'}
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <path d="m3 9 5-5 5 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <ProviderIcon provider={providerId} size={24} />
          <h3 className="group-title">{displayName}</h3>
          <span className="group-count-badge">{count} 个账号</span>
        </div>

        <div className="group-right">
          <button type="button" className="group-add-btn" aria-expanded={showModels} aria-controls={`models-${providerId}`} onClick={() => { setModelsVisited(true); setShowModels(!showModels); }}>{showModels ? '收起模型' : '查看模型'}</button>
          <button
            type="button"
            className="group-add-btn"
            onClick={() => onAddAccount(providerId)}
            title={`为 ${displayName} 添加账号`}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            <span>添加账号</span>
          </button>
        </div>
      </div>

      <div id={`models-${providerId}`} className={`model-catalog-reveal ${showModels ? 'is-open' : ''}`} inert={!showModels} aria-hidden={!showModels}>
        <div>{modelsVisited && <ModelCatalogPanel accounts={accounts} providerId={providerId} />}</div>
      </div>
      {!collapsed && (
        <div className="accounts-cards-grid" id={`accounts-${providerId}`}>
          {accounts.map((acct) => (
            <AccountCard
              key={acct.id}
              account={acct}
              isBusy={busyId === acct.id}
              onToggleEnable={onToggleEnable}
              onRefreshQuota={onRefreshQuota}
              onOpenDetail={onOpenDetail}
            />
          ))}
        </div>
      )}
    </div>
  );
};
