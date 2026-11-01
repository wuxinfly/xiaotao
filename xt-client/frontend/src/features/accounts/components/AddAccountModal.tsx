import React from 'react';
import { createPortal } from 'react-dom';
import type { ProviderInfo } from '../../../lib/api';
import { ProviderIcon } from './ProviderIcon';

interface AddAccountModalProps {
  providers: ProviderInfo[];
  onSelectProvider: (providerId: string, region?: string) => void;
  onClose: () => void;
}

export const AddAccountModal: React.FC<AddAccountModalProps & { onImportCodex?: () => void }> = ({
  providers,
  onSelectProvider,
  onImportCodex,
  onClose,
}) => {
  const dialogRef = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    dialog?.querySelector<HTMLButtonElement>('button')?.focus();
    const trap = (event: KeyboardEvent) => {
      if (event.key !== 'Tab' || !dialog) return;
      const nodes = Array.from(dialog.querySelectorAll<HTMLElement>('button, select, [tabindex="0"]'));
      const first = nodes[0], last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    dialog?.addEventListener('keydown', trap);
    return () => { dialog?.removeEventListener('keydown', trap); previous?.focus(); };
  }, []);
  React.useEffect(() => { const handler = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); }; window.addEventListener('keydown', handler); return () => window.removeEventListener('keydown', handler); }, [onClose]);
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

  return createPortal(
    <div
      className="modal-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-add-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="add-account-dialog" ref={dialogRef}>
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
          {onImportCodex && (
            <button
              type="button"
              className="add-provider-item"
              style={{ borderStyle: "dashed" }}
              onClick={onImportCodex}
            >
              <div className="provider-item-icon">
                <ProviderIcon provider="codex" size={36} />
              </div>
              <div className="provider-item-info">
                <strong className="provider-item-name">导入 Codex 登录态</strong>
              </div>
              <div className="provider-item-arrow">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </div>
            </button>
          )}
          {available.flatMap((p) =>
            (p.id === 'qoder' ? ['cn', 'global'] : [undefined]).map((region) => (
              <button
                key={region ? `${p.id}-${region}` : p.id}
                type="button"
                className="add-provider-item"
                onClick={() => onSelectProvider(p.id, region)}
              >
                <div className="provider-item-icon">
                  <ProviderIcon provider={p.id} size={36} />
                </div>
                <div className="provider-item-info">
                  <strong className="provider-item-name">
                    {p.displayName}
                    {region === 'cn' && ' 国内版'}
                    {region === 'global' && ' 海外版'}
                  </strong>
                </div>
                <div className="provider-item-arrow">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <polyline points="9 18 15 12 9 6" />
                  </svg>
                </div>
              </button>
            ))
          )}
        </div>
      </div>
    </div>, document.body
  );
};
