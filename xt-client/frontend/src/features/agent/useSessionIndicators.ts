import {useEffect, useRef, useState} from 'react';
import type {AgentSession} from '../../lib/api';

type RecordEntry = {status: AgentSession['status']; unread?: 'completed' | 'failed'};
const storageKey = 'xiaotao.agent.sessionIndicators';

function load(): Record<string, RecordEntry> {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) || '{}');
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
    return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, RecordEntry] => {
      const item = entry[1] as RecordEntry | null;
      return !!item && ['idle', 'running', 'completed', 'failed', 'canceled'].includes(item.status)
        && (item.unread === undefined || item.unread === 'completed' || item.unread === 'failed');
    }));
  } catch { return {}; }
}

export function useSessionIndicators(sessions: AgentSession[], activeId: string | undefined, loading: boolean) {
  const records = useRef<Record<string, RecordEntry> | null>(null);
  if (records.current === null) records.current = load();
  const [unread, setUnread] = useState<Record<string, 'completed' | 'failed'>>({});
  useEffect(() => {
    if (loading) return;
    const next: Record<string, RecordEntry> = {};
    const indicators: Record<string, 'completed' | 'failed'> = {};
    for (const session of sessions) {
      const previous = records.current?.[session.id];
      let marker = previous?.unread;
      if (previous?.status === 'running' && (session.status === 'completed' || session.status === 'failed')) marker = session.status;
      if (session.id === activeId || session.status === 'running' || session.status === 'canceled') marker = undefined;
      next[session.id] = {status: session.status, ...(marker ? {unread: marker} : {})};
      if (marker) indicators[session.id] = marker;
    }
    records.current = next;
    setUnread(indicators);
    try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* Keep indicators in memory when storage is unavailable. */ }
  }, [sessions, activeId, loading]);
  return unread;
}
