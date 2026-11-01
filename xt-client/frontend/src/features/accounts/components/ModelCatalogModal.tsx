import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import type { Account, ProviderInfo } from '../../../lib/api';
import { ModelCatalogPanel } from './ModelCatalogPanel';
import { ProviderIcon } from './ProviderIcon';

interface ModelCatalogModalProps {
  providerId: string;
  providerInfo?: ProviderInfo;
  accounts: Account[];
  onClose: () => void;
}

export function ModelCatalogModal({
  providerId,
  providerInfo,
  accounts,
  onClose,
}: ModelCatalogModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusables = () => Array.from(dialog?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), select:not([disabled]), [tabindex="0"]',
    ) ?? []);
    const initialFocus = dialog?.querySelector<HTMLElement>(
      'select:not([disabled]), button:not([disabled])',
    );
    initialFocus?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeRef.current();
        return;
      }
      if (event.key !== 'Tab') return;
      const nodes = focusables();
      if (!nodes.length) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => {
      window.removeEventListener('keydown', handleKeyDown, true);
      previous?.focus();
    };
  }, []);

  const providerName = providerInfo?.displayName || providerId.toUpperCase();

  return createPortal(
    <div
      className="modal-backdrop"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="model-catalog-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="model-catalog-title"
      >
        <div className="dialog-header">
          <div className="model-catalog-dialog-heading">
            <ProviderIcon provider={providerId} size={32} />
            <div>
              <h2 id="model-catalog-title" className="dialog-title">模型目录</h2>
              <p className="model-catalog-dialog-subtitle">
                {providerName} · {accounts.length} 个账号
              </p>
            </div>
          </div>
          <button
            type="button"
            className="dialog-close-btn"
            onClick={onClose}
            title="关闭 (Esc)"
            aria-label="关闭模型目录弹窗"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="dialog-scroll-body model-catalog-dialog-body">
          <ModelCatalogPanel accounts={accounts} providerId={providerId} />
        </div>

        <div className="dialog-footer model-catalog-dialog-footer">
          <span>选择账号可查看对应的模型目录</span>
          <button type="button" className="dialog-btn primary-finish" onClick={onClose}>
            完成
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
