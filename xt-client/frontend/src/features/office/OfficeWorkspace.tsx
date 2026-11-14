import {useState} from 'react';
import {Archive, ArrowLeft, ClipboardList, MessageCircle, Plus, Settings2, Users} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import type {AgentModelConfig} from '../../lib/agent-model-settings';
import type {ReactNode} from 'react';
import {formatClock, isTerminalTask, type OfficeMember, type OfficeSnapshot, type OfficeTaskDetail} from '../../lib/office';
import {TaskCard} from './OfficeRoom';
import {TaskDetail} from './TaskDetail';
import {MemberChat} from './MemberChat';
import {Select} from '../../components/select/Select';

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
  return member.appearance?.primaryColor || '#54746c';
}

export function OfficeWorkspace(props: Props) {
  const {snapshot, view} = props;
  const [taskFilter, setTaskFilter] = useState('active');
  const tasks = snapshot?.tasks.filter(task => taskFilter === 'all' || (taskFilter === 'active' ? !isTerminalTask(task.status) : isTerminalTask(task.status))) || [];
  const activeCount = snapshot?.tasks.filter(task => !isTerminalTask(task.status)).length || 0;
  const selected = props.activeMemberId && snapshot?.members.find(member => member.id === props.activeMemberId);
  return <div className="office-workspace">
    <header className="workspace-heading">
      <div><span className="workspace-kicker">协作空间</span><h1>办公室<span className="workspace-live-dot" title="状态实时同步" aria-hidden="true"/></h1></div>
      <div className="workspace-heading-right"><span className="workspace-project-label">当前项目</span><Select aria-label="切换项目" value={props.projectPath} onChange={props.onProject} options={props.projects.map(project => ({value: project.path, label: project.name}))}/><button className="workspace-create" type="button" onClick={props.onCreate}><Plus size={17}/>新建任务</button></div>
    </header>
    <div className="workspace-grid">
      <main className="workspace-scene" aria-label={`${props.projectName}的办公室`}>
        {props.children}
        {props.error && <p className="office-error workspace-scene-error" role="alert">刷新办公室失败：{props.error}<button className="office-link" onClick={props.onRefresh}>重试</button></p>}
      </main>
      <aside className={`workspace-sidebar ${view === 'task' || view === 'member' ? 'is-detail' : ''}`} aria-label="办公室协作面板">
        {view === 'task' ? <div className="workspace-detail-wrap">
          <button type="button" className="workspace-back" onClick={props.onCloseDetail}><ArrowLeft size={15}/>返回任务列表</button>
          <TaskDetail key={props.activeTaskId} projectPath={props.projectPath} detail={props.detail} loading={props.detailLoading} error={props.detailError}
            onRefresh={props.onRefreshDetail} onLoadEarlier={props.onLoadEarlier} onOpenArtifact={props.onOpenArtifact}
            onCancel={props.onCancelTask} onResolveApproval={props.onResolveApproval} onPostMessage={props.onPostMessage}/>
        </div> : view === 'member' && selected ? <div className="workspace-detail-wrap">
          <button type="button" className="workspace-back" onClick={props.onCloseDetail}><ArrowLeft size={15}/>返回团队成员</button>
          <div className="workspace-conversation-title"><span className="workspace-member-avatar" style={{background: roleColor(selected)}}>{selected.name.slice(0, 1)}</span><div><h2>与{selected.name}对话</h2><p>{roleLabel[selected.role] || '项目成员'} · 对话不操作项目文件</p></div></div>
          <MemberChat key={selected.id} projectPath={props.projectPath} projectName={props.projectName} member={selected}
            preferredModel={props.preferredModel} models={props.models} modelConfigs={props.modelConfigs} onHandoff={props.onHandoff}/>
        </div> : <>
          <div className="workspace-tabs" role="tablist" aria-label="协作面板">
            <button type="button" role="tab" aria-selected={view === 'tasks'} className={view === 'tasks' ? 'is-selected' : ''} onClick={() => props.onView('tasks')}><ClipboardList size={15}/>任务{activeCount > 0 && <span>{activeCount}</span>}</button>
            <button type="button" role="tab" aria-selected={view === 'team'} className={view === 'team' ? 'is-selected' : ''} onClick={() => props.onView('team')}><Users size={15}/>团队<span>{snapshot?.members.length || 0}</span></button>
          </div>
          {view === 'tasks' ? <section className="workspace-tasks" aria-label="任务列表">
            <div className="workspace-panel-heading"><div><h2>任务区</h2><p>查看目标、进度和执行过程</p></div><button className="workspace-icon-button" title="新建任务" aria-label="新建任务" onClick={props.onCreate}><Plus size={17}/></button></div>
            <div className="workspace-task-filters" aria-label="筛选任务">{[['active', '进行中'], ['finished', '已结束'], ['all', '全部']].map(([value, label]) => <button type="button" key={value} className={taskFilter === value ? 'is-selected' : ''} aria-pressed={taskFilter === value} onClick={() => setTaskFilter(value)}>{label}</button>)}</div>
            <div className="workspace-task-list">{tasks.map(task => <TaskCard key={task.id} task={task} onSelect={() => props.onSelectTask(task.id)}/>)}
              {!tasks.length && <div className="workspace-empty"><ClipboardList size={26}/><strong>{taskFilter === 'active' ? '现在没有进行中的任务' : '这里还没有任务'}</strong><p>{taskFilter === 'active' ? '有个想法？交给小涛，看看它能怎么落地。' : '切换筛选查看其他任务。'}</p>{taskFilter === 'active' && <button type="button" className="workspace-create" onClick={props.onCreate}><Plus size={15}/>新建任务</button>}</div>}
            </div>
            <div className="workspace-panel-footer"><span className="workspace-live-dot"/>{props.loading ? '正在同步任务状态' : '任务状态自动同步'}{snapshot && <time>{formatClock(snapshot.tasks[0]?.updatedAt)}</time>}</div>
          </section> : <section className="workspace-team" aria-label="团队成员列表">
            <div className="workspace-panel-heading"><div><h2>项目团队</h2><p>点击成员，开始对话</p></div><button className="workspace-icon-button" title="管理团队" aria-label="管理团队" onClick={props.onManage}><Settings2 size={17}/></button></div>
            <div className="workspace-member-list">{snapshot?.members.map(member => <button type="button" className="workspace-member" key={member.id} data-member-id={member.id} onClick={() => props.onSelectMember(member.id)}>
              <span className="workspace-member-avatar" style={{background: roleColor(member)}}>{member.name.slice(0, 1)}<i className={`status-${member.status}`}/></span>
              <span className="workspace-member-info"><strong><span title={member.name}>{member.name}</span><small>{roleLabel[member.role] || '项目成员'}</small></strong><span>{member.responsibility || (member.taskId ? '正在执行项目任务' : '随时待命，等你开聊')}</span><em className={`status-${member.status}`}>{memberStatus[member.status] || member.status}{member.status === 'running' && member.taskTitle ? ` · ${member.taskTitle}` : ''}</em></span><MessageCircle size={16}/>
            </button>)}{!snapshot?.members.length && <div className="workspace-empty"><Users size={26}/><strong>团队还空着</strong><p>添加你的第一位协作伙伴。</p><button className="workspace-create" onClick={props.onManage}>管理团队</button></div>}</div>
            <button type="button" className="workspace-library-link" onClick={props.onOpenLibrary}><Archive size={16}/><span>项目资料馆<small>{snapshot?.artifacts.length || 0} 份任务产物</small></span></button>
          </section>}
        </>}
      </aside>
    </div>
    <footer className="workspace-statusbar"><span><i className="workspace-live-dot"/>办公室已连接</span><span>{snapshot?.members.length || 0} 位成员 · {activeCount} 个进行中任务</span><span>任务在项目目录内执行</span></footer>
  </div>;
}
