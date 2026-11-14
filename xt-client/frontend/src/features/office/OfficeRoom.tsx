import {Archive, Maximize2, Minus, Plus} from 'lucide-react';
import type {OfficeMember, OfficeTask} from '../../lib/office';
import {taskStatusLabel, taskStatusTone} from '../../lib/office';
import './office-scene.css';
import './office-3d.css';
import {useRef, useState} from 'react';
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
  const [sceneStatus, setSceneStatus] = useState('连接中');
  const controls = useRef<SceneControls>(null);
  const team = sceneMembers(members);
  const choose = (id: string) => onSelectMember(id);
  const openLibrary = () => onOpenLibrary();
  return <section className="office-room sea-office office-3d-room" aria-label="办公室">
    <div className="office-3d-stage"><OfficeScene3D ref={controls} active={active} members={team} onSelect={choose} onShelf={openLibrary} onReady={() => {setSceneStatus('实时同步'); onReady();}} onError={message => {setSceneStatus('场景需重载'); onError(message);}}/>
      <header className="room-toolbar"><div className="room-project"><span className="room-project-icon"><Archive size={17}/></span><div><strong>{projectName || '项目办公室'}</strong><small><i className={active ? 'is-online' : ''}/>{sceneStatus}</small></div></div><span className="room-scene-label">3D 工作空间</span></header>
      <div className="room-caption"><span>{team.length} 位成员 · 随时可以聊聊</span></div>
      {!active && <div className="office-3d-status" role="status">正在准备工作空间…</div>}
      <button className="office-3d-library" onClick={openLibrary} aria-label="打开项目资料馆"><Archive size={16}/><span>项目资料馆</span><small>{artifactCount} 份资料</small></button>
    </div>
    <footer className="room-bottom-bar">
      <span className="room-explore-hint">自由探索你的工作空间</span>
      <div className="office-camera-controls" aria-label="办公室视角"><button onClick={() => controls.current?.zoomOut()} aria-label="缩小视角" title="缩小"><Minus size={14}/></button><button onClick={() => controls.current?.reset()} aria-label="复位视角" title="复位视角"><Maximize2 size={13}/></button><button onClick={() => controls.current?.zoomIn()} aria-label="放大视角" title="放大"><Plus size={14}/></button></div>
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
