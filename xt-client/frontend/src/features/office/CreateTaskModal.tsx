import {useMemo, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import type {AgentModelOption} from '../../lib/api';
import {modelSettingsFor} from '../../lib/agent-model-settings';
import type {AgentModelConfig} from '../../lib/agent-model-settings';
import type {OfficeTask} from '../../lib/office';
import {officeApi} from '../../lib/office';

type Props = {
  projectPath: string;
  projectName: string;
  models: AgentModelOption[];
  modelConfigs: AgentModelConfig[];
  preferredModel: string;
  initialGoal?: string;
  onCreated: (task: OfficeTask) => void;
  onClose: () => void;
};

// CreateTaskModal is the one place a user starts work in the office. It only
// submits a goal and an executor model; the server creates the real session.
export function CreateTaskModal({projectPath, projectName, models, modelConfigs, preferredModel, initialGoal = '', onCreated, onClose}: Props) {
  const usable = useMemo(() => models.filter(model => !model.unavailable), [models]);
  const [goal, setGoal] = useState(initialGoal);
  const [title, setTitle] = useState('');
  const [model, setModel] = useState(() => usable.find(item => item.model === preferredModel)?.model || usable[0]?.model || '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const canSubmit = !!goal.trim() && !!model && !busy;

  async function submit() {
    if (!canSubmit) return;
    setBusy(true);
    setError('');
    try {
      const task = await officeApi.createTask({
        projectPath,
        projectName,
        title: title.trim(),
        goal: goal.trim(),
        model,
        modelSettings: modelSettingsFor(modelConfigs, model, models),
      });
      onCreated(task);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }

  return <ModalFrame
    title="交给小涛一个任务"
    description={`项目：${projectName || projectPath}。小涛会作为负责人安排一个执行者在这个项目里干活。`}
    className="office-create-modal"
    onClose={() => { if (!busy) onClose(); }}
    footer={<div className="app-modal-actions">
      <button type="button" className="app-modal-button secondary" disabled={busy} onClick={onClose}>取消</button>
      <button type="button" className="app-modal-button primary" disabled={!canSubmit} onClick={() => void submit()}>
        {busy ? '安排中…' : '开始任务'}
      </button>
    </div>}
  >
    <label className="office-field">
      <span>任务目标</span>
      <textarea value={goal} onChange={event => setGoal(event.target.value)} rows={4} maxLength={20000}
        placeholder="说清楚要做什么、做到什么程度。执行者会直接在这个项目目录里工作。"
        disabled={busy} autoFocus/>
    </label>
    <label className="office-field">
      <span>标题（可选）</span>
      <input value={title} onChange={event => setTitle(event.target.value)} maxLength={100} disabled={busy}
        placeholder="不填就用目标开头自动生成"/>
    </label>
    <label className="office-field">
      <span>执行者模型</span>
      {usable.length
        ? <select value={model} onChange={event => setModel(event.target.value)} disabled={busy}>
            {usable.map(item => <option key={item.model} value={item.model}>{item.label}{item.sourceLabel ? ` · ${item.sourceLabel}` : ''}</option>)}
          </select>
        : <p className="office-hint" role="alert">还没有可用模型，先去左下角「模型接口」添加接口和模型。</p>}
    </label>
    <p className="office-hint">执行中不能直接打断执行者聊天。你可以发消息，它会排队，在本轮成功结束后送达；失败或取消后不会自动补发。</p>
    {error && <p className="office-error" role="alert">{error}</p>}
  </ModalFrame>;
}
