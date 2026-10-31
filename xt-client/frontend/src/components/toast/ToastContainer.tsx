import { useEffect, useState } from 'react';
import { toast, toastManager, type ToastItem, type ToastType } from './toast';
import './toast.css';

function ToastIcon({ type }: { type: ToastType }) {
  switch (type) {
    case 'success':
      return (
        <svg viewBox="0 0 18 18" fill="none" aria-hidden="true">
          <circle cx="9" cy="9" r="8" fill="var(--toast-success)" />
          <path d="M5.5 9.2L7.8 11.5L12.5 6.8" stroke="#091521" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      );
    case 'error':
      return (
        <svg viewBox="0 0 18 18" fill="none" aria-hidden="true">
          <circle cx="9" cy="9" r="8" fill="var(--toast-error)" />
          <path d="M6.5 6.5L11.5 11.5M11.5 6.5L6.5 11.5" stroke="#091521" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
      );
    case 'warning':
      return (
        <svg viewBox="0 0 18 18" fill="none" aria-hidden="true">
          <path d="M9 2.5L16.2 15H1.8L9 2.5Z" fill="var(--toast-warning)" />
          <path d="M9 7V10M9 12.5V13" stroke="#091521" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
      );
    case 'info':
      return (
        <svg viewBox="0 0 18 18" fill="none" aria-hidden="true">
          <circle cx="9" cy="9" r="8" fill="var(--toast-info)" />
          <path d="M9 5.5V6M9 8.2V12.5" stroke="#091521" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
      );
    case 'loading':
      return (
        <svg viewBox="0 0 18 18" fill="none" className="toast-spinner" aria-hidden="true">
          <circle cx="9" cy="9" r="7" stroke="#224556" strokeWidth="2.2" />
          <path d="M9 2A7 7 0 0 1 16 9" stroke="var(--toast-loading)" strokeWidth="2.2" strokeLinecap="round" />
        </svg>
      );
  }
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path d="M2.5 2.5L9.5 9.5M9.5 2.5L2.5 9.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

export function ToastContainer() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  useEffect(() => {
    return toastManager.subscribe(setToasts);
  }, []);

  if (toasts.length === 0) return null;

  return (
    <div
      className="toast-container"
      aria-live="polite"
      aria-atomic="true"
      onMouseEnter={() => toastManager.pauseAll()}
      onMouseLeave={() => toastManager.resumeAll()}
    >
      {toasts.map(item => (
        <div
          key={item.id}
          className={`toast-item toast-${item.type}${item.isLeaving ? ' is-leaving' : ''}`}
          role={item.type === 'error' ? 'alert' : 'status'}
          onFocus={() => toastManager.pauseAll()}
          onBlur={() => toastManager.resumeAll()}
        >
          <span className="toast-icon-wrap">
            <ToastIcon type={item.type} />
          </span>
          <span className="toast-message">{item.message}</span>
          {item.action && (
            <button
              type="button"
              className="toast-action"
              onClick={() => {
                item.action?.onClick();
                toast.dismiss(item.id);
              }}
            >
              {item.action.label}
            </button>
          )}
          <button
            type="button"
            className="toast-close"
            aria-label="关闭提示"
            title="关闭"
            onClick={() => toast.dismiss(item.id)}
          >
            <CloseIcon />
          </button>
        </div>
      ))}
    </div>
  );
}
