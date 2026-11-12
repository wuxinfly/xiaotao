import {createContext, useContext, useEffect, useLayoutEffect, useId, useRef, useState, type ReactNode} from 'react';
import {createPortal} from 'react-dom';
import {X} from 'lucide-react';
import './modal.css';

const ModalExiting = createContext(false);

export function ModalPresence({children}: {children: ReactNode}) {
  const [retained, setRetained] = useState(children);
  useLayoutEffect(() => { if (children) setRetained(children); }, [children]);
  useEffect(() => {
    if (children) return;
    const timer = window.setTimeout(() => setRetained(null), 180);
    return () => window.clearTimeout(timer);
  }, [children]);
  return <ModalExiting.Provider value={!children}>{children || retained}</ModalExiting.Provider>;
}

export interface ModalFrameProps {
  title: string;
  description?: string;
  className?: string;
  origin?: {x: number; y: number};
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
}

export function ModalFrame({title, description, className = '', origin, onClose, children, footer}: ModalFrameProps) {
  const exiting = useContext(ModalExiting);
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  const initialOrigin = useRef(origin);
  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const triggerRect = trigger && trigger !== document.body && !dialog.contains(trigger) ? trigger.getBoundingClientRect() : null;
    const point = initialOrigin.current || (triggerRect && triggerRect.width && triggerRect.height ? {x: triggerRect.left + triggerRect.width / 2, y: triggerRect.top + triggerRect.height / 2} : null);
    if (!point) return;
    dialog.style.animation = 'none';
    const rect = dialog.getBoundingClientRect();
    dialog.style.setProperty('--modal-origin-x', `${point.x - rect.left}px`);
    dialog.style.setProperty('--modal-origin-y', `${point.y - rect.top}px`);
    dialog.style.setProperty('--modal-enter-x', `${(point.x - rect.left - rect.width / 2) * .1}px`);
    dialog.style.setProperty('--modal-enter-y', `${(point.y - rect.top - rect.height / 2) * .1}px`);
    dialog.style.animation = '';
  }, []);

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusables = () => Array.from(dialog?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]',
    ) ?? []).filter(node => !node.matches(':disabled') && !node.closest('[inert], [hidden], [aria-hidden="true"]') && node.getClientRects().length > 0);
    focusables()[0]?.focus();

    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (event.target instanceof Element && event.target.closest('[data-escape-layer]')) return;
        event.preventDefault();
        closeRef.current();
        return;
      }
      if (event.key !== 'Tab') return;
      const nodes = focusables();
      if (!nodes.length) return;
      const first = nodes[0], last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    window.addEventListener('keydown', handleKey, true);
    return () => {
      window.removeEventListener('keydown', handleKey, true);
      previous?.focus();
    };
  }, []);

  return createPortal(
    <div className={'app-modal-backdrop' + (exiting ? ' is-exiting' : '')} inert={exiting} onPointerDown={event => {
      if (event.target === event.currentTarget) onClose();
    }}>
      <div
        ref={dialogRef}
        className={'app-modal ' + className}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <header className="app-modal-header">
          <div>
            <h2 id={titleId}>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button type="button" className="app-modal-close" onClick={onClose} aria-label="关闭弹窗" title="关闭">
            <X aria-hidden="true"/>
          </button>
        </header>
        <div className="app-modal-body">{children}</div>
        <footer className="app-modal-footer">{footer}</footer>
      </div>
    </div>,
    document.body,
  );
}

