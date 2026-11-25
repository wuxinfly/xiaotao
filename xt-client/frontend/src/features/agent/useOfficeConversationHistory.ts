import {useEffect, useRef, useState} from 'react';
import type {AgentEvent, AgentSession} from '../../lib/api';
import {ApiError} from '../../lib/http';
import {officeApi, type OfficeHistory} from '../../lib/office';
import {officeConversations} from '../../lib/office-conversations';
import {applyEvent, mergeHistory, mergeRunStats, readEventStream} from './messages/conversationEvents';

type Options = {
  parent: string; project: string; projectName: string; memberId: string;
  historicalTask?: string; visible: boolean; historyKey?: string; status?: string;
  busy: boolean; onError: (message: string) => void;
};

function startSession(event: AgentEvent, project: string, projectName: string): AgentSession {
  const date = event.message?.createdAt || new Date().toISOString();
  return {
    id: event.sessionId, project: {path: project, name: projectName}, title: '', model: '', status: 'running',
    messages: [], activity: [], thoughts: [], runStats: [], createdAt: date, updatedAt: date,
  };
}

function mergePage(current: OfficeHistory | null, page: OfficeHistory): OfficeHistory {
  const session = page.session;
  if (current?.session.id !== session.id) return {...page, session: {...session, streamDrafts: page.streamDrafts}};
  const recent = current.session;
  const runStats = mergeRunStats(session.runStats, recent.runStats);
  const messages = mergeHistory(session.messages, recent.messages);
  const streamDrafts = mergeHistory(page.streamDrafts, recent.streamDrafts)
    .filter(draft => !runStats.some(run => run.runId === draft.runId && run.endedAt) &&
      !messages.some(message => message.role === 'assistant' && !message.toolCalls?.length && message.runId === draft.runId));
  return {...page, session: {
    ...session, messages, runStats, streamDrafts,
    activity: mergeHistory(session.activity, recent.activity), thoughts: mergeHistory(session.thoughts, recent.thoughts),
  }};
}

export function useOfficeConversationHistory({parent, project, projectName, memberId, historicalTask, visible, historyKey, status, busy, onError}: Options) {
  const [history, setHistory] = useState<OfficeHistory | null>(null);
  const [connected, setConnected] = useState(false);
  const connecting = useRef<Promise<void> | undefined>(undefined);
  const revision = useRef(0);

  useEffect(() => {
    if (!parent || !visible || historicalTask) return;
    const controller = new AbortController();
    let retry = 0;
    async function connect() {
      let notify = () => {};
      connecting.current = new Promise(resolve => {notify = resolve;});
      let retryAfter = true;
      try {
        const response = await officeConversations.events(parent, project, memberId, controller.signal);
        if (!response.ok || !response.headers.get('content-type')?.includes('text/event-stream')) {
          retryAfter = false;
          return;
        }
        if (controller.signal.aborted) return;
        setConnected(true);
        notify();
        await readEventStream(response, event => {
          if (controller.signal.aborted || !event.sessionId) return;
          revision.current++;
          setHistory(current => {
            const same = current?.session.id === event.sessionId;
            const session = same ? current.session : startSession(event, project, projectName);
            return {before: same ? current.before : '', session: applyEvent(session, event)};
          });
        });
      } catch {
        // Keep polling durable history while reconnecting.
      } finally {
        notify();
        if (!controller.signal.aborted) {
          setConnected(false);
          if (retryAfter) retry = window.setTimeout(connect, 1200);
        }
      }
    }
    void connect();
    return () => {controller.abort(); window.clearTimeout(retry);};
  }, [parent, project, projectName, memberId, historicalTask, visible]);

  useEffect(() => {
    if (!parent || !visible || !historyKey && !historicalTask) return;
    const controller = new AbortController();
    let timer = 0;
    async function poll() {
      const started = revision.current;
      try {
        const page = historicalTask ? await officeApi.taskHistory(project, historicalTask) : await officeConversations.history(parent, project, memberId, '', controller.signal);
        if (!controller.signal.aborted) setHistory(current => {
          if (revision.current !== started && current?.session.id !== page.session.id) return current;
          const merged = mergePage(current, page);
          if (revision.current !== started && current) merged.session.status = current.session.status;
          return merged;
        });
      } catch (reason) {
        if (!controller.signal.aborted && !(reason instanceof ApiError && reason.status === 404)) onError(String(reason));
      }
      if (!controller.signal.aborted && busy) timer = window.setTimeout(poll, connected ? 1800 : 500);
    }
    void poll();
    return () => {controller.abort(); window.clearTimeout(timer);};
  }, [parent, project, memberId, historicalTask, visible, historyKey, status, busy, connected, onError]);

  async function waitForStream() {
    let timer = 0;
    try {
      await Promise.race([connecting.current, new Promise(resolve => {timer = window.setTimeout(resolve, 600);})]);
    } finally {
      window.clearTimeout(timer);
    }
  }

  return {history, waitForStream, connected};
}
