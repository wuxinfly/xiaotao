import {useId, useRef, useState} from 'react';
import {ChevronDown} from 'lucide-react';
import {Popover} from '../popover/Popover';
import {SelectPanel, type SelectOption} from './SelectPanel';
import './select.css';

export type {SelectOption};

export type SelectFieldProps = {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  searchable?: boolean;
  disabled?: boolean;
  placeholder?: string;
  emptyText?: string;
  searchPlaceholder?: string;
  className?: string;
  size?: 'sm' | 'md';
  id?: string;
  'aria-label'?: string;
  'aria-labelledby'?: string;
};

export function Select({value, options, onChange, searchable = false, disabled = false, placeholder = '请选择', emptyText = '暂无可选项', searchPlaceholder = '搜索选项', className = '', size = 'md', id, 'aria-label': ariaLabel, 'aria-labelledby': ariaLabelledby}: SelectFieldProps) {
  const [open, setOpen] = useState(false);
  const [session, setSession] = useState(0);
  const [minWidth, setMinWidth] = useState(0);
  const trigger = useRef<HTMLButtonElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const panelId = useId();
  const selected = options.find(option => option.value === value);
  const label = selected?.label ?? placeholder;

  function openPanel() {
    if (disabled) return;
    setMinWidth(trigger.current?.getBoundingClientRect().width ?? 0);
    setSession(current => current + 1);
    setOpen(true);
  }

  function pick(next: string) {
    if (next !== value) onChange(next);
    setOpen(false);
    trigger.current?.focus({preventScroll: true});
  }

  return <>
    <button ref={trigger} id={id} type="button"
      className={`app-select-trigger app-select-trigger--${size}${selected ? '' : ' is-placeholder'}${className ? ` ${className}` : ''}`}
      aria-haspopup="listbox" aria-expanded={open} aria-controls={panelId}
      aria-label={ariaLabel} aria-labelledby={ariaLabelledby}
      title={selected?.label} disabled={disabled}
      onClick={() => {if (open) setOpen(false); else openPanel();}}
      onKeyDown={event => {if (!open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {event.preventDefault(); openPanel();}}}>
      <span className="app-select-value">{label}</span>
      <ChevronDown className="app-select-chevron" size={14} aria-hidden="true"/>
    </button>
    <Popover open={open} onClose={() => setOpen(false)} anchorRef={trigger} id={panelId} role="listbox"
      aria-label={ariaLabel ?? '选项列表'} className={`app-select-panel app-select-panel--${size}`}
      initialFocusRef={searchable ? search : undefined}>
      <SelectPanel value={value} options={options} onPick={pick} searchable={searchable}
        emptyText={emptyText} searchPlaceholder={searchPlaceholder} minWidth={minWidth}
        session={session} searchRef={search}/>
    </Popover>
  </>;
}
