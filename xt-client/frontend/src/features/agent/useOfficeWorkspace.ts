import {useCallback, useEffect, useRef, useState} from 'react';
import {toast} from '../../components/toast';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {ApiError} from '../../lib/http';
import {officeApi, type OfficeMember} from '../../lib/office';
import {officeConversations, type OfficeConversation} from '../../lib/office-conversations';

const officeTab = 'office';
const errorMessage = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);
const working = (item: OfficeConversation) => item.status === 'running' || item.status === 'queued' || item.status === 'waiting';
type MemberTab = {memberId: string; name: string; taskId?: string; hidden?: boolean};
type TabState = {members: Map<string, MemberTab>; selected: string};
const emptyTabs: TabState = {members: new Map(), selected: officeTab};
const emptyConversations: OfficeConversation[] = [];

export function useOfficeWorkspace({parent, project, projectName, visible, pickingRole, paneOpen, onOpen}: {
  parent: string; project: string; projectName: string; visible: boolean; pickingRole: boolean; paneOpen: boolean; onOpen: (open: boolean) => void;
}) {
  const confirm = useConfirm();
  const [tabs, setTabs] = useState<Record<string, TabState>>({});
  const [members, setMembers] = useState<{project: string; items: OfficeMember[]}>({project: '', items: []});
  const [state, setState] = useState<{parent: string; items: OfficeConversation[]; error?: string}>({parent: '', items: []});
  const [acting, setActing] = useState('');
  const [openVersion, setOpenVersion] = useState(0);
  const notified = useRef(new Set<string>());
  const parentRef = useRef(parent);
  parentRef.current = parent;
  const currentTabs = tabs[parent] || emptyTabs;
  const conversations = state.parent === parent ? state.items : emptyConversations;

  const refresh = useCallback(async (id = parent, signal?: AbortSignal) => {
    if (!id || !project) return;
    try {
      const items = await officeConversations.list(id, signal);
      if (!signal?.aborted && parentRef.current === id) setState({parent: id, items: Array.isArray(items) ? items : []});
    } catch (reason) {
      if (!signal?.aborted && parentRef.current === id) setState(current => ({
        parent: id, items: current.parent === id ? current.items : [], error: errorMessage(reason),
      }));
    }
  }, [parent, project]);

  useEffect(() => {
    if (!project || !visible) return;
    const controller = new AbortController();
    officeApi.snapshot(project, projectName, controller.signal)
      .then(value => { if (!controller.signal.aborted) setMembers({project, items: value.members.filter(member => member.role !== 'coordinator' && member.role !== 'executor')}); })
      .catch(() => { if (!controller.signal.aborted) setMembers({project, items: []}); });
    return () => controller.abort();
  }, [project, projectName, visible, pickingRole]);

  useEffect(() => {
    if (!parent || !project || !visible) return;
    const controller = new AbortController();
    let timer = 0;
    const poll = async () => {
      await refresh(parent, controller.signal);
      if (!controller.signal.aborted) timer = window.setTimeout(poll, 1350);
    };
    void poll();
    return () => {controller.abort(); window.clearTimeout(timer);};
  }, [parent, project, refresh, visible]);

  const openMember = useCallback((memberId: string, memberName: string, id = parent, taskId?: string) => {
    if (!id || parentRef.current && id !== parentRef.current) return;
    const key = taskId ? memberId + ':' + taskId : memberId;
    setOpenVersion(current => current + 1);
    setTabs(current => {
      const next = current[id] || emptyTabs;
      return {...current, [id]: {members: new Map([...next.members, [key, {memberId, name: memberName, taskId}]]), selected: key}};
    });
    onOpen(true);
  }, [parent, onOpen]);

  useEffect(() => {
    if (!visible) return;
    for (const item of conversations) {
      const question = item.question || item.coordinatorQuestion;
      if (!question || item.historical) continue;
      const key = JSON.stringify([parent, item.memberId, item.taskId, question]);
      if (notified.current.has(key)) continue;
      notified.current.add(key);
      if (!paneOpen || currentTabs.selected !== item.memberId) {
        const name = item.memberName || '员工';
        toast.info(`${name}等你回复`, {placement: 'bottom-right', duration: 4500, action: {label: '查看', onClick: () => openMember(item.memberId, name)}});
      }
    }
  }, [parent, currentTabs.selected, conversations, visible, paneOpen, openMember]);
  const selectTab = (selected: string) => setTabs(current => ({...current, [parent]: {...currentTabs, selected}}));
  const closeTab = (memberId: string) => setTabs(current => {
    const next = current[parent] || emptyTabs;
    const remaining = new Map(next.members);
    const tab = remaining.get(memberId);
    if (tab) remaining.set(memberId, {...tab, hidden: true});
    return {...current, [parent]: {members: remaining, selected: next.selected === memberId ? officeTab : next.selected}};
  });

  const act = async (item: OfficeConversation, work: () => Promise<unknown>) => {
    setActing(item.memberId);
    try {await work(); await refresh();}
    catch (reason) {toast.error(errorMessage(reason));}
    finally {setActing('');}
  };

  const forget = (id: string) => {
    setTabs(current => {
      const next = {...current};
      delete next[id];
      return next;
    });
    if (parentRef.current === id) setState({parent: id, items: []});
  };

  const close = async (id: string, remove = false) => {
    const options: {force?: boolean; discardSummary?: boolean} = {};
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        await (remove ? officeConversations.remove(id, options) : officeConversations.close(id, options));
        forget(id);
        return true;
      } catch (reason) {
        if (!(reason instanceof ApiError) || reason.status !== 409) throw reason;
        const items = await officeConversations.list(id);
        if (!options.force && items.some(working)) {
          if (!await confirm({title: '停止员工并关闭会话？', description: '该会话还有员工正在执行或等待回复。停止后将清理员工对话原文。', confirmLabel: '停止并关闭'})) return false;
          options.force = true;
        } else if (!options.discardSummary && items.some(item => item.summaryStatus === 'failed')) {
          if (!await confirm({title: '放弃未保存的总结？', description: '可以在员工对话里重试保存。确认后将清理原文，已存的长期记忆会保留。', confirmLabel: '放弃并关闭'})) return false;
          options.discardSummary = true;
        } else throw reason;
      }
    }
    return false;
  };

  return {
    members: members.project === project ? members.items : [], openVersion,
    conversations, error: state.parent === parent ? state.error : undefined,
    tabs: currentTabs.members, selected: currentTabs.selected, acting,
    openMember, selectTab, closeTab, refresh, close,
    delegate: (item: OfficeConversation) => act(item, () => officeConversations.reply(parent, item.memberId, {delegate: true})),
  };
}

export type OfficeWorkspaceState = ReturnType<typeof useOfficeWorkspace>;
