import {useCallback, useEffect, useRef, useState} from 'react';
import {teamApi, type TeamSnapshot} from '../../lib/team';

export type EntryPhase = 'loading' | 'onboarding' | 'empty' | 'preparing' | 'ready' | 'error';

export function useProjectTeam(projectPath: string, visible: boolean) {
  const [state, setState] = useState<{path: string; team: TeamSnapshot | null; phase: EntryPhase; error: string}>({path: '', team: null, phase: 'loading', error: ''});
  const generation = useRef(0);
  const load = useCallback(async (path: string, prepare = false) => {
    const id = ++generation.current;
    setState({path, team: null, phase: 'loading', error: ''});
    try {
      const team = await teamApi.snapshot(path);
      if (generation.current !== id) return null;
      const phase: EntryPhase = !team.members.length && team.onboarding === 'pending' ? 'onboarding' : prepare || team.members.length ? 'preparing' : 'empty';
      setState({path, team, phase, error: ''});
      return team;
    } catch (reason) {
      if (generation.current === id) setState({path, team: null, phase: 'error', error: reason instanceof Error ? reason.message : String(reason)});
      return null;
    }
  }, []);
  useEffect(() => {
    if (!projectPath) return;
    const controller = new AbortController();
    const id = ++generation.current;
    const requestGeneration = id;
    setState({path: projectPath, team: null, phase: 'loading', error: ''});
    teamApi.snapshot(projectPath, controller.signal).then(team => {
      if (generation.current !== id) return;
      const phase: EntryPhase = !team.members.length && team.onboarding === 'pending' ? 'onboarding' : team.members.length ? 'preparing' : 'empty';
      setState({path: projectPath, team, phase, error: ''});
    }).catch(reason => {
      if (!controller.signal.aborted && generation.current === id) setState({path: projectPath, team: null, phase: 'error', error: reason instanceof Error ? reason.message : String(reason)});
    });
    return () => {controller.abort(); if (generation.current === requestGeneration) generation.current = requestGeneration + 1;};
  }, [projectPath]);
  const setPhase = (phase: EntryPhase, error = '') => setState(current => current.path === projectPath && (current.phase !== 'error' || phase === 'error') ? {...current, phase, error} : current);
  const setTeam = (team: TeamSnapshot) => setState(current => current.path === projectPath ? {...current, team, phase: 'preparing', error: ''} : current);
  const current = state.path === projectPath ? state : {path: projectPath, team: null, phase: 'loading' as EntryPhase, error: ''};
  return {...current, visible, reload: () => load(projectPath), prepare: () => load(projectPath, true), setPhase, setTeam};
}
