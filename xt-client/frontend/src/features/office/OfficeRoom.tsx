import {Library, Radio, Users} from 'lucide-react';
import type {OfficeMember, OfficeTask} from '../../lib/office';
import {taskStatusLabel, taskStatusTone} from '../../lib/office';
import './office-scene.css';
import './office-3d.css';
import {useEffect, useRef, useState} from 'react';
import {OfficeScene3D, type SceneControls} from './OfficeScene3D';
import {sceneMembers} from './scene-contract';

type Props = {
  members: OfficeMember[];
  artifactCount: number;
  onError: (message: string) => void;
  onReady: () => void;
  projectName: string;
  onSelectMember: (memberId: string) => void;
  onOpenLibrary: () => void;
  active: boolean;
};

export function OfficeRoom({projectName, members, artifactCount, onSelectMember, onOpenLibrary, active, onError, onReady}: Props) {
  const working = members.filter(member => member.status === 'running').length;
  const [instructionsDismissed, setInstructionsDismissed] = useState(false);
  const [membersExpanded, setMembersExpanded] = useState(() => !matchMedia('(max-width: 520px)').matches);
  useEffect(() => {
    const query = matchMedia('(max-width: 520px)');
    const update = () => setMembersExpanded(!query.matches);
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);
  const controls = useRef<SceneControls>(null);
  const team = sceneMembers(members);
  const choose = (id: string) => {setInstructionsDismissed(true); onSelectMember(id);};
  const openLibrary = () => {setInstructionsDismissed(true); onOpenLibrary();};
  return <section className="office-room sea-office office-3d-room" aria-label="办公室">
    <header className="sea-office-head">
      <div className="sea-office-title">
        <span className="sea-office-label">工作空间 / {projectName}</span>
        <h1>Agent 办公室</h1>
        <p>选择一位成员，开始协作。</p>
      </div>
      <div className="sea-office-live">
        <Radio size={14} aria-hidden="true"/>
        <span>{working ? `${working} 位成员在忙` : '办公室在线'}</span>
        <small>成员状态</small>
      </div>
    </header>
    <div className="sea-office-room-label"><Users size={14} aria-hidden="true"/>员工活动区<span>点击员工暂停并打开操作</span></div>
    <div className="office-3d-stage"><OfficeScene3D ref={controls} active={active} members={team} onSelect={choose} onShelf={openLibrary} onReady={onReady} onError={onError}/>
      {!active && <div className="office-3d-status" role="status">正在绘制第一帧…</div>}
      <button className="office-3d-library" onClick={openLibrary} aria-label="打开项目资料馆"><Library size={17}/>项目资料馆 · {artifactCount} 本资料</button>
    </div>
    <details className="office-3d-accessible" open={membersExpanded} onToggle={event => setMembersExpanded(event.currentTarget.open)}><summary><Users size={15} aria-hidden="true"/>团队成员 <span>{team.length}</span></summary><div>{team.map(member => <button key={member.id} data-member-id={member.id} onClick={() => {controls.current?.pauseMember(member.id); choose(member.id);}}><span className="office-member-avatar" aria-hidden="true">{member.name.slice(0, 1)}</span><span className="office-member-copy"><strong>{member.name}</strong><small>{member.responsibility || '项目成员'}</small></span><span className={`office-member-state state-${member.status}`}>{member.status === 'running' ? '工作中' : member.status === 'failed' ? '需关注' : member.status === 'waiting' ? '等待中' : '空闲'}</span></button>)}{!team.length && <p>暂无成员，可在团队管理中添加。</p>}</div></details>
    {!instructionsDismissed && <div className="office-3d-instructions"><span>拖拽平移 · 滚轮缩放 · 点击成员对话</span><button aria-label="关闭操作提示" onClick={() => setInstructionsDismissed(true)}>×</button></div>}
    <div className="office-camera-controls" aria-label="办公室视角"><button className="office-ghost" onClick={() => controls.current?.zoomIn()} aria-label="放大视角">＋</button><button className="office-ghost" onClick={() => controls.current?.zoomOut()} aria-label="缩小视角">−</button><button className="office-ghost" onClick={() => controls.current?.reset()}>复位视角</button></div>
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
