import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Plus, RefreshCw} from 'lucide-react';
import {api, type AgentModelOption, type AgentProject} from '../../lib/api';
import {useAgentModelSettings} from '../../lib/agent-model-settings';
import {isTerminalTask, officeApi, type OfficeArtifact, type OfficeArtifactDetail, type OfficeTask, type OfficeTaskDetail} from '../../lib/office';
import {toast} from '../../components/toast';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {ModalPresence} from '../../components/modal/ModalFrame';
import {ArtifactModal} from './ArtifactModal';
import {MemberChat} from './MemberChat';
import {CreateTaskModal} from './CreateTaskModal';
import {OfficeRoom, TaskCard} from './OfficeRoom';
import {TaskDetail} from './TaskDetail';
import {useOfficePolling} from './useOfficePolling';
import './office.css';

type Props = {visible: boolean};

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
  const [creating, setCreating] = useState(false);
  const [handoffGoal, setHandoffGoal] = useState('');
  const [activeMemberId, setActiveMemberId] = useState('');
  const [artifact, setArtifact] = useState<OfficeArtifactDetail | null>(null);
  const detailRequest = useRef(0);

  const project = useMemo(() => projects.find(item => projectKey(item.path) === projectKey(selectedPath)), [projects, selectedPath]);
  const projectName = project?.name || '';
  const {snapshot, loading, error, lastUpdated, refresh} = useOfficePolling(selectedPath, projectName, visible);
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
    if (!selectedPath || !taskId) return;
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

  useEffect(() => {
    setActiveTaskId('');
    setActiveMemberId('');
    setDetail(null);
    setDetailError('');
  }, [selectedPath]);

  // The board snapshot changes on every poll; refresh the open task with it
  // instead of running a second timer. Detail is never loaded for other tasks,
  // and a finished task's transcript is not reloaded on every idle poll.
  useEffect(() => {
    if (!visible || !activeTaskId) return;
    const current = snapshot?.tasks.find(item => item.id === activeTaskId);
    if (current && isTerminalTask(current.status) && detail?.task.id === activeTaskId) return;
    void loadDetail(activeTaskId, true);
  }, [visible, activeTaskId, loadDetail, snapshot, detail?.task.id]);

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
    setActiveMemberId('');
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
    try {
      setArtifact(await officeApi.artifact(selectedPath, item.id));
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : String(reason));
    }
  }

  if (!projectsLoading && !projects.length) {
    return <div className="office-page content-column">
      <div className="office-empty-project">
        <h1>Agent 办公室</h1>
        <p>一个项目对应一间办公室。先在左侧「小涛」里选一个项目目录，或保存一个项目，再回来安排任务。</p>
        {projectsError && <p className="office-error" role="alert">{projectsError}</p>}
      </div>
    </div>;
  }

  return <div className="office-page content-column">
    <header className="office-page-head">
      <div>
        <h1>Agent 办公室</h1>
        <p>找一位伙伴聊聊想法。准备好动手时，再一起整理交接。</p>
      </div>
      <div className="office-page-actions">
        <label className="office-project-picker">
          <span>项目</span>
          <select value={selectedPath} onChange={event => setSelectedPath(event.target.value)} disabled={projectsLoading}>
            {projects.map(item => <option key={item.path} value={item.path}>{item.name}</option>)}
          </select>
        </label>
        <button type="button" className="office-ghost" onClick={() => refresh()} disabled={!selectedPath || loading} title="刷新">
          <RefreshCw aria-hidden="true"/><span>{loading ? '刷新中…' : '刷新'}</span>
        </button>
        <button type="button" className="office-primary" disabled={!selectedPath || !snapshot} onClick={() => {setActiveTaskId(''); setActiveMemberId('xiaotao');}}>
          <Plus aria-hidden="true"/><span>找小涛聊聊</span>
        </button>
      </div>
    </header>
    {projectsError && <p className="office-error" role="alert">{projectsError}</p>}
    {error && <p className="office-error" role="alert">刷新办公室失败：{error}</p>}
    {!selectedPath && projectsLoading && <p role="status">读取项目…</p>}
    {selectedPath && snapshot && <p className="office-updated">
      上次更新 {lastUpdated ? new Date(lastUpdated).toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit', second: '2-digit'}) : '—'}
      {snapshot.accessMode === 'full' ? ' · 完全访问模式，高危操作不再逐条确认' : ' · 安全模式，高危操作需要你确认'}
    </p>}
    <div className="office-layout">
      <div className="office-canvas">
        {snapshot
          ? <OfficeRoom members={snapshot.members} artifacts={snapshot.artifacts} activeTaskId={activeTaskId} activeMemberId={activeMemberId} onSelectTask={selectTask} onSelectMember={id => {setActiveTaskId(''); setActiveMemberId(id);}} onOpenArtifact={item => void openArtifact(item)}/>
          : <div className="office-room office-room-skeleton">{loading ? <p role="status">正在打开办公室…</p> : <p className="office-muted">选择项目后加载办公室。</p>}</div>}
        <section className="office-board" aria-label="任务">
          <header><h2>任务</h2><span>{snapshot ? `${snapshot.tasks.filter(task => !isTerminalTask(task.status)).length} 个进行中 / 共 ${snapshot.tasks.length} 个` : ''}</span></header>
          {snapshot?.tasks.length
            ? <div className="office-task-list" role="list">{snapshot.tasks.map(task => <div role="listitem" key={task.id}>
                <TaskCard task={task} active={task.id === activeTaskId} onSelect={() => selectTask(task.id)}/>
              </div>)}</div>
            : <p className="office-muted">{loading ? '读取任务…' : '这个项目还没有任务。先找一位伙伴聊聊想法吧。'}</p>}
        </section>
        {snapshot && snapshot.artifacts.length > 0 && <section className="office-shelf-list" aria-label="全部产物">
          <header><h2>项目资料架</h2><span>{snapshot.artifacts.length} 个</span></header>
          <ul>{snapshot.artifacts.map(item => <li key={item.id}>
            <button type="button" className="office-artifact" onClick={() => void openArtifact(item)}>
              <span className="office-artifact-name">{item.path}</span>
              <small>{item.taskTitle} · {item.tool} · {new Date(item.updatedAt).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})}</small>
            </button>
          </li>)}</ul>
        </section>}
        {snapshot && <section className="office-caveats" aria-label="能力边界">
          <h2>这一版实际能做到什么</h2>
          <ul>{snapshot.capabilities.notes.map(note => <li key={note}>{note}</li>)}</ul>
        </section>}
      </div>
      {activeMemberId && snapshot?.members.find(member => member.id === activeMemberId)
        ? <MemberChat key={`${selectedPath}:${activeMemberId}`} projectPath={selectedPath} projectName={projectName} member={snapshot.members.find(member => member.id === activeMemberId)!}
            model={preferredModel} models={models} modelConfigs={modelConfigs} onClose={() => setActiveMemberId('')}
            onHandoff={goal => {setHandoffGoal(goal); setCreating(true);}}/>
        : <TaskDetail projectPath={selectedPath} detail={activeTaskId ? detail : null} loading={detailLoading} error={detailError}
            onRefresh={() => void loadDetail(activeTaskId)} onLoadEarlier={() => void loadEarlier()} onOpenArtifact={item => void openArtifact(item)}
            onCancel={() => void cancelTask()} onResolveApproval={(id, approved) => void resolveApproval(id, approved)} onPostMessage={postMessage}/>}
    </div>
    <ModalPresence>{creating && <CreateTaskModal projectPath={selectedPath} projectName={projectName} models={models}
      modelConfigs={modelConfigs} preferredModel={preferredModel} initialGoal={handoffGoal}
      onCreated={(task: OfficeTask) => { setCreating(false); toast.success('任务已安排，执行者开始工作'); refresh(); selectTask(task.id); }}
      onClose={() => setCreating(false)}/>}</ModalPresence>
    <ModalPresence>{artifact && <ArtifactModal detail={artifact} onClose={() => setArtifact(null)}/>}</ModalPresence>
  </div>;
}
