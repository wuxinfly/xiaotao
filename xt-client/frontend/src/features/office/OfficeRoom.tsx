import {BookOpen, Library, Radio, Users} from 'lucide-react';
import type {OfficeArtifact, OfficeMember, OfficeTask} from '../../lib/office';
import {taskStatusLabel, taskStatusTone} from '../../lib/office';
import {OfficeActor} from './OfficeActor';
import './office-scene.css';

type Props = {
  members: OfficeMember[];
  artifacts: OfficeArtifact[];
  projectName: string;
  onSelectMember: (memberId: string) => void;
  onSelectTask: (taskId: string) => void;
  onOpenLibrary: () => void;
};

export function OfficeRoom({projectName, members, artifacts, onSelectMember, onSelectTask, onOpenLibrary}: Props) {
  const working = members.filter(member => member.status === 'running').length;
  return <section className="office-room sea-office" aria-label="办公室">
    <div className="sea-office-atmosphere" aria-hidden="true">
      <span className="sea-office-window"><i/><i/><i/></span>
      <span className="sea-office-wall-light"/>
      <span className="sea-office-floor"/>
      <span className="sea-office-desk sea-office-desk-one"><i/><i/></span>
      <span className="sea-office-desk sea-office-desk-two"><i/><i/></span>
      <span className="sea-office-plant"><i/><i/><i/></span>
    </div>
    <header className="sea-office-head">
      <div className="sea-office-title">
        <span className="sea-office-label">工作空间 / {projectName}</span>
        <h1>Agent 办公室</h1>
        <p>员工正在办公室里活动，点一下看看他们在做什么。</p>
      </div>
      <div className="sea-office-live">
        <Radio size={14} aria-hidden="true"/>
        <span>{working ? `${working} 位成员在忙` : '办公室在线'}</span>
        <small>真实运行状态</small>
      </div>
    </header>
    <div className="sea-office-room-label"><Users size={14} aria-hidden="true"/>员工活动区<span>点击员工暂停并打开操作</span></div>
    <div className="sea-office-actors" aria-label="办公室员工">
      {members.map((member, index) => <OfficeActor
        key={member.id}
        member={member}
        index={index}
        onSelectMember={onSelectMember}
        onSelectTask={onSelectTask}
      />)}
      {!members.length && <p className="sea-office-empty">这个项目还没有成员。</p>}
    </div>
    <button type="button" className="sea-library-object" onClick={onOpenLibrary} aria-haspopup="dialog" aria-label="打开项目资料馆">
      <span className="sea-library-sign"><Library size={18}/><strong>项目资料馆</strong><small>员工已整理归档</small></span>
      <span className="sea-library-case" aria-hidden="true">
        <span className="sea-library-top"><i/><i/><i/><i/><i/></span>
        <span className="sea-library-middle"><i/><i/><i/><i/><i/><i/></span>
        <span className="sea-library-bottom"><i/><i/><i/><i/></span>
      </span>
      <span className="sea-library-foot"><BookOpen size={14}/>{artifacts.length} 本资料 · 打开目录</span>
    </button>
  </section>;
}

export function TaskCard({task, onSelect}: {task: OfficeTask; onSelect: () => void}) {
  return <button type="button" className={`office-task-card tone-${taskStatusTone[task.status]}`} onClick={onSelect}>
    <span className="office-task-status"><i aria-hidden="true"/>{taskStatusLabel[task.status]}</span>
    <strong>{task.title}</strong>
    <span className="office-task-meta">{task.executor.name} · {task.executor.modelLabel || task.executor.model}</span>
    {task.latestActivity && <span className="office-task-note">{task.latestActivity.title}{task.latestActivity.detail ? ` · ${task.latestActivity.detail}` : ''}</span>}
    {task.status === 'waiting' && <span className="office-task-wait">{task.waitingReason || '等待确认'}</span>}
    {!!task.queuedMessages && <span className="office-task-queued">{task.queuedMessages} 条消息排队中</span>}
    {task.error && <span className="office-task-error">{task.error}</span>}
  </button>;
}
