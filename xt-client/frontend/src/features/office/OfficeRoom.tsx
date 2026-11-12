import type {OfficeArtifact, OfficeMember, OfficeTask} from '../../lib/office';
import {formatBytes, taskStatusLabel, taskStatusTone} from '../../lib/office';
import './office-scene.css';

type Props = {
  members: OfficeMember[];
  artifacts: OfficeArtifact[];
  activeTaskId?: string;
  activeMemberId?: string;
  onSelectMember?: (memberId: string) => void;
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

export function OfficeRoom({members, artifacts, activeTaskId, activeMemberId, onSelectMember, onSelectTask, onOpenArtifact}: Props) {
  const working = members.filter(member => member.status === 'running').length;
  return <section className="office-room sea-office" aria-label="办公室">
    <header className="sea-office-head">
      <div><strong>今天的办公室</strong><small>{working ? `${working} 位成员正在工作` : '办公室里静悄悄的'}</small></div>
      <span className="sea-office-live"><i/>真实运行状态</span>
    </header>
    <div className="sea-office-stage">
      <div className="sea-office-window" aria-hidden="true"><span/><span/><span/></div>
      <div className="sea-office-horizon" aria-hidden="true"/>
      <div className="sea-office-members">
        {members.map(member => {
          const status = memberStatus(member);
          const tone = taskStatusTone[(member.taskStatus || member.status) as keyof typeof taskStatusTone] || 'idle';
          const active = (member.taskId && member.taskId === activeTaskId) || member.id === activeMemberId;
          const selectable = !!member.taskId || !!onSelectMember;
          return <button type="button" key={member.id} className={`office-desk sea-station tone-${tone}${active ? ' is-active' : ''}`}
            disabled={!selectable} aria-pressed={selectable ? !!active : undefined}
            onClick={() => member.taskId && (member.role === 'executor' || !onSelectMember) ? onSelectTask(member.taskId) : onSelectMember?.(member.id)}
            aria-label={`${member.name}，${status}${member.taskTitle ? `，${member.taskTitle}` : ''}`}>
            <span className="sea-station-scene" aria-hidden="true">
              <span className="sea-station-halo"/><span className="sea-person"><span className="sea-person-head"/><span className="sea-person-body"/></span>
              <span className="sea-monitor"><span/></span><span className="sea-table"/>
            </span>
            <span className="sea-station-info"><span className="sea-station-name">{member.name}<span className="sea-station-status"><i/>{status}</span></span>
              {member.taskTitle && <span className="sea-station-task">{member.taskTitle}</span>}
              <span className="sea-station-note">{memberNote(member)}</span>
            </span>
          </button>;
        })}
        {!members.length && <p className="sea-office-empty">项目里还没有成员。给小涛一个任务，他就会安排执行者进场。</p>}
      </div>
      <div className="office-shelf sea-shelf" aria-label="项目资料架">
        <div className="sea-shelf-heading"><span className="sea-shelf-icon" aria-hidden="true">▤</span><span><strong>项目资料架</strong><small>工作留下的文件</small></span></div>
        <div className="sea-shelf-files">{artifacts.length
          ? artifacts.slice(0, 6).map(artifact => <button type="button" key={artifact.id} className="sea-shelf-file" title={`${artifact.path} · ${formatBytes(artifact.size)}`} onClick={() => onOpenArtifact(artifact)}>
              <span className="sea-file-icon" aria-hidden="true"/><span>{artifact.path.split(/[\\/]/).pop()}</span>
            </button>)
          : <span className="sea-shelf-empty">还没有可识别的产物。执行者写入文件后会出现在这里。</span>}
          {artifacts.length > 6 && <span className="sea-shelf-more">+{artifacts.length - 6}</span>}
        </div>
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
