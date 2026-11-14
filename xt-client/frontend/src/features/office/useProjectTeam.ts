import {useCallback, useEffect, useRef, useState} from 'react';
import {teamApi, type TeamSnapshot} from '../../lib/team';

export function useProjectTeam(projectPath: string) {
  const [state, setState] = useState<{path: string; team: TeamSnapshot | null; error: string}>({path: '', team: null, error: ''});
  const [attempt, setAttempt] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    if (!projectPath) return;
    const controller = new AbortController();
    const id = ++generation.current;
    setState({path: projectPath, team: null, error: ''});
    teamApi.snapshot(projectPath, controller.signal).then(team => {
      if (controller.signal.aborted || generation.current !== id) return;
      setState({path: projectPath, team, error: ''});
    }).catch(reason => {
      if (!controller.signal.aborted && generation.current === id) setState({path: projectPath, team: null, error: reason instanceof Error ? reason.message : String(reason)});
    });
    return () => {controller.abort();};
  }, [projectPath, attempt]);
  const setTeam = useCallback((team: TeamSnapshot) => {
    generation.current++;
    setState(current => current.path === projectPath ? {path: projectPath, team, error: ''} : current);
  }, [projectPath]);
  const current = state.path === projectPath ? state : {team: null, error: ''};
  return {...current, retry: () => setAttempt(value => value + 1), setTeam};
}
