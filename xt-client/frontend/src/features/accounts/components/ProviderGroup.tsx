import React, { useState } from 'react';
import type { Account, ProviderInfo } from '../../../lib/api';
import { ProviderIcon } from './ProviderIcon';
import { AccountCard } from './AccountCard';

interface ProviderGroupProps {
  providerId: string;
  providerInfo?: ProviderInfo;
  accounts: Account[];
  busyId: string | null;
  isReordering: boolean;
  onAddAccount: (providerId: string) => void;
  onToggleEnable: (account: Account) => Promise<void>;
  onRefreshQuota: (account: Account) => Promise<void>;
  onCheckin?: (account: Account) => Promise<void>;
  onCheckinAll?: (providerId: string) => Promise<void>;
  onReorder: (providerId: string, accountIds: string[]) => Promise<void>;
  onOpenDetail: (account: Account) => void;
  onOpenModels: () => void;
}

export const ProviderGroup: React.FC<ProviderGroupProps> = ({
  providerId,
  providerInfo,
  accounts,
  busyId,
  isReordering,
  onAddAccount,
  onToggleEnable,
  onRefreshQuota,
  onCheckin,
  onCheckinAll,
  onReorder,
  onOpenDetail,
  onOpenModels,
}) => {
  const [collapsed, setCollapsed] = useState(true);
  const [draggedId, setDraggedId] = useState<string | null>(null);
  const [dropTargetId, setDropTargetId] = useState<string | null>(null);

  const displayName = providerInfo?.displayName || providerId.toUpperCase();
  const count = accounts.length;
  const toggleCollapsed = () => setCollapsed(value => !value);
  const supportsCheckin = providerId.toLowerCase() === 'trae' || providerId.toLowerCase() === 'workbuddy';
  const canReorder = count > 1 && !isReordering;

  const moveAccount = (accountId: string, delta: -1 | 1) => {
    const index = accounts.findIndex(account => account.id === accountId);
    const target = index + delta;
    if (!canReorder || index < 0 || target < 0 || target >= accounts.length) return;
    const next = [...accounts];
    [next[index], next[target]] = [next[target], next[index]];
    void onReorder(providerId, next.map(account => account.id));
  };

  const dropAccount = (targetId: string) => {
    if (!canReorder || !draggedId || draggedId === targetId) return;
    const sourceIndex = accounts.findIndex(account => account.id === draggedId);
    const targetIndex = accounts.findIndex(account => account.id === targetId);
    if (sourceIndex < 0 || targetIndex < 0) return;
    const next = [...accounts];
    const [moved] = next.splice(sourceIndex, 1);
    next.splice(targetIndex, 0, moved);
    void onReorder(providerId, next.map(account => account.id));
  };

  const clearDrag = () => {
    setDraggedId(null);
    setDropTargetId(null);
  };

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
          {supportsCheckin && count > 0 && onCheckinAll && (
            <button
              type="button"
              className="group-add-btn group-checkin-btn"
              onClick={() => onCheckinAll(providerId)}
              title={`为当前 ${displayName} 账号一键签到打卡`}
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <polyline points="20 6 9 17 4 12" />
              </svg>
              <span>一键打卡</span>
            </button>
          )}
          <button
            type="button"
            className="group-add-btn"
            onClick={onOpenModels}
            title={`查看 ${displayName} 模型目录`}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M4 6h16M4 12h16M4 18h10" />
            </svg>
            <span>查看模型</span>
          </button>
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

      {!collapsed && (
        <div className="accounts-cards-grid" id={`accounts-${providerId}`}>
          {accounts.map((acct, index) => (
            <div
              key={acct.id}
              className={[
                'account-sort-item',
                draggedId === acct.id ? 'is-dragging' : '',
                dropTargetId === acct.id && draggedId !== acct.id ? 'is-drop-target' : '',
              ].filter(Boolean).join(' ')}
              onDragOver={event => {
                if (!canReorder || !draggedId || draggedId === acct.id) return;
                event.preventDefault();
                event.dataTransfer.dropEffect = 'move';
                setDropTargetId(current => current === acct.id ? current : acct.id);
              }}
              onDragLeave={event => {
                if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
                setDropTargetId(current => current === acct.id ? null : current);
              }}
              onDrop={event => {
                event.preventDefault();
                dropAccount(acct.id);
                clearDrag();
              }}
              onDragEnd={clearDrag}
            >
              <AccountCard
                account={acct}
                isBusy={busyId === acct.id}
                canReorder={canReorder}
                onDragStart={event => {
                  setDraggedId(acct.id);
                  event.dataTransfer.effectAllowed = 'move';
                  event.dataTransfer.setData('text/plain', acct.id);
                }}
                onMove={direction => moveAccount(acct.id, direction)}
                canMoveBackward={index > 0}
                canMoveForward={index < accounts.length - 1}
                onToggleEnable={onToggleEnable}
                onRefreshQuota={onRefreshQuota}
                onCheckin={onCheckin}
                onOpenDetail={onOpenDetail}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
