import type {SourceInput} from '../../lib/model-sources';

type Model = SourceInput['models'][number];
export const emptyModel = (): Model => ({upstreamModelId: '', label: '', supportsImages: false, supportsReasoning: false});

export function SourceModelFields({model, index, removable, onChange, onRemove}: {model: Model; index: number; removable: boolean; onChange: (model: Model) => void; onRemove: () => void}) {
  const prefix = `模型 ${index + 1}`;
  const numberField = (key: 'contextWindow' | 'maxOutputTokens', title: string) => <label><span>{title}</span><input aria-label={`${prefix} ${title}`} type="number" min="1" step="1" value={model[key] ?? ''} onChange={event => onChange({...model, [key]: event.target.value ? Number(event.target.value) : undefined})} placeholder="未声明"/></label>;
  return <fieldset className="source-model-fields">
    <legend>{prefix}</legend>
    <div className="source-form-grid">
      <label><span>上游模型 ID</span><input aria-label={`${prefix} 上游模型 ID`} required value={model.upstreamModelId} onChange={event => onChange({...model, upstreamModelId: event.target.value})} placeholder="例如 org/model"/></label>
      <label><span>显示名</span><input aria-label={`${prefix} 显示名`} value={model.label} onChange={event => onChange({...model, label: event.target.value})} placeholder="留空使用上游模型 ID"/></label>
      {numberField('contextWindow', '上下文窗口')}{numberField('maxOutputTokens', '最大输出')}
    </div>
    <div className="source-model-options">
      <label><input type="checkbox" checked={model.supportsImages} onChange={event => onChange({...model, supportsImages: event.target.checked})}/>支持图片</label>
      <label><input type="checkbox" checked={model.supportsReasoning} onChange={event => onChange({...model, supportsReasoning: event.target.checked, reasoningEfforts: event.target.checked ? model.reasoningEfforts : undefined})}/>支持推理</label>
      <button type="button" className="app-modal-button secondary" disabled={!removable} onClick={onRemove} aria-label={`删除${prefix}`}>删除模型</button>
    </div>
    {model.supportsReasoning && <label><span>推理档位（可选，逗号分隔）</span><input aria-label={`${prefix} 推理档位`} value={model.reasoningEfforts?.join(',') ?? ''} onChange={event => onChange({...model, reasoningEfforts: event.target.value.split(/[,，]/)})} placeholder="low, medium, high"/></label>}
  </fieldset>;
}
