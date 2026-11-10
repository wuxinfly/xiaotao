import {useCallback, type Dispatch, type MutableRefObject, type SetStateAction} from 'react';
import {api, type AgentSession} from '../../lib/api';
import {toast} from '../../components/toast';

type SelectSession = (session: AgentSession) => Promise<void>;

/** Creates a branch for one answer, adds it to the sidebar and switches to it. */
export function useBranchSession(
  setSessions: Dispatch<SetStateAction<AgentSession[]>>,
  activeIdRef: MutableRefObject<string | undefined>,
  selectSession: SelectSession,
) {
  return useCallback(async (messageId: string) => {
    const source = activeIdRef.current;
    if (!source) return;
    try {
      const created = await api.branchAgentSession(source, messageId);
      setSessions(current => [{...created, messages: [], activity: [], thoughts: [], runStats: []}, ...current.filter(item => item.id !== created.id)]);
      await selectSession(created);
      toast.success('已创建分支');
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : String(reason), {duration: 4000});
    }
  }, [setSessions, activeIdRef, selectSession]);
}