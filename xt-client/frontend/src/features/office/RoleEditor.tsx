import {useId, useState} from 'react';
import type {AgentModelOption} from '../../lib/api';
import type {Appearance, RoleDraft} from '../../lib/team';
import {Select} from '../../components/select/Select';
import {RolePreview} from './RolePreview';

export function draftWithModel<T extends RoleDraft>(draft: T, models: AgentModelOption[]): T {
  const model = models.find(item => item.model === draft.model.modelId && !item.unavailable) || models.find(item => !item.unavailable);
  return {...draft, model: {...draft.model, modelId: model?.model || '', contextWindow: draft.model.contextWindow || model?.contextWindow || 0, maxOutputTokens: draft.model.maxOutputTokens || model?.maxOutputTokens || 0}};
}
const labels: Record<keyof Appearance, string> = {shape: '人物造型', primaryColor: '上衣主色', secondaryColor: '下装配色'};
const shapes: {value: Appearance['shape']; label: string}[] = [{value: 'round', label: '圆润'}, {value: 'square', label: '方形'}, {value: 'triangle', label: '三角形'}, {value: 'hexagon', label: '六边形'}];

export function RoleEditor({draft: initial, models, onChange, onSave, onCancel, saving = false, roleId}: {draft: RoleDraft; models: AgentModelOption[]; onChange?: (draft: RoleDraft) => void; onSave?: (draft: RoleDraft) => Promise<void>; onCancel?: () => void; saving?: boolean; roleId?: string}) {
  const [local, setLocal] = useState(initial);
  const [error, setError] = useState('');
  const shapeLabelId = useId();
  const modelLabelId = useId();
  const draft = onChange ? initial : local;
  function update(next: RoleDraft) {if (onChange) onChange(next); else setLocal(next);}
  const model = models.find(item => item.model === draft.model.modelId);
  return <section className="role-editor" aria-busy={saving}><div className="role-editor-fields">
    {roleId && <p className="role-stable-id">稳定角色 ID：<code>{roleId}</code></p>}
    <label>姓名<input required maxLength={40} value={draft.name} onChange={event => update({...draft, name: event.target.value})}/></label>
    <label>职责<textarea required maxLength={240} rows={2} value={draft.responsibility} onChange={event => update({...draft, responsibility: event.target.value})}/></label>
    <label>能力描述<input maxLength={1200} value={draft.capabilities.join('；')} onChange={event => update({...draft, capabilities: event.target.value.split(/[；;]/).map(item => item.trim()).filter(Boolean)})}/></label>
    <label><span id={modelLabelId}>模型</span><Select aria-labelledby={modelLabelId} searchable value={draft.model.modelId} placeholder={!model ? '模型不可用，请选择' : '请选择'}
      onChange={next => update(draftWithModel({...draft, model: {...draft.model, modelId: next, contextWindow: 0, maxOutputTokens: 0}}, models))}
      options={models.filter(item => !item.unavailable).map(item => ({value: item.model, label: item.label}))}/>
      {draft.model.modelId && !model && <small className="office-error" role="status">已保存的模型当前不可用；开始对话前请选择可用模型。</small>}</label>
    <div className="role-model-settings"><label>上下文 tokens<input type="number" min={0} value={draft.model.contextWindow} onChange={event => update({...draft, model: {...draft.model, contextWindow: Number(event.target.value)}})}/></label>
      <label>最大输出 tokens<input type="number" min={0} value={draft.model.maxOutputTokens} onChange={event => update({...draft, model: {...draft.model, maxOutputTokens: Number(event.target.value)}})}/></label></div>
  </div><div className="role-appearance"><RolePreview appearance={draft.appearance} name={draft.name || '角色预览'}/>
    <label><span id={shapeLabelId}>{labels.shape}</span><Select aria-labelledby={shapeLabelId} value={draft.appearance.shape} onChange={next => update({...draft, appearance: {...draft.appearance, shape: next as Appearance['shape']}})} options={shapes}/></label>
    {(['primaryColor', 'secondaryColor'] as const).map(key => <label key={key}>{labels[key]}<input type="color" value={draft.appearance[key]} onChange={event => update({...draft, appearance: {...draft.appearance, [key]: event.target.value}})}/></label>)}
  </div>{error && <p className="office-error" role="alert">{error}</p>}{onSave && <div className="team-actions"><button className="office-primary" disabled={saving} onClick={() => {if (!draft.name.trim() || !draft.responsibility.trim() || !draft.model.modelId) {setError('请填写姓名、职责并选择可用模型。'); return;} void onSave(draft);}}>{saving ? '正在保存…' : '保存角色'}</button><button className="office-ghost" disabled={saving} onClick={onCancel}>取消</button></div>}</section>;
}
