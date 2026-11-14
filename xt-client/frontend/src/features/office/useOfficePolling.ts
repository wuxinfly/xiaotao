import {useCallback, useEffect, useRef, useState} from 'react';
import {officeApi, type OfficeSnapshot} from '../../lib/office';

type PollState = {
  projectPath: string;
  snapshot: OfficeSnapshot | null;
  loading: boolean;
  error: string;
  lastUpdated: number;
};

const ACTIVE_INTERVAL = 2500;
const IDLE_INTERVAL = 12000;

// useOfficePolling refreshes the board with chained timeouts so requests never
// overlap. It polls fast only while a task is running, pauses when the page is
// hidden, and stops entirely when the office page is not visible.
export function useOfficePolling(projectPath: string, projectName: string, visible: boolean) {
  const [state, setState] = useState<PollState>({projectPath, snapshot: null, loading: !!projectPath, error: '', lastUpdated: 0});
  const sequence = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const controller = useRef<AbortController | null>(null);
  const currentProject = useRef(projectPath);
  currentProject.current = projectPath;

  const load = useCallback(async (showLoading: boolean) => {
    if (!projectPath || currentProject.current !== projectPath) return;
    const id = ++sequence.current;
    controller.current?.abort();
    const next = new AbortController();
    controller.current = next;
    if (showLoading) setState(current => ({...current, loading: true}));
    try {
      const snapshot = await officeApi.snapshot(projectPath, projectName, next.signal);
      if (id !== sequence.current) return;
      setState({projectPath, snapshot, loading: false, error: '', lastUpdated: Date.now()});
    } catch (reason) {
      if (id !== sequence.current) return;
      if (next.signal.aborted) return;
      setState(current => ({...current, loading: false, error: reason instanceof Error ? reason.message : String(reason)}));
    }
  }, [projectPath, projectName]);

  useEffect(() => {
    sequence.current++;
    controller.current?.abort();
    setState({projectPath, snapshot: null, loading: !!projectPath, error: '', lastUpdated: 0});
    if (!projectPath) return;
    void load(true);
    return () => {controller.current?.abort();};
  }, [projectPath, load]);

  useEffect(() => {
    window.clearTimeout(timer.current);
    if (!visible || !projectPath) return;
    let disposed = false;
    const schedule = () => {
      const active = !!state.snapshot?.tasks.some(task => task.status === 'running' || task.status === 'waiting' || task.status === 'queued');
      timer.current = setTimeout(async () => {
        if (disposed) return;
        if (document.visibilityState === 'hidden') { schedule(); return; }
        await load(false);
        if (!disposed) schedule();
      }, active ? ACTIVE_INTERVAL : IDLE_INTERVAL);
    };
    schedule();
    const onVisible = () => { if (document.visibilityState === 'visible' && !disposed) void load(false); };
    document.addEventListener('visibilitychange', onVisible);
    return () => {disposed = true; window.clearTimeout(timer.current); document.removeEventListener('visibilitychange', onVisible);};
  }, [visible, projectPath, load, state.snapshot]);

  const refresh = useCallback(() => load(true), [load]);

  return state.projectPath === projectPath ? {...state, refresh}
    : {snapshot: null, loading: !!projectPath, error: '', lastUpdated: 0, refresh};
}
