import {useEffect, useRef, useState, type KeyboardEvent, type RefObject} from 'react';

export type SelectOption = {value: string; label: string; hint?: string; disabled?: boolean};

type Props = {
  value: string;
  options: SelectOption[];
  onPick: (value: string) => void;
  searchable: boolean;
  emptyText: string;
  searchPlaceholder: string;
  minWidth: number;
  session: number;
  searchRef: RefObject<HTMLInputElement | null>;
};

export function SelectPanel({value, options, onPick, searchable, emptyText, searchPlaceholder, minWidth, session, searchRef}: Props) {
  const [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null);
  const needle = query.trim().toLowerCase();
  const filtered = needle
    ? options.filter(option => option.label.toLowerCase().includes(needle) || option.hint?.toLowerCase().includes(needle))
    : options;

  useEffect(() => {
    setQuery('');
    const frame = requestAnimationFrame(() => root.current?.querySelector<HTMLElement>('[role="option"][aria-selected="true"]')?.scrollIntoView({block: 'nearest'}));
    return () => cancelAnimationFrame(frame);
  }, [session]);

  function optionsInOrder() {
    return Array.from(root.current?.querySelectorAll<HTMLButtonElement>('[role="option"]:not(:disabled)') ?? []);
  }

  function searchKey(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      const items = optionsInOrder();
      if (!items.length) return;
      event.preventDefault();
      (event.key === 'ArrowDown' ? items[0] : items[items.length - 1]).focus();
      return;
    }
    if (event.key === 'Enter') {
      const first = filtered.find(option => !option.disabled);
      if (!first) return;
      event.preventDefault();
      onPick(first.value);
    }
  }

  return <div ref={root} className="app-select-body" style={minWidth ? {minWidth} : undefined}>
    {searchable && <input ref={searchRef} className="app-select-search" value={query} onChange={event => setQuery(event.target.value)}
      onKeyDown={searchKey} placeholder={searchPlaceholder} aria-label="搜索选项" autoComplete="off"/>}
    <div className="app-select-list" role="presentation">
      {filtered.map(option => <button key={option.value} type="button" role="option" aria-selected={option.value === value}
        disabled={option.disabled} className="app-select-option" onClick={() => onPick(option.value)}>
        <span className="app-select-option-label">{option.label}</span>
        {option.hint && <small className="app-select-option-hint">{option.hint}</small>}
      </button>)}
    </div>
    {!filtered.length && <p className="app-select-empty" role="status">{options.length ? '没有匹配项' : emptyText}</p>}
  </div>;
}
