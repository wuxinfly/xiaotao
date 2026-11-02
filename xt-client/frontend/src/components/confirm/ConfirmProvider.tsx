import {createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode} from 'react';
import {createPortal} from 'react-dom';
import {TriangleAlert} from 'lucide-react';
import './confirm.css';

type Options = {title: string; description: string; confirmLabel?: string};
type Request = Options & {resolve: (confirmed: boolean) => void};
const Context = createContext<(options: Options) => Promise<boolean>>(() => Promise.resolve(false));
export const useConfirm = () => useContext(Context);

export function ConfirmProvider({children}: {children: ReactNode}) {
  const [request, setRequest] = useState<Request | null>(null);
  const pending = useRef<Request | null>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const accept = useRef<HTMLButtonElement>(null);
  const previous = useRef<HTMLElement | null>(null);
  const confirm = useCallback((options: Options) => new Promise<boolean>(resolve => {
    if (pending.current) { resolve(false); return; }
    previous.current = document.activeElement as HTMLElement | null;
    const next = {...options, resolve};
    pending.current = next;
    setRequest(next);
  }), []);
  const finish = useCallback((value: boolean) => {
    const current = pending.current;
    if (!current) return;
    pending.current = null;
    setRequest(null);
    current.resolve(value);
    const target = previous.current;
    requestAnimationFrame(() => {if (target?.isConnected) target.focus();});
  }, []);
  useEffect(() => () => {pending.current?.resolve(false); pending.current = null;}, []);
  useEffect(() => {
    if (!request) return;
    cancel.current?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {event.preventDefault(); event.stopImmediatePropagation(); finish(false);}
      if (event.key === 'Tab') {event.preventDefault(); (document.activeElement === cancel.current ? accept.current : cancel.current)?.focus();}
    };
    window.addEventListener('keydown', keydown, true);
    return () => window.removeEventListener('keydown', keydown, true);
  }, [request, finish]);
  return <Context.Provider value={confirm}>{children}{request && createPortal(
    <div className="confirm-overlay" onPointerDown={event => {if (event.target === event.currentTarget) finish(false);}}>
      <section className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-description">
        <div className="confirm-heading"><span className="confirm-icon"><TriangleAlert size={21}/></span><div><span className="confirm-eyebrow">请确认操作</span><h2 id="confirm-title">{request.title}</h2></div></div>
        <p id="confirm-description" className="confirm-description">{request.description}</p>
        <div className="confirm-actions"><button ref={cancel} type="button" className="confirm-cancel" onClick={() => finish(false)}>取消</button><button ref={accept} type="button" className="confirm-danger" onClick={() => finish(true)}>{request.confirmLabel || '确认'}</button></div>
      </section>
    </div>, document.body)}</Context.Provider>;
}
