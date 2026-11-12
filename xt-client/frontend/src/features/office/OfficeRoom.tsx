import type {OfficeArtifact, OfficeMember, OfficeTask} from '../../lib/office';
import {formatBytes, taskStatusLabel, taskStatusTone} from '../../lib/office';

type Props = {
  members: OfficeMember[];
  artifacts: OfficeArtifact[];
  activeTaskId?: string;
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

// OfficeRoom renders only real desks derived from real tasks. A finished task
// leaves the room; its artifacts stay on the shelf.
export function OfficeRoom({members, artifacts, activeTaskId, onSelectTask, onOpenArtifact}: Props) {
  const working = members.filter(member => member.role === 'executor' && member.status === 'running').length;
  return <section className="office-room" aria-label="办公室">
    <header className="office-room-head">
      <div>
        <strong>今天的办公室</strong>
        <small>{working ? `${working} 位执行者正在工作` : members.length > 1 ? '执行者已就位，等待任务' : '还没有执行者进场'}</small>
      </div>
      <span className="office-room-tag">真实运行状态</span>
    </header>
    <div className="office-room-floor">
      <div className="office-window" aria-hidden="true"/>
      <div className="office-desks">
        {members.map(member => {
          const status = member.role === 'coordinator' ? 'idle' : member.status;
          const tone = member.role === 'coordinator' ? 'coordinator' : taskStatusTone[(member.taskStatus || 'queued') as keyof typeof taskStatusTone] || 'idle';
          const clickable = !!member.taskId;
          return <button type="button" key={member.id} className={`office-desk tone-${tone}${member.taskId === activeTaskId ? ' is-active' : ''}`}
            disabled={!clickable}
            onClick={() => member.taskId && onSelectTask(member.taskId)}
            aria-label={`${member.name}，${member.role === 'coordinator' ? '负责人' : taskStatusLabel[(member.taskStatus || 'queued') as keyof typeof taskStatusLabel] || '空闲'}`}>
            <span className="office-avatar" aria-hidden="true">{member.role === 'coordinator' ? '🧑🏻' : '👩🏻‍💻'}</span>
            <span className="office-desk-status">
              <i aria-hidden="true"/>{member.role === 'coordinator' ? '负责人' : taskStatusLabel[(member.taskStatus || 'queued') as keyof typeof taskStatusLabel] || status}
            </span>
            <strong>{member.name}</strong>
            {member.taskTitle && <span className="office-desk-task">{member.taskTitle}</span>}
            <span className="office-desk-note">{memberNote(member)}</span>
          </button>;
        })}
        {!members.length && <p className="office-room-empty">项目里还没有成员。给小涛一个任务，他就会安排执行者进场。</p>}
      </div>
      <div className="office-shelf" aria-label="项目资料架">
        <b>▥ 项目资料架</b>
        {artifacts.length
          ? artifacts.slice(0, 6).map(artifact => <button type="button" key={artifact.id} className="office-shelf-item" title={`${artifact.path} · ${formatBytes(artifact.size)}`} onClick={() => onOpenArtifact(artifact)}>
              <span aria-hidden="true">📄</span><span className="office-shelf-name">{artifact.path.split('/').pop()}</span>
            </button>)
          : <span className="office-shelf-empty">还没有可识别的产物。执行者写入文件后会出现在这里。</span>}
        {artifacts.length > 6 && <span className="office-shelf-more">+{artifacts.length - 6}</span>}
      </div>
    </div>
  </section>;
}

export function TaskCard({task, active, onSelect}: {task: OfficeTask; active: boolean; onSelect: () => void}) {
  return <button type="button" className={`office-task-card tone-${taskStatusTone[task.status]}${active ? ' is-active' : ''}`}
    aria-current={active ? 'true' : undefined} onClick={onSelect}>
    <span className="office-task-status"><i aria-hidden="true"/>{taskStatusLabel[task.status]}</span>
    <strong>{task.title}</strong>
    <span className="office-task-meta">{task.executor.name} · {task.executor.modelLabel || task.executor.model}</span>
    {task.latestActivity && <span className="office-task-note">{task.latestActivity.title}{task.latestActivity.detail ? ` · ${task.latestActivity.detail}` : ''}</span>}
    {task.status === 'waiting' && <span className="office-task-wait">{task.waitingReason || '等待确认'}</span>}
    {!!task.queuedMessages && <span className="office-task-queued">{task.queuedMessages} 条消息排队中</span>}
    {task.error && <span className="office-task-error">{task.error}</span>}
  </button>;
}
