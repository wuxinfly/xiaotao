import {MessageCircle, Settings2} from 'lucide-react';
import type {OfficeMember} from '../../lib/office';

export function TeamList({members, onSelect, onManage}: {members: OfficeMember[]; onSelect: (id: string) => void; onManage: () => void}) {
  return <section className="team-fallback" aria-label="二维团队列表">
    <header><h2>项目团队</h2><button className="office-ghost" onClick={onManage}><Settings2 size={15}/>管理团队</button></header>
    {members.length ? <div className="team-fallback-grid">{members.map(member => <button key={member.id} data-member-id={member.id} onClick={() => onSelect(member.id)}>
      <i className={`team-presence status-${member.status}`} aria-hidden="true"/><strong>{member.name}</strong><span>{member.responsibility || '项目成员'}</span><small>{member.status === 'running' ? '工作中' : member.status === 'waiting' ? '等待确认' : member.status === 'failed' ? '需关注' : '空闲'} · 对话</small><MessageCircle size={16} aria-hidden="true"/>
    </button>)}</div> : <p>还没有团队成员。可自由创建角色，或使用可编辑的小队模板。</p>}
  </section>;
}
