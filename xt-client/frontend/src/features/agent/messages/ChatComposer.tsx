import type {ReactNode, Ref} from 'react';
import {ArrowUp, Square} from 'lucide-react';

export function ChatComposer({composerRef, children, controls}: {composerRef?: Ref<HTMLDivElement>; children: ReactNode; controls: ReactNode}) {
  return <div className="agent-composer" ref={composerRef}>{children}<div className="agent-toolbar">{controls}</div></div>;
}

export function ChatSendButton({busy, disabled, onSend, onStop, stopLabel = '停止任务'}: {
  busy: boolean; disabled: boolean; onSend: () => void; onStop: () => void; stopLabel?: string;
}) {
  return busy
    ? <button type="button" className="agent-send is-stop" onClick={onStop} aria-label={stopLabel} title={stopLabel}><Square aria-hidden="true"/></button>
    : <button type="button" className="agent-send" disabled={disabled} onClick={onSend} aria-label="发送消息" title="发送消息"><ArrowUp aria-hidden="true"/></button>;
}
