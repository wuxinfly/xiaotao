import {ClipboardList, Footprints, MessageCircle, Play} from 'lucide-react';
import {useState} from 'react';
import type {OfficeMember} from '../../lib/office';

type Props = {
  member: OfficeMember;
  index: number;
  onSelectMember: (memberId: string) => void;
  onSelectTask: (taskId: string) => void;
};

const positions = [
  {left: '12%', top: '47%'},
  {left: '38%', top: '61%'},
  {left: '63%', top: '43%'},
  {left: '24%', top: '69%'},
  {left: '72%', top: '66%'},
];

const roleTitles: Record<string, string> = {
  coordinator: '办公室协调',
  designer: '产品设计',
  developer: '开发工程师',
  executor: '任务执行',
};

export function OfficeActor({member, index, onSelectMember, onSelectTask}: Props) {
  const [moving, setMoving] = useState(true);
  const [actionsOpen, setActionsOpen] = useState(false);
  const position = positions[index % positions.length];
  const title = roleTitles[member.role] || (member.role === 'executor' ? '任务执行' : '办公室伙伴');
  const taskId = member.taskId;

  function stopAndOpen() {
    setMoving(false);
    setActionsOpen(true);
  }

  return <div
    className={`office-actor actor-path-${index % positions.length}${moving ? ' is-moving' : ' is-paused'}`}
    style={{left: position.left, top: position.top, animationDuration: `${10 + (index % 3) * 2}s`}}
  >
    <button
      type="button"
      className="office-actor-hit"
      aria-label={`${member.name}，${title}，点击暂停并打开操作`}
      aria-expanded={actionsOpen}
      onClick={stopAndOpen}
    >
      <span className="office-actor-shadow" aria-hidden="true"/>
      <span className="office-actor-body" aria-hidden="true">
        <span className="office-actor-head"/>
        <span className="office-actor-torso"/>
        <span className="office-actor-leg office-actor-leg-left"/>
        <span className="office-actor-leg office-actor-leg-right"/>
      </span>
      <span className="office-actor-label">
        <strong>{member.name}</strong>
        <small>{title}</small>
      </span>
      {!moving && <span className="office-actor-paused"><Footprints size={11}/>已停下</span>}
    </button>
    {actionsOpen && <div className="office-actor-actions" role="menu" aria-label={`${member.name}的操作`}>
      <button type="button" role="menuitem" onClick={() => onSelectMember(member.id)}><MessageCircle size={15}/>对话</button>
      {taskId && <button type="button" role="menuitem" onClick={() => onSelectTask(taskId)}><ClipboardList size={15}/>查看任务</button>}
      <button type="button" role="menuitem" onClick={() => {setActionsOpen(false); setMoving(true);}}><Play size={14}/>继续走动</button>
    </div>}
  </div>;
}
