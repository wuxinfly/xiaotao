import {Archive, Box, Maximize2, Minus, Plus, UsersRound} from 'lucide-react';
import type {OfficeMember, OfficeTask} from '../../lib/office';
import {taskStatusLabel, taskStatusTone} from '../../lib/office';
import './office-3d.css';
import {useMemo, useRef, type ReactNode} from 'react';
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
  onShowTeam: () => void;
  ready: boolean;
  setup?: ReactNode;
  active: boolean;
};

export function OfficeRoom({projectName, members, artifactCount, onSelectMember, onOpenLibrary, onShowTeam, active, ready, setup, onError, onReady}: Props) {
  const controls = useRef<SceneControls>(null);
  const team = useMemo(() => sceneMembers(members), [members]);
  return <section className="office-room office-3d-room" aria-label="办公室">
    <header className="room-toolbar">
      <div className="room-project"><span className="room-project-icon"><Box size={18}/></span><div><strong title={projectName}>{projectName || '项目办公室'}</strong><small>3D 工作空间</small></div></div>
      <div className="room-toolbar-actions">{setup}<button type="button" className="room-tool" onClick={onShowTeam}><UsersRound size={16}/><span>成员</span><b>{team.length}</b></button></div>
    </header>
    <div className="office-3d-stage">
      <OfficeScene3D ref={controls} active={active} members={team} onSelect={onSelectMember} onShelf={onOpenLibrary} onReady={onReady} onError={onError}/>
      {!ready && <div className="office-3d-status" role="status">正在加载 3D 场景…</div>}
    </div>
    <footer className="room-bottom-bar">
      <button type="button" className="room-tool office-3d-library" onClick={onOpenLibrary} aria-label="打开项目资料馆" aria-description={`${artifactCount} 份资料`}><Archive size={16}/><span>资料馆</span><b>{artifactCount}</b></button>
      <span className="room-explore-hint">拖动平移 · 滚轮缩放 · 点击成员互动</span>
      <div className="office-camera-controls" role="group" aria-label="办公室视角"><button disabled={!ready} onClick={() => controls.current?.zoomOut()} aria-label="缩小视角" title="缩小视角"><Minus size={15}/></button><button disabled={!ready} onClick={() => controls.current?.reset()} aria-label="复位视角" title="复位视角"><Maximize2 size={14}/></button><button disabled={!ready} onClick={() => controls.current?.zoomIn()} aria-label="放大视角" title="放大视角"><Plus size={15}/></button></div>
    </footer>
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
