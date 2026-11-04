import {memo, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {ChevronDown, Folder, MoreVertical, MoreHorizontal, SquarePen} from 'lucide-react';
import type {AgentSession} from '../../lib/api';

type Props = {
  sessions: AgentSession[]; activeId?: string; view: 'project' | 'session';
  loading: boolean; busy: boolean; deleting: string; menuId?: string;
  onNewProjectChat: (path: string, name: string) => void;
  onProjectMenu: (path: string, name: string, trigger: HTMLElement, key: string) => void;
  onSelect: (session: AgentSession) => void;
  onMenu: (session: AgentSession, x: number, y: number, trigger: HTMLElement) => void;
};
type Row = {key: string; top: number; height: number} & ({kind: 'session'; item: AgentSession} | {kind: 'group'; name: string; path: string});

function dateLabel(value: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(value)) / 60000));
  if (!Number.isFinite(minutes)) return '';
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return `${minutes} 分钟前`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前`;
  const days = Math.floor(minutes / 1440);
  return days < 30 ? `${days} 天前` : days < 365 ? `${Math.floor(days / 30)} 个月前` : `${Math.floor(days / 365)} 年前`;
}

export const SessionList = memo(function SessionList(props: Props) {
  const {sessions, activeId, view, loading, busy, deleting, menuId, onSelect, onMenu} = props;
  const host = useRef<HTMLElement>(null);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const [viewport, setViewport] = useState({top: 0, height: 600});
  const [focusedKey, setFocusedKey] = useState('');
  const [, tick] = useState(0);
  useEffect(() => { const timer = window.setInterval(() => tick(n => n + 1), 60000); return () => clearInterval(timer); }, []);
  const sorted = useMemo(() => [...sessions].sort((a,b) => b.updatedAt.localeCompare(a.updatedAt)), [sessions]);
  const rows = useMemo(() => {
    const result: Row[] = [];
    let top = 0;
    const addSession = (item: AgentSession) => { result.push({kind:'session', key:item.id, item, top, height:56}); top += 56; };
    if (view === 'session') sorted.forEach(addSession);
    else {
      const groups = new Map<string, {name: string; path: string; items: AgentSession[]}>();
      sorted.forEach(item => {
        const path = item.project.isDefault ? '' : item.project.path;
        const key = path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '');
        const group = groups.get(key) ?? {name:path ? item.project.name : '最新', path, items:[]};
        group.items.push(item); groups.set(key, group);
      });
      const ordered = [...groups.entries()].filter(([key]) => key !== '');
      if (groups.has('')) ordered.push(['', groups.get('')!]);
      ordered.forEach(([key, group]) => {
        result.push({kind:'group', key:'group:'+key, name:group.name, path:group.path, top, height:34}); top += 34;
        if (!collapsed.has(key)) group.items.forEach(addSession);
      });
    }
    return result;
  }, [sorted, view, collapsed]);
  const rowIndices = useMemo(() => new Map(rows.map((row, index) => [row.key, index])), [rows]);
  const height = rows.length ? rows.at(-1)!.top + rows.at(-1)!.height : 0;
  useLayoutEffect(() => {
    const el = host.current;
    if (!el) return;
    const observer = new ResizeObserver(() => setViewport({top:el.scrollTop, height:el.clientHeight}));
    observer.observe(el); return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    const el = host.current;
    const row = activeId ? rows[rowIndices.get(activeId) ?? -1] : undefined;
    if (el && row) {
      if (row.top < el.scrollTop) el.scrollTop = row.top;
      else if (row.top + row.height > el.scrollTop + el.clientHeight) el.scrollTop = row.top + row.height - el.clientHeight;
      setViewport({top:el.scrollTop, height:el.clientHeight});
    }
  }, [activeId, view, sorted]);
  // Keep a focused row mounted when the pointer scrolls it outside the window.
  const firstAt = (offset: number) => {
    let low = 0, high = rows.length;
    while (low < high) { const middle = (low + high) >>> 1; if (rows[middle].top + rows[middle].height < offset) low = middle + 1; else high = middle; }
    return low;
  };
  const start = firstAt(viewport.top - 280);
  const end = Math.min(rows.length, firstAt(viewport.top + viewport.height + 280) + 1);
  const indices = new Set(Array.from({length:end-start}, (_, index) => start+index));
  for (const key of [focusedKey, menuId]) { const index = key ? rowIndices.get(key) : undefined; if (index !== undefined) indices.add(index); }
  const visible = [...indices].sort((a,b) => a-b).map(index => rows[index]);
  return <nav ref={host} className="agent-session-list" aria-label="会话历史" tabIndex={0}
    onScroll={event => setViewport({top:event.currentTarget.scrollTop, height:event.currentTarget.clientHeight})}
    onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setFocusedKey(''); }}
    onKeyDown={event => {
      if (!['ArrowDown','ArrowUp','Home','End'].includes(event.key)) return;
      const index = rowIndices.get(focusedKey) ?? -1;
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? rows.length-1 : Math.max(0, Math.min(rows.length-1, index + (event.key === 'ArrowDown' ? 1 : -1)));
      const row = rows[next]; const el = host.current;
      if (!row || !el) return;
      event.preventDefault();
      el.scrollTop = Math.max(0, row.top - el.clientHeight / 2);
      setViewport({top:el.scrollTop, height:el.clientHeight}); setFocusedKey(row.key);
      requestAnimationFrame(() => el.querySelector<HTMLElement>(`[data-row-index="${next}"] button`)?.focus());
    }}>
    {loading && <p>正在读取…</p>}
    {!loading && !sessions.length && <p>还没有会话</p>}
    <div className="agent-session-window" style={{height}}>{visible.map(row => <div key={row.key} data-row-index={rowIndices.get(row.key)} className="agent-session-window-row" style={{top:row.top, height:row.height}} onFocus={() => setFocusedKey(row.key)}>
      {row.kind === 'group' ? <div className={'agent-session-group' + (!collapsed.has(row.key.slice(6)) ? ' is-open' : '')}>
        <button type="button" className={'agent-group-label' + (!row.path ? ' agent-group-label-recent' : '')} title={row.path || row.name} aria-expanded={!collapsed.has(row.key.slice(6))} onClick={() => setCollapsed(current => {const next = new Set(current); const key = row.key.slice(6); if (next.has(key)) next.delete(key); else next.add(key); return next;})}>
          {row.path && <Folder aria-hidden="true"/>}<span>{row.name}</span>{!row.path && <ChevronDown className="agent-group-chevron" aria-hidden="true"/>}
        </button>
        {row.path && <div className="agent-group-actions"><button type="button" aria-label={'项目操作 '+row.name} aria-haspopup="menu" aria-expanded={menuId === row.key} disabled={busy} onClick={event => props.onProjectMenu(row.path, row.name, event.currentTarget, row.key)}><MoreHorizontal aria-hidden="true"/></button><button type="button" aria-label={'在 '+row.name+' 中新建会话'} title="新建会话" disabled={busy} onClick={() => props.onNewProjectChat(row.path, row.name)}><SquarePen aria-hidden="true"/></button></div>}
      </div> : <div className="agent-session-row" onContextMenu={event => {event.preventDefault(); onMenu(row.item, event.clientX, event.clientY, event.currentTarget.querySelector('button')!);}}>
        <button type="button" disabled={busy} className={'agent-session-select ' + (row.item.id === activeId ? 'is-active' : '')} title={row.item.title} aria-current={row.item.id === activeId ? 'true' : undefined} onClick={() => onSelect(row.item)}>
          <span>{row.item.title}</span><small>{[view === 'session' ? row.item.project.name || '默认工作目录' : '', dateLabel(row.item.updatedAt)].filter(Boolean).join(' · ')}</small>
        </button>
        <button type="button" className="agent-session-delete" disabled={busy || !!deleting} onClick={event => {const rect = event.currentTarget.getBoundingClientRect(); onMenu(row.item, rect.right, rect.bottom+4, event.currentTarget);}} aria-label={'会话操作 '+row.item.title} aria-haspopup="menu" aria-expanded={menuId === row.item.id}><MoreVertical aria-hidden="true"/></button>
      </div>}
    </div>)}</div>
  </nav>;
});
