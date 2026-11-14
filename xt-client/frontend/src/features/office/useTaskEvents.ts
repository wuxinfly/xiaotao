import {useEffect, useRef, useState} from 'react';
import type {AgentEvent, AgentSession} from '../../lib/api';
import {applyEvent, mergeHistory, mergeRunStats, readEventStream} from '../agent/messages/conversationEvents';
import {isTerminalTask, type OfficeTaskDetail} from '../../lib/office';

function mergeSession(history: AgentSession, live: AgentSession | null): AgentSession {
  if (!live) return history;
  return {...history,
    streamDrafts: live.streamDrafts,
    messages: mergeHistory(history.messages, live.messages),
    activity: mergeHistory(history.activity, live.activity),
    thoughts: mergeHistory(history.thoughts, live.thoughts),
    runStats: mergeRunStats(history.runStats, live.runStats),
  };
}

export function useTaskEvents(projectPath: string, detail: OfficeTaskDetail | null, onUpdate: () => void) {
  const task = detail?.task;
  const [session, setSession] = useState<AgentSession | null>(null);
  const live = useRef<AgentSession | null>(null);
  const taskID = task?.id || '';
  const active = !!task && !isTerminalTask(task.status);
  const history = detail?.history?.session;
  const initial = useRef<AgentSession | null>(null);
  initial.current = history || null;
  const update = useRef(onUpdate);
  update.current = onUpdate;
  useEffect(() => {
    if (!history) return;
    setSession(mergeSession(history, live.current));
  }, [history]);
  useEffect(() => {
    live.current = null;
    setSession(null);
    if (!active || !taskID) return;
    const controller = new AbortController();
    let disposed = false;
    const listen = async () => {
      try {
        const response = await fetch(`/api/office/tasks/${encodeURIComponent(taskID)}/events?projectPath=${encodeURIComponent(projectPath)}`, {signal: controller.signal, headers: {Accept: 'text/event-stream'}});
        if (!response.ok) return;
        await readEventStream(response, (event: AgentEvent) => {
          if (disposed) return;
          if (live.current) live.current = applyEvent(live.current, event);
          else if (initial.current) live.current = applyEvent(initial.current, event);
          if (live.current) setSession(live.current);
          if (event.type === 'status' || event.type === 'error' || event.type === 'approval' || (event.type === 'activity' && event.activity?.status === 'completed')) update.current();
        });
      } catch {
        // Snapshot polling and durable history remain authoritative on disconnect.
      }
    };
    void listen();
    return () => {disposed = true; controller.abort();};
  }, [projectPath, taskID, active]);
  return session || history || null;
}
