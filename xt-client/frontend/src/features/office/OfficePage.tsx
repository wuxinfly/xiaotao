import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {api, type AgentModelOption} from '../../lib/api';
import {useAgentModelSettings} from '../../lib/agent-model-settings';
import {isTerminalTask, officeApi, type OfficeArtifact, type OfficeArtifactDetail, type OfficeMember, type OfficeTask, type OfficeTaskDetail} from '../../lib/office';
import {toast} from '../../components/toast';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {ModalPresence} from '../../components/modal/ModalFrame';
import {ArtifactModal} from './ArtifactModal';
import {ArtifactLibraryModal} from './ArtifactLibraryModal';
import {CreateTaskModal} from './CreateTaskModal';
import {OfficeEntryFlow, type OfficeEntryHandle} from './OfficeEntryFlow';
import type {OfficeConversation} from '../../lib/office-conversations';
import {OfficeWorkspace} from './OfficeWorkspace';
import {useOfficePolling} from './useOfficePolling';
import {TeamManager} from './TeamManager';
import './team.css';
import './office.css';
import './office-workspace.css';

type Props = {
  visible: boolean;
  projectPath: string;
  projectName?: string;
  conversations: OfficeConversation[];
  coordinatorRunning: boolean;
  onOpenMember: (member: OfficeMember) => void;
  onOpenConversation: (item: OfficeConversation) => void;
};
type Panel = 'library' | 'create' | 'team' | null;
type WorkspaceView = 'tasks' | 'team' | 'task' | null;

export function OfficePage(props: Props) {
  if (!props.projectPath) {
    return <div className="office-empty-page">
      <div className="office-empty-project">
        <h2>办公室</h2>
        <p>在小涛对话里选择项目后，就能查看它的办公室。</p>
      </div>
    </div>;
  }
  return <ProjectOffice key={props.projectPath} {...props}/>;
}

