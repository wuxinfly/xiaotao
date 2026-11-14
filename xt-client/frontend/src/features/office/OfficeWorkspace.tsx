import {useEffect, useRef, useState} from 'react';
import {Archive, ArrowLeft, ClipboardList, MessageCircle, Plus, Settings2, Users} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import type {AgentModelConfig} from '../../lib/agent-model-settings';
import type {CSSProperties, ReactNode} from 'react';
import {isTerminalTask, type OfficeMember, type OfficeSnapshot, type OfficeTaskDetail} from '../../lib/office';
import {TaskCard} from './OfficeRoom';
import {TaskDetail} from './TaskDetail';
import {MemberChat} from './MemberChat';
import {Select} from '../../components/select/Select';
import './office-detail-panel.css';

type Props = {
  projectPath: string;
  projectName: string;
  projects: {path: string; name: string}[];
  snapshot: OfficeSnapshot | null;
  loading: boolean;
  error: string;
  view: 'tasks' | 'team' | 'task' | 'member';
  activeTaskId: string;
  activeMemberId: string;
  detail: OfficeTaskDetail | null;
  detailLoading: boolean;
  detailError: string;
  models: AgentModelOption[];
  modelConfigs: AgentModelConfig[];
  preferredModel: string;
  children: ReactNode;
  onProject: (path: string) => void;
  onView: (view: 'tasks' | 'team') => void;
  onRefresh: () => void;
  onCreate: () => void;
  onManage: () => void;
  onSelectTask: (id: string) => void;
  onSelectMember: (id: string) => void;
  onCloseDetail: () => void;
  onRefreshDetail: () => void;
  onLoadEarlier: () => void;
  onCancelTask: () => void;
  onResolveApproval: (id: string, approved: boolean) => void;
  onPostMessage: (text: string) => Promise<void>;
  onOpenArtifact: (item: OfficeSnapshot['artifacts'][number]) => void;
  onHandoff: (goal: string) => void;
  onOpenLibrary: () => void;
};

const roleLabel: Record<string, string> = {coordinator: '协调', designer: '设计', developer: '开发', executor: '执行'};
const memberStatus = {running: '工作中', waiting: '等待确认', failed: '需关注', idle: '在线'} as Record<string, string>;

function roleColor(member: OfficeMember) {
  return {'--member-color': member.appearance?.primaryColor || '#5faea2'} as CSSProperties;
}

