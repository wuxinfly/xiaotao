import {useCallback, useEffect, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {toast} from '../../components/toast';
import type {AgentModelOption} from '../../lib/api';
import {teamApi, defaultAppearance, type RoleDraft, type TeamSnapshot, type TeamTemplate} from '../../lib/team';
import {RoleEditor, draftWithModel} from './RoleEditor';

type Props = {projectPath: string; models: AgentModelOption[]; onChanged: (team: TeamSnapshot) => void | Promise<void>; onClose: () => void};

export function TeamManager({projectPath, models, onChanged, onClose}: Props) {
  const confirm = useConfirm();
  const [team, setTeam] = useState<TeamSnapshot | null>(null);
  const [template, setTemplate] = useState<TeamTemplate | null>(null);
  const [drafts, setDrafts] = useState<RoleDraft[]>([]);
  const [editing, setEditing] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const load = useCallback(async () => {
    try {setTeam(await teamApi.snapshot(projectPath)); setError('');}
    catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
  }, [projectPath]);
  useEffect(() => {
    void load();
    void teamApi.templates().then(items => {
      const selected = items[0] || null;
      setTemplate(selected); setDrafts(selected?.roles || []);
    }).catch(reason => setError(String(reason)));
  }, [load]);
  async function commit(action: () => Promise<TeamSnapshot>, message: string) {
    setSaving(true); setError('');
    try {const next = await action(); setTeam(next); await onChanged(next); setEditing(''); setCreating(false); toast.success(message);}
    catch (reason) {const message = String(reason); setError(message); if (message.includes('修改')) await load();}
    finally {setSaving(false);}
  }
  async function remove(id: string) {
    if (!team || !await confirm({title: '移出这位成员？', description: '只结束成员关系，不删除对话、任务或产物。运行中的成员需要先停止对话。', confirmLabel: '移出成员'})) return;
    await commit(() => teamApi.remove(projectPath, team, id), '成员已移出，历史记录仍保留');
  }
  async function editGlobal(role: RoleDraft) {
    if (!team || !editing.startsWith('global:') || !await confirm({title: '更新所有项目中的角色？', description: '名称、职责、模型和外观会同步到该角色已加入的所有项目，不会复制项目对话或任务。', confirmLabel: '保存全局修改'})) return;
    await commit(() => teamApi.update(projectPath, team, editing.slice(7), role), '全局角色已更新');
  }
  if (!team || !template) return <ModalFrame title="管理团队" footer={null} onClose={onClose}><p>{error || '正在读取团队…'}</p></ModalFrame>;
  if (!team.members.length && !creating) return <ModalFrame title="建立项目团队" description="选择模板，或直接创建自己的角色。" className="team-manager-modal" footer={null} onClose={onClose}>
    <section className="team-template"><h3>{template.name}</h3><p>{template.description}</p>
      {drafts.map((draft, index) => <RoleEditor key={index} draft={draft} models={models} onChange={next => setDrafts(items => items.map((item, at) => at === index ? next : item))}/>)}
      <div className="team-actions"><button className="office-primary" disabled={saving} onClick={() => void commit(() => teamApi.bootstrap(projectPath, team, template.id, drafts), '项目团队已建立')}>创建可编辑小队</button>
        <button className="office-ghost" onClick={() => setCreating(true)}>自由创建角色</button></div>
    </section>
  </ModalFrame>;
  const editMember = team.members.find(member => member.id === editing);
  const editGlobalRole = team.globalRoles.find(role => `global:${role.id}` === editing);
  return <ModalFrame title="管理团队" description="角色提供讨论身份和外观，不独立执行任务。" className="team-manager-modal" footer={null} onClose={onClose}>
    {error && <p className="office-error" role="alert">{error}</p>}
    <div className="team-member-list">{team.members.map(member => <article key={member.id}>
      <header><h3>{member.name}</h3><span>{member.scope === 'global' ? '全局角色 · 配置共享' : '项目角色'}</span></header>
      <p>{member.responsibility}</p><small>{member.capabilities.join(' · ')}</small>
      <div className="team-actions"><button className="office-ghost" onClick={() => setEditing(member.id)}>编辑</button>
        {member.scope === 'project' && <button className="office-ghost" onClick={async () => {if (await confirm({title: '提升为全局角色？', description: '保留角色 ID 和当前成员关系，其他项目可邀请此角色；项目经历和对话不会共享。', confirmLabel: '提升为全局'})) await commit(() => teamApi.promote(projectPath, team, member.id), '已提升为全局角色');}}>提升为全局</button>}
        {member.scope === 'global' && <button className="office-ghost" onClick={() => void commit(() => teamApi.archive(projectPath, team, member.id), '未引用角色已归档')}>归档全局角色</button>}
        <button className="office-ghost" onClick={() => void remove(member.id)}>移出项目</button></div>
    </article>)}</div>
    <section className="team-invite"><h3>邀请全局角色</h3><p>只共享角色配置，不复制任何项目历史。</p>
      {team.globalRoles.filter(role => !team.members.some(member => member.id === role.id)).map(role => <div key={role.id}><button className="office-ghost" onClick={() => void commit(() => teamApi.invite(projectPath, team, role.id), '角色已加入项目')}>邀请 {role.name}</button><button className="office-ghost" onClick={() => setEditing(`global:${role.id}`)}>编辑全局角色</button><button className="office-ghost" onClick={async () => {if (await confirm({title: '归档全局角色？', description: '仅当没有任何项目引用时才允许归档。历史记录会保留，不会级联删除项目成员。', confirmLabel: '归档角色'})) await commit(() => teamApi.archive(projectPath, team, role.id), '全局角色已归档');}}>归档</button></div>)}
    </section>
    {(creating || editMember || editGlobalRole) && <RoleEditor roleId={editMember?.id || editGlobalRole?.id} saving={saving} draft={editMember || editGlobalRole ? draftWithModel(editMember || editGlobalRole!, models) : draftWithModel({name: '', responsibility: '', capabilities: [], model: {modelId: '', contextWindow: 0, maxOutputTokens: 0}, appearance: defaultAppearance}, models)} models={models} onSave={draft => {
      if (editGlobalRole) return editGlobal(draft);
      if (editMember) return commit(() => teamApi.update(projectPath, team, editMember.id, draft), '角色已更新');
      return commit(() => teamApi.create(projectPath, team, 'project', draft), '角色已创建');
    }} onCancel={() => {setEditing(''); setCreating(false);}}/>}
    {!creating && !editMember && !editGlobalRole && <button className="office-primary" onClick={() => setCreating(true)}>自由创建角色</button>}
  </ModalFrame>;
}
