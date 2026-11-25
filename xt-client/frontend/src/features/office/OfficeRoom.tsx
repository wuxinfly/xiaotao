import {ArrowRight, Bell, ClipboardList, MessageCircle} from 'lucide-react';
import type {ReactNode} from 'react';
import type {OfficeMember, OfficeTask} from '../../lib/office';
import type {OfficeConversation} from '../../lib/office-conversations';
import {RoleMark} from './RolePreview';
import './office-desks.css';

export interface OfficeRoomProps {
  members: OfficeMember[];
  tasks: OfficeTask[];
  conversations: OfficeConversation[];
  coordinatorRunning: boolean;
  loading: boolean;
  error: string;
  setup?: ReactNode;
  onSelectMember: (id: string) => void;
  onSelectTask: (id: string) => void;
  onSelectConversation: (item: OfficeConversation) => void;
}

const roleLabel: Record<string, string> = {coordinator: '协调', designer: '设计', developer: '开发', executor: '任务执行'};
const statusLabel: Record<string, string> = {running: '进行中', waiting: '等待确认', queued: '排队中', failed: '需关注', idle: '空闲'};

export function OfficeRoom({members, tasks, conversations, coordinatorRunning, loading, error, setup, onSelectMember, onSelectTask, onSelectConversation}: OfficeRoomProps) {
  const pendingConversations = conversations.filter(item => item.approval || item.question || item.coordinatorQuestion);
  const pendingTasks = tasks.filter(task => task.status === 'waiting' && task.approval && !pendingConversations.some(item => item.taskId === task.id && item.approval));
  const pendingCount = pendingConversations.length + pendingTasks.length;
  return <section className="office-desks" aria-label="办公室工位">
    {pendingCount > 0 && <section className="office-attention" aria-label="待你处理">
      <h3><Bell size={15} aria-hidden="true"/>待你处理<span>{pendingCount}</span></h3>
      {pendingConversations.map(item => <button type="button" key={item.memberId} onClick={() => onSelectConversation(item)}>
        <span>{item.memberName || '员工'} · {item.approval ? '等你确认' : '等你回复'}</span><ArrowRight size={14} aria-hidden="true"/>
      </button>)}
      {pendingTasks.map(task => <button type="button" key={task.id} onClick={() => onSelectTask(task.id)}>
        <span>{task.title} · 等你确认</span><ArrowRight size={14} aria-hidden="true"/>
      </button>)}
    </section>}
    <div className="office-desks-heading"><h3>工位</h3>{setup}</div>
    {members.length ? <ul className="office-desk-grid">{members.map(member => {
      const conversation = conversations.find(item => item.memberId === member.id);
      const taskId = member.taskId || conversation?.taskId;
      const taskTitle = member.taskTitle || (conversation?.status === 'completed' ? undefined : conversation?.taskTitle);
      const status = member.role === 'coordinator' && coordinatorRunning ? 'running'
        : conversation?.approval ? 'waiting'
        : conversation?.status === 'running' || conversation?.status === 'queued' ? conversation.status
        : member.taskStatus === 'queued' ? 'queued' : member.status;
      const label = (conversation?.question || conversation?.coordinatorQuestion) && !conversation.approval ? '等你回复' : statusLabel[status] || '空闲';
      return <li key={member.id} className="office-desk" data-status={status}>
        <button type="button" className="office-desk-person" data-member-id={member.id} onClick={() => onSelectMember(member.id)}>
          <span className="office-desk-portrait"><RoleMark name={member.name} appearance={member.appearance}/></span>
          <span className="office-desk-identity"><strong title={member.name}>{member.name}</strong><small title={member.responsibility}>{member.responsibility || roleLabel[member.role] || '项目成员'}</small></span>
          {member.role === 'executor' && member.taskId ? <ClipboardList size={15} aria-hidden="true"/> : <MessageCircle size={15} aria-hidden="true"/>}
        </button>
        <div className="office-desk-work"><span><i aria-hidden="true"/>{label}</span>
          {taskId && taskTitle && <button type="button" title={taskTitle} onClick={() => onSelectTask(taskId)}>{taskTitle}<ArrowRight size={12} aria-hidden="true"/></button>}
        </div>
      </li>;
    })}</ul> : <p role="status" className="office-desks-empty">{error ? '暂时无法读取工位。' : loading ? '正在读取工位…' : '还没有工位。组建小队后，成员会出现在这里。'}</p>}
  </section>;
}