export function OfficeWorkspace(props: Props) {
  const {snapshot, view} = props;
  const [filters, setFilters] = useState({path: props.projectPath, task: 'active'});
  const projectChanged = filters.path !== props.projectPath;
  if (projectChanged) setFilters({path: props.projectPath, task: 'active'});
  const taskFilter = projectChanged ? 'active' : filters.task;
  const panel = useRef<HTMLElement>(null);
  const previous = useRef({view, path: props.projectPath});
  useEffect(() => {
    const sameProject = previous.current.path === props.projectPath;
    const changedView = previous.current.view !== view;
    previous.current = {view, path: props.projectPath};
    if (!sameProject || !changedView) return;
    if (!matchMedia('(max-width: 840px)').matches) return;
    const target = panel.current;
    if (target && target.getBoundingClientRect().top > window.innerHeight - 120) target.scrollIntoView({block: 'start'});
  }, [view, props.projectPath]);
  const tasks = snapshot?.tasks.filter(task => taskFilter === 'all' || (taskFilter === 'active' ? !isTerminalTask(task.status) : isTerminalTask(task.status))) || [];
  const activeCount = snapshot?.tasks.filter(task => !isTerminalTask(task.status)).length || 0;
  const canCreate = !!snapshot;
  const selected = props.activeMemberId && snapshot?.members.find(member => member.id === props.activeMemberId);
  return <div className="office-workspace">
    <header className="workspace-heading">
      <div><span className="workspace-kicker">项目协作</span><h1>办公室</h1></div>
      <div className="workspace-heading-right"><span className="workspace-project-label">当前项目</span><Select aria-label="切换项目" value={props.projectPath} onChange={props.onProject} options={props.projects.map(project => ({value: project.path, label: project.name}))}/><button className="workspace-create" type="button" disabled={!canCreate} onClick={props.onCreate}><Plus size={17}/>新建任务</button></div>
    </header>
    <div className="workspace-grid">
      <div className="workspace-scene" role="region" aria-label={`${props.projectName}的办公室`}>
        {props.children}
        {props.error && <p className="office-error workspace-scene-error" role="alert">刷新办公室失败：{props.error}<button className="office-link" onClick={props.onRefresh}>重试</button></p>}
      </div>
      <aside ref={panel} className={`workspace-sidebar ${view === 'task' || view === 'member' ? 'is-detail' : ''}`} aria-label="办公室协作面板">
        {view === 'task' ? <div className="workspace-detail-wrap">
          <button type="button" className="workspace-back" onClick={props.onCloseDetail}><ArrowLeft size={15}/>返回任务列表</button>
          <TaskDetail key={props.activeTaskId} projectPath={props.projectPath} detail={props.detail} loading={props.detailLoading} error={props.detailError}
            onRefresh={props.onRefreshDetail} onLoadEarlier={props.onLoadEarlier} onOpenArtifact={props.onOpenArtifact}
            onCancel={props.onCancelTask} onResolveApproval={props.onResolveApproval} onPostMessage={props.onPostMessage}/>
        </div> : view === 'member' && selected ? <div className="workspace-detail-wrap">
          <button type="button" className="workspace-back" onClick={props.onCloseDetail}><ArrowLeft size={15}/>返回团队成员</button>
          <div className="workspace-conversation-title"><span className="workspace-member-avatar" style={roleColor(selected)}>{selected.name.slice(0, 1)}</span><div><h2>与{selected.name}对话</h2><p>{roleLabel[selected.role] || '项目成员'} · 对话不操作项目文件</p></div></div>
          <MemberChat key={selected.id} projectPath={props.projectPath} projectName={props.projectName} member={selected}
            preferredModel={props.preferredModel} models={props.models} modelConfigs={props.modelConfigs} onHandoff={props.onHandoff}/>
        </div> : <>
          <div className="workspace-tabs" role="tablist" aria-label="协作面板">
            <button type="button" role="tab" aria-selected={view === 'tasks'} className={view === 'tasks' ? 'is-selected' : ''} onClick={() => props.onView('tasks')}><ClipboardList size={15}/>任务{activeCount > 0 && <span>{activeCount}</span>}</button>
            <button type="button" role="tab" aria-selected={view === 'team'} className={view === 'team' ? 'is-selected' : ''} onClick={() => props.onView('team')}><Users size={15}/>团队<span>{snapshot?.members.length || 0}</span></button>
          </div>
          {view === 'tasks' ? <section className="workspace-tasks" aria-label="任务列表">
            <div className="workspace-panel-heading"><div><h2>项目任务</h2><p>跟踪进度和执行记录</p></div></div>
            <div className="workspace-task-filters" aria-label="筛选任务">{[['active', '进行中'], ['finished', '已结束'], ['all', '全部']].map(([value, label]) => <button type="button" key={value} className={taskFilter === value ? 'is-selected' : ''} aria-pressed={taskFilter === value} onClick={() => setFilters({path: props.projectPath, task: value})}>{label}</button>)}</div>
            <div className="workspace-task-list">{tasks.map(task => <TaskCard key={task.id} task={task} onSelect={() => props.onSelectTask(task.id)}/>)}
              {!tasks.length && <div className="workspace-empty"><span className="workspace-empty-icon"><ClipboardList size={22}/></span><strong>{!snapshot ? (props.error ? '任务读取失败' : '正在读取项目任务…') : taskFilter === 'active' ? '现在没有进行中的任务' : taskFilter === 'finished' ? '还没有已结束的任务' : '这个项目还没有任务'}</strong>{snapshot && <p>{taskFilter === 'finished' ? '任务结束后会出现在这里。' : '把目标和交付要求交给小涛，执行进度会显示在这里。'}</p>}</div>}
            </div>
          </section> : <section className="workspace-team" aria-label="团队成员列表">
            <div className="workspace-panel-heading"><div><h2>项目团队</h2><p>查看成员与当前工作</p></div><button className="workspace-icon-button" disabled={!props.projectPath} onClick={props.onManage}><Settings2 size={15}/>管理团队</button></div>
            <div className="workspace-member-list">{snapshot?.members.map(member => <button type="button" className="workspace-member" key={member.id} data-member-id={member.id} onClick={() => props.onSelectMember(member.id)}>
              <span className="workspace-member-avatar" style={roleColor(member)}>{member.name.slice(0, 1)}</span>
              <span className="workspace-member-info"><strong><span title={member.name}>{member.name}</span><small>{roleLabel[member.role] || '项目成员'}</small></strong><span>{member.responsibility || (member.taskId ? '执行项目任务' : '可发起对话')}</span><em className={`status-${member.status}`}>{memberStatus[member.status] || member.status}{member.taskTitle ? ` · ${member.taskTitle}` : ''}</em></span>{member.taskId ? <ClipboardList size={16}/> : <MessageCircle size={16}/>}
            </button>)}{!snapshot?.members.length && <div className="workspace-empty"><span className="workspace-empty-icon"><Users size={22}/></span><strong>{snapshot ? '团队还空着' : props.error ? '团队读取失败' : '正在读取团队…'}</strong></div>}</div>
            <button type="button" className="workspace-library-link" onClick={props.onOpenLibrary}><Archive size={16}/><span>项目资料馆</span><small>{snapshot?.artifacts.length || 0} 份任务产物</small></button>
          </section>}
        </>}
      </aside>
    </div>
    <footer className="workspace-statusbar"><span><i className={`workspace-live-dot ${snapshot && !props.error ? 'is-connected' : ''}`}/>{props.error ? '同步失败' : props.loading ? '正在同步' : snapshot ? '状态自动同步' : '等待同步'}</span><span>{snapshot ? `${snapshot.members.length} 位成员 · ${activeCount} 个进行中任务` : ''}</span><span>任务在项目目录内执行</span></footer>
  </div>;
}
