import {useEffect, useState} from 'react';
import {ArrowRight, Layers3, Plus, UsersRound, X} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import {teamApi, type RoleDraft, type TeamSnapshot, type TeamTemplate} from '../../lib/team';
import {RoleEditor, draftWithModel} from './RoleEditor';

type Props = {projectPath: string; projectName: string; team: TeamSnapshot; models: AgentModelOption[]; onCreated: (team: TeamSnapshot) => void; onSkip: (team: TeamSnapshot) => void};

export function TeamOnboarding({projectPath, projectName, team, models, onCreated, onSkip}: Props) {
  const [template, setTemplate] = useState<TeamTemplate | null>(null);
  const [drafts, setDrafts] = useState<RoleDraft[]>([]);
  const [custom, setCustom] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    teamApi.templates(controller.signal).then(result => {
      const selected = result[0] || null;
      setTemplate(selected);
      setDrafts((selected?.roles || []).map(draft => draftWithModel(draft, models)));
    }).catch(reason => {if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));});
    return () => controller.abort();
  }, [projectPath, models]);
  async function create() {
    if (!template || !drafts.length || saving) return;
    setSaving(true); setError('');
    try {
      const created = await teamApi.bootstrap(projectPath, team, custom ? '' : template.id, drafts);
      onCreated(created);
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setSaving(false);}
  }
  async function skip() {
    setSaving(true); setError('');
    try {onSkip(await teamApi.skip(projectPath, team));}
    catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setSaving(false);}
  }
  return <section className="team-onboarding" aria-label="建立项目团队">
    <div className="onboarding-heading"><span className="onboarding-icon"><UsersRound size={25}/></span><span className="office-eyebrow">欢迎来到 {projectName || '新项目'}</span>
      <h1>为你的项目，组建一支小队。</h1><p>一起梳理想法、讨论方案，再把明确的任务交给小涛。每个项目都有独立的团队与对话。</p></div>
    <div className="onboarding-workspace"><nav className="onboarding-tabs" aria-label="建队方式"><button className={!custom ? 'selected' : ''} onClick={() => setCustom(false)}><Layers3 size={16}/>从小队模板开始</button><button className={custom ? 'selected' : ''} onClick={() => {setCustom(true); setDrafts([draftWithModel({name: '', responsibility: '', capabilities: [], model: {modelId: '', contextWindow: 0, maxOutputTokens: 0}, appearance: {shape: 'round', primaryColor: '#36a9c9', secondaryColor: '#173b55'}}, models)]);}}><Plus size={16}/>自由创建</button></nav>
      {!template ? <p role="status">{error || '正在读取可编辑模板…'}</p> : <>
        <div className="onboarding-template"><div><span className="office-eyebrow">{custom ? '你的专属角色' : template.name}</span><h2>{custom ? '从第一位成员开始' : '产品实现小队'}</h2><p>{custom ? '设置姓名、职责、讨论模型和外观，之后随时可以继续添加成员。' : template.description}</p></div><span className="team-count">{drafts.length} 位成员</span></div>
        <div className="onboarding-role-list">{drafts.map((draft, index) => <details className="onboarding-role" key={index} open={custom || drafts.length === 1}><summary><span className="role-color" style={{background: draft.appearance.primaryColor}}/><span><strong>{draft.name || `成员 ${index + 1}`}</strong><small>{draft.responsibility || '设置这位成员的职责与外观'}</small></span><span className="role-edit-label">编辑 <ArrowRight size={14}/></span></summary><RoleEditor draft={draft} models={models} onChange={next => setDrafts(items => items.map((item, at) => at === index ? next : item))}/></details>)}</div>
        {custom && <button className="office-ghost" disabled={drafts.length >= 32} onClick={() => setDrafts(items => [...items, {...items[0], name: '', responsibility: '', capabilities: []}])}><Plus size={15}/>再添加一位成员</button>}
      </>}
      {error && <p className="office-error" role="alert">{error}</p>}
    </div>
    <footer className="onboarding-footer"><button className="office-ghost" disabled={saving} onClick={() => void skip()}><X size={15}/>稍后设置</button><span>模板只创建可编辑角色，不会创建工作流程。</span><button className="office-primary" disabled={saving || !template || !drafts.length} onClick={() => void create()}>{saving ? '正在保存团队…' : '创建我的小队'}<ArrowRight size={17}/></button></footer>
  </section>;
}
