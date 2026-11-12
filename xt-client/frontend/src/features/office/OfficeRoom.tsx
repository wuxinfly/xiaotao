import {Archive, ArrowUpRight, FileText} from 'lucide-react';
import type {OfficeArtifact, OfficeMember, OfficeTask} from '../../lib/office';
import {formatBytes, taskStatusLabel, taskStatusTone} from '../../lib/office';
import './office-scene.css';

type Props = {
  members: OfficeMember[];
  artifacts: OfficeArtifact[];
  projectName: string;
  onSelectMember: (memberId: string) => void;
  onSelectTask: (taskId: string) => void;
  onOpenArtifact: (artifact: OfficeArtifact) => void;
};

const activityLabel: Record<string, string> = {
  read_file: '正在读文件',
  list_files: '正在看目录',
  search_files: '正在搜代码',
  write_file: '正在写文件',
  str_replace: '正在改文件',
  run_command: '正在跑命令',
};

function memberNote(member: OfficeMember) {
  if (member.role === 'coordinator') return '协调任务与交接';
  const activity = member.latestActivity;
  if (activity) {
    if (activity.status === 'running') return activityLabel[activity.kind] || activity.title;
    return `${activity.title}${activity.detail ? ' · ' + activity.detail : ''}`;
  }
  switch (member.status) {
    case 'running': return '正在处理任务';
    case 'waiting': return '等待你确认';
    case 'completed': return '任务已完成';
    case 'failed': return '任务失败';
    case 'canceled': return '任务已取消';
    default: return '暂时空闲';
  }
}

function memberStatus(member: OfficeMember) {
  const status = member.taskStatus || member.status;
  return taskStatusLabel[status as keyof typeof taskStatusLabel] || (status === 'idle' ? '空闲' : status || '状态未知');
}

export function OfficeRoom({projectName, members, artifacts, onSelectMember, onSelectTask, onOpenArtifact}: Props) {
  const working = members.filter(member => member.status === 'running').length;
  return <section className="office-room sea-office" aria-label="办公室">
    <header className="sea-office-head">
      <div><span className="sea-office-label">工作空间 / {projectName}</span><h1>Agent 办公室</h1><p>点一位伙伴，聊聊手头的事。</p></div>
      <span className="sea-office-live"><i/>{working ? <span>{working} 位成员在忙</span> : <span>办公室在线</span>}<small>真实运行状态</small></span>
    </header>
    <div className="sea-office-stage">
      <div className="sea-office-workspace">
        <div className="sea-office-window" aria-hidden="true"><span/><span/><span/></div>
        <span className="sea-office-zone" aria-hidden="true">01 / 员工工位</span>
        <div className="sea-office-members">
          {members.map((member, index) => <Station key={member.id} member={member} number={index + 1}
            onSelect={() => member.taskId ? onSelectTask(member.taskId) : onSelectMember(member.id)}/>)}
          {!members.length && <p className="sea-office-empty">这个项目还没有成员。</p>}
        </div>
      </div>
      <aside className="sea-shelf" aria-labelledby="office-shelf-title">
        <div className="sea-shelf-heading"><Archive size={19} aria-hidden="true"/><h2 id="office-shelf-title">项目资料架</h2><span>{artifacts.length}</span></div>
        {artifacts.length ? <ul className="sea-shelf-files">
          {artifacts.map(item => <li key={item.id}>
            <button type="button" className="sea-shelf-file" aria-haspopup="dialog" title={item.path} onClick={() => onOpenArtifact(item)}>
              <FileText size={17} aria-hidden="true"/><span><strong>{item.path.split(/[\\/]/).pop()}</strong><small>{formatBytes(item.size)} · {item.taskTitle}</small></span><ArrowUpRight size={13} aria-hidden="true"/>
            </button>
          </li>)}
        </ul> : <p className="sea-shelf-empty">还没有项目资料。员工写入的文件会出现在这里。</p>}
      </aside>
    </div>
  </section>;
}

function Station({member, number, onSelect}: {member: OfficeMember; number: number; onSelect: () => void}) {
  const status = memberStatus(member);
  const tone = taskStatusTone[(member.taskStatus || member.status) as keyof typeof taskStatusTone] || 'idle';
  return <button type="button" className={`office-desk sea-station role-${member.role} tone-${tone}`} onClick={onSelect}
    aria-haspopup="dialog" aria-label={`${member.name}，${status}${member.taskTitle ? `，${member.taskTitle}` : ''}`}>
    <span className="sea-station-scene" aria-hidden="true">
      <span className="sea-station-halo"/><span className="sea-person"><span className="sea-person-head"/><span className="sea-person-body"/></span>
      <span className="sea-monitor"><span/></span><span className="sea-table"/><span className="sea-station-number">{String(number).padStart(2, '0')}</span>
    </span>
    <span className="sea-station-info">
      <span className="sea-station-name">{member.name}<span className="sea-station-status"><i/>{status}</span></span>
      {member.taskTitle && <span className="sea-station-task">{member.taskTitle}</span>}
      <span className="sea-station-note">{memberNote(member)}</span>
      <span className="sea-station-link">{member.taskId ? '查看任务' : '开始对话'}<ArrowUpRight size={12} aria-hidden="true"/></span>
    </span>
  </button>;
}

export function TaskCard({task, onSelect}: {task: OfficeTask; onSelect: () => void}) {
  return <button type="button" className={`office-task-card tone-${taskStatusTone[task.status]}`}
    onClick={onSelect}>
    <span className="office-task-status"><i aria-hidden="true"/>{taskStatusLabel[task.status]}</span>
    <strong>{task.title}</strong>
    <span className="office-task-meta">{task.executor.name} · {task.executor.modelLabel || task.executor.model}</span>
    {task.latestActivity && <span className="office-task-note">{task.latestActivity.title}{task.latestActivity.detail ? ` · ${task.latestActivity.detail}` : ''}</span>}
    {task.status === 'waiting' && <span className="office-task-wait">{task.waitingReason || '等待确认'}</span>}
    {!!task.queuedMessages && <span className="office-task-queued">{task.queuedMessages} 条消息排队中</span>}
    {task.error && <span className="office-task-error">{task.error}</span>}
  </button>;
}
