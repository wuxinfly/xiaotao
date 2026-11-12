import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {api, type AgentModelOption, type AgentProject} from '../../lib/api';
import {useAgentModelSettings} from '../../lib/agent-model-settings';
import {isTerminalTask, officeApi, type OfficeArtifact, type OfficeArtifactDetail, type OfficeTask, type OfficeTaskDetail} from '../../lib/office';
import {toast} from '../../components/toast';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {ModalFrame, ModalPresence} from '../../components/modal/ModalFrame';
import {ArtifactModal} from './ArtifactModal';
import {MemberChat} from './MemberChat';
import {CreateTaskModal} from './CreateTaskModal';
import {OfficeRoom} from './OfficeRoom';
import {OfficeMenu, OfficeMenuButton} from './OfficeMenu';
import {TaskDetail} from './TaskDetail';
import {useOfficePolling} from './useOfficePolling';
import './office.css';

type Props = {visible: boolean};
type Panel = 'menu' | 'task' | 'member' | 'artifact' | 'create' | null;

const projectKey = (path: string) => path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, '');

export function OfficePage({visible}: Props) {
  const confirm = useConfirm();
  const [projects, setProjects] = useState<AgentProject[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(true);
  const [projectsError, setProjectsError] = useState('');
  const [selectedPath, setSelectedPath] = useState('');
  const [models, setModels] = useState<AgentModelOption[]>([]);
  const [detail, setDetail] = useState<OfficeTaskDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const [activeTaskId, setActiveTaskId] = useState('');
  const [panel, setPanel] = useState<Panel>(null);
  const [handoffGoal, setHandoffGoal] = useState('');
  const [activeMemberId, setActiveMemberId] = useState('');
  const [artifact, setArtifact] = useState<OfficeArtifactDetail | null>(null);
  const detailRequest = useRef(0);
  const taskSelection = useRef<{path: string; id: string} | null>(null);
  const artifactRequest = useRef(0);
  const artifactReturn = useRef<Panel>(null);

  const project = useMemo(() => projects.find(item => projectKey(item.path) === projectKey(selectedPath)), [projects, selectedPath]);
  const projectName = project?.name || '';
  const {snapshot, loading, error, lastUpdated, refresh} = useOfficePolling(selectedPath, projectName, visible);
  const member = snapshot?.members.find(item => item.id === activeMemberId);
  const task = snapshot?.tasks.find(item => item.id === activeTaskId);
  const {enabledModels, modelConfigs} = useAgentModelSettings();

  useEffect(() => {
    let live = true;
    api.agentProjects()
      .then(result => {
        if (!live) return;
        const saved = result.projects.filter(item => item.saved || !item.isDefault);
        setProjects(saved);
        setSelectedPath(current => current || saved[0]?.path || '');
      })
      .catch(reason => live && setProjectsError(reason instanceof Error ? reason.message : String(reason)))
      .finally(() => live && setProjectsLoading(false));
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

  function showPanel(next: Panel) {
    artifactRequest.current++;
    setPanel(next);
    if (next !== 'task' && next !== 'artifact') {
      taskSelection.current = null;
      detailRequest.current++;
      setActiveTaskId('');
      setDetail(null);
      setDetailError('');
      setDetailLoading(false);
    }
  }

  function selectProject(path: string) {
    showPanel('menu');
    setSelectedPath(path);
  }

  // The board snapshot changes on every poll; refresh the open task with it
  // instead of running a second timer. Detail is never loaded for other tasks,
  // and a finished task's transcript is not reloaded on every idle poll.
  useEffect(() => {
    if (!visible || !activeTaskId || (panel !== 'task' && panel !== 'artifact')) return;
    const current = snapshot?.tasks.find(item => item.id === activeTaskId);
    if (current && isTerminalTask(current.status) && detail?.task.id === activeTaskId) return;
    void loadDetail(activeTaskId, true);
  }, [visible, activeTaskId, loadDetail, snapshot, detail?.task.id, panel]);

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
    showPanel('task');
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

  async function openArtifact(item: OfficeArtifact) {
    const id = ++artifactRequest.current;
    try {
      const result = await officeApi.artifact(selectedPath, item.id);
      if (id !== artifactRequest.current) return;
      if (!result.available) {
        void refresh();
        if (activeTaskId) void loadDetail(activeTaskId, true);
      }
      artifactReturn.current = panel;
      setArtifact(result);
      showPanel('artifact');
    } catch (reason) {
      if (id === artifactRequest.current) toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }

  function selectMember(id: string) {
    setActiveMemberId(id);
    showPanel('member');
  }

  if (!projectsLoading && !projects.length) {
    return <div className="office-page content-column">
      <div className="office-empty-project">
        <h1>Agent 办公室</h1>
        <p>一个项目对应一间办公室。先返回小涛选择项目目录，或保存一个项目。</p>
        {projectsError && <p className="office-error" role="alert">{projectsError}</p>}
      </div>
    </div>;
  }

  const close = () => showPanel(null);
  const activeCount = snapshot?.tasks.filter(item => !isTerminalTask(item.status)).length ?? 0;
  const visibleArtifacts = snapshot ? new Set(snapshot.artifacts.map(item => item.id)) : null;
  const currentDetail = detail && visibleArtifacts ? {...detail, artifacts: detail.artifacts.filter(item => visibleArtifacts.has(item.id))} : detail;
  return <div className="office-page content-column">
    {error && <p className="office-error office-page-error" role="alert">刷新办公室失败：{error}</p>}
    {snapshot ? <OfficeRoom projectName={snapshot.project.name} members={snapshot.members} artifacts={snapshot.artifacts}
      onSelectTask={selectTask} onSelectMember={selectMember} onOpenArtifact={item => void openArtifact(item)}/>
      : <div className="office-room office-room-skeleton" role="status">{projectsLoading || loading ? '正在打开办公室…' : '暂时无法打开办公室，请在菜单中刷新。'}</div>}
    {!projectsLoading && <OfficeMenuButton count={activeCount} onOpen={() => showPanel('menu')}/>}
    <ModalPresence>
      {panel === 'menu' ? <OfficeMenu projects={projects} selectedPath={selectedPath} snapshot={snapshot} lastUpdated={lastUpdated}
        loading={loading} onChange={selectProject} onRefresh={() => void refresh()} onTask={selectTask} onClose={close}/>
      : panel === 'member' && member ? <ModalFrame title={`与${member.name}对话`} description={`${projectName} · 办公室对话`}
        className="office-chat-modal" onClose={close} footer={null}>
        <MemberChat key={member.id} projectPath={selectedPath} projectName={projectName} member={member} model={preferredModel}
          models={models} modelConfigs={modelConfigs} onHandoff={goal => {setHandoffGoal(goal); showPanel('create');}}/>
      </ModalFrame>
      : panel === 'task' ? <ModalFrame title="任务详情" className="office-task-modal" onClose={close}
        footer={<button type="button" className="office-ghost" onClick={() => showPanel('menu')}>查看全部任务</button>}>
        <TaskDetail key={activeTaskId} projectPath={selectedPath} detail={currentDetail} loading={detailLoading} error={detailError}
          onRefresh={() => void loadDetail(activeTaskId)} onLoadEarlier={() => void loadEarlier()} onOpenArtifact={item => void openArtifact(item)}
          onCancel={() => void cancelTask()} onResolveApproval={(id, approved) => void resolveApproval(id, approved)} onPostMessage={postMessage}/>
        {!detail && task && <p className="office-muted">{task.title}</p>}
      </ModalFrame>
      : panel === 'create' ? <CreateTaskModal projectPath={selectedPath} projectName={projectName} models={models}
        modelConfigs={modelConfigs} preferredModel={preferredModel} initialGoal={handoffGoal}
        onCreated={(created: OfficeTask) => {toast.success('任务已安排，执行者开始工作'); void refresh(); selectTask(created.id);}}
        onClose={() => showPanel('member')}/>
      : panel === 'artifact' && artifact ? <ArtifactModal detail={artifact} onClose={() => showPanel(artifactReturn.current)}/>
      : null}
    </ModalPresence>
  </div>;
}
