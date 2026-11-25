import {useRef, type ReactNode, type Ref} from 'react';
import {ArrowUp, ChevronDown, Square} from 'lucide-react';
import {Popover} from '../../../components/popover/Popover';
import type {AgentModelOption} from '../../../lib/api';
import {modelPickerLabel, ModelPickerOptions} from '../modelOptions';

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

export function ChatModelPicker({models, value, displayValue = value, open, onOpen, onSelect, disabled, label = '选择模型'}: {
  models: AgentModelOption[]; value: string; displayValue?: string; open: boolean; onOpen: (open: boolean) => void;
  onSelect: (value: string) => void; disabled: boolean; label?: string;
}) {
  const anchor = useRef<HTMLButtonElement>(null);
  return <div className="agent-model-picker">
    <button ref={anchor} type="button" className="agent-option agent-model" aria-label={label} aria-haspopup="menu" aria-expanded={open}
      disabled={disabled || !models.length} onClick={() => onOpen(!open)}>
      <span>{modelPickerLabel(models, displayValue)}</span><ChevronDown aria-hidden="true"/>
    </button>
    <Popover open={open} onClose={() => onOpen(false)} anchorRef={anchor} side="top" align="end" className="agent-model-menu" aria-label={label}>
      <ModelPickerOptions models={models} value={value} onSelect={model => {onSelect(model); onOpen(false); anchor.current?.focus();}}/>
    </Popover>
  </div>;
}