function ProjectOffice({visible, projectPath: selectedPath, projectName, conversations, coordinatorRunning, onOpenMember, onOpenConversation}: Props) {
  const confirm = useConfirm();
  const name = projectName || selectedPath.split(/[\\/]/).filter(Boolean).at(-1) || selectedPath;
  const [models, setModels] = useState<AgentModelOption[]>([]);
  const [detail, setDetail] = useState<OfficeTaskDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const [activeTaskId, setActiveTaskId] = useState('');
  const [panel, setPanel] = useState<Panel>(null);
  const [workspaceView, setWorkspaceView] = useState<WorkspaceView>(null);
  const [artifact, setArtifact] = useState<{id: number; detail: OfficeArtifactDetail; trigger: HTMLElement | null} | null>(null);
  const detailRequest = useRef(0);
  const taskSelection = useRef<{path: string; id: string} | null>(null);
  const artifactRequest = useRef(0);
  const entry = useRef<OfficeEntryHandle>(null);
  const {snapshot, loading, error, refresh} = useOfficePolling(selectedPath, name, visible);
  const {enabledModels, modelConfigs} = useAgentModelSettings();

  useEffect(() => {
    let live = true;
    api.agentModels().then(result => { if (live) setModels(result.models); }).catch(() => {});
    return () => {live = false;};
  }, []);

  const loadDetail = useCallback(async (taskId: string, silent = false) => {
    const selected = taskSelection.current;
    if (!selected || selected.path !== selectedPath || selected.id !== taskId) return;
    const id = ++detailRequest.current;
    if (!silent) setDetailLoading(true);
    try {
      const next = await officeApi.taskDetail(selectedPath, taskId);
      if (id === detailRequest.current) { setDetail(next); setDetailError(''); }
    } catch (reason) {
      if (id === detailRequest.current) setDetailError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      if (id === detailRequest.current) setDetailLoading(false);
    }
  }, [selectedPath]);

  function clearTask() {
    taskSelection.current = null;
    detailRequest.current++;
    setActiveTaskId('');
    setDetail(null);
    setDetailError('');
    setDetailLoading(false);
  }

  function showPanel(next: Panel) {
    artifactRequest.current++;
    setArtifact(null);
    setPanel(next);
  }

  // The board snapshot changes on every poll; refresh the open task with it
  // instead of running a second timer. Detail is never loaded for other tasks,
  // and a finished task's transcript is not reloaded on every idle poll.
  useEffect(() => {
    if (!visible || !activeTaskId || workspaceView !== 'task') return;
    const current = snapshot?.tasks.find(item => item.id === activeTaskId);
    if (current && isTerminalTask(current.status) && detail?.task.id === activeTaskId
      && detail.task.status === current.status && detail.task.updatedAt === current.updatedAt) return;
    void loadDetail(activeTaskId, true);
  }, [visible, activeTaskId, loadDetail, snapshot, detail?.task.id, detail?.task.status, detail?.task.updatedAt, workspaceView]);

  const loadEarlier = useCallback(async () => {
    if (!selectedPath || !activeTaskId || !detail?.history?.before) return;
    try {
      const page = await officeApi.taskHistory(selectedPath, activeTaskId, detail.history.before);
      setDetail(current => {
        if (!current?.history || current.task.id !== activeTaskId) return current;
        const session = current.history.session;
        const older = page.session;
        return {...current, history: {
          before: page.before,
          session: {...session,
            messages: [...older.messages, ...session.messages],
            activity: [...older.activity, ...session.activity],
            thoughts: [...older.thoughts, ...session.thoughts]},
        }};
      });
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }, [selectedPath, activeTaskId, detail?.history?.before]);

  const preferredModel = useMemo(() =>
    models.find(item => enabledModels.includes(item.model) && !item.unavailable)?.model
    || models.find(item => !item.unavailable)?.model || '', [models, enabledModels]);

  function selectTask(taskId: string) {
    showPanel(null);
    setWorkspaceView('task');
    taskSelection.current = {path: selectedPath, id: taskId};
    if (taskId !== activeTaskId) {
      // Never show the previous task's transcript under the new title while
      // its detail request is still in flight.
      setDetail(current => current?.task.id === taskId ? current : null);
      setDetailError('');
    }
    setActiveTaskId(taskId);
    void loadDetail(taskId);
  }

  async function cancelTask() {
    if (!activeTaskId) return;
    const ok = await confirm({title: '停止这个任务？', description: '执行者会尽快停下，已经写入的文件和记录会保留。', confirmLabel: '停止任务'});
    if (!ok) return;
    try {
      await officeApi.cancelTask(selectedPath, activeTaskId);
      toast.info('已请求停止任务');
      refresh();
      void loadDetail(activeTaskId);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }

  async function resolveApproval(approvalId: string, approved: boolean) {
    try {
      await officeApi.resolveApproval(selectedPath, activeTaskId, approvalId, approved);
      toast.success(approved ? '已允许执行' : '已拒绝执行');
      void loadDetail(activeTaskId);
      refresh();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }

  async function postMessage(text: string) {
    await officeApi.postMessage(selectedPath, activeTaskId, text);
    void loadDetail(activeTaskId);
    refresh();
  }

  async function openArtifact(item: OfficeArtifact, trigger = document.activeElement) {
    const id = ++artifactRequest.current;
    try {
      const result = await officeApi.artifact(selectedPath, item.id);
      if (id !== artifactRequest.current) return;
      if (!result.available) {
        void refresh();
        if (activeTaskId) void loadDetail(activeTaskId, true);
      }
      setArtifact({id, detail: result, trigger: trigger instanceof HTMLElement ? trigger : null});
    } catch (reason) {
      if (id === artifactRequest.current) toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }

  function selectMember(id: string) {
    const member = snapshot?.members.find(item => item.id === id);
    if (!member) return;
    if (member.role === 'executor' && member.taskId) selectTask(member.taskId);
    else onOpenMember(member);
  }

  const close = () => showPanel(null);
  const visibleArtifacts = snapshot ? new Set(snapshot.artifacts.map(item => item.id)) : null;
  const currentDetail = detail && visibleArtifacts ? {...detail, artifacts: detail.artifacts.filter(item => visibleArtifacts.has(item.id))} : detail;
  return <div className="office-page office-live-page">
    <OfficeWorkspace projectPath={selectedPath} projectName={name} visible={visible} snapshot={snapshot} loading={loading} error={error}
      view={workspaceView} activeTaskId={activeTaskId} detail={currentDetail} detailLoading={detailLoading} detailError={detailError}
      onView={next => {setWorkspaceView(next); clearTask();}}
      onCloseDrawer={() => {clearTask(); setWorkspaceView(null);}}
      onRefresh={() => void refresh()}
      onCreate={() => showPanel('create')} onManage={() => showPanel('team')} onSelectTask={selectTask} onSelectMember={selectMember}
      onCloseDetail={() => {clearTask(); setWorkspaceView('tasks');}}
      onRefreshDetail={() => void loadDetail(activeTaskId)} onLoadEarlier={() => void loadEarlier()} onCancelTask={() => void cancelTask()}
      onResolveApproval={(id, approved) => void resolveApproval(id, approved)} onPostMessage={postMessage}
      onOpenArtifact={item => void openArtifact(item)}
      onOpenLibrary={() => showPanel('library')}>
      <OfficeEntryFlow ref={entry} projectPath={selectedPath} projectName={name} members={snapshot?.members || []} tasks={snapshot?.tasks || []}
        conversations={conversations} coordinatorRunning={coordinatorRunning} loading={loading} error={error} models={models}
        onTeamChanged={() => void refresh()} onSelectMember={selectMember} onSelectTask={selectTask} onSelectConversation={onOpenConversation}/>
    </OfficeWorkspace>
    <ModalPresence>
      {panel === 'team' ? <TeamManager projectPath={selectedPath} models={models} onChanged={async team => {
        entry.current?.setTeam(team);
        await refresh();
      }} onClose={close}/>
      : panel === 'create' ? <CreateTaskModal projectPath={selectedPath} projectName={name} models={models}
        modelConfigs={modelConfigs} preferredModel={preferredModel} members={snapshot?.members || []} accessMode={snapshot?.accessMode || 'safe'}
        onCreated={(created: OfficeTask) => {toast.success(`任务已交给${created.executor.name}`); void refresh(); selectTask(created.id);}}
        onClose={close}/>
      : panel === 'library' ? <ArtifactLibraryModal artifacts={snapshot?.artifacts || []} covered={!!artifact} onOpenArtifact={(item, trigger) => void openArtifact(item, trigger)} onClose={close}/>
      : null}
    </ModalPresence>
    <ModalPresence>{artifact && <ArtifactModal key={artifact.id} detail={artifact.detail} stacked={panel === 'library'} returnFocus={artifact.trigger} onClose={() => {artifactRequest.current++; setArtifact(null);}}/>}</ModalPresence>
  </div>;
}
