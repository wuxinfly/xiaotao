import {useMemo, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Select} from '../../components/select/Select';
import type {AgentModelOption} from '../../lib/api';
import {modelSettingsFor} from '../../lib/agent-model-settings';
import type {AgentModelConfig} from '../../lib/agent-model-settings';
import type {OfficeTask} from '../../lib/office';
import {officeApi} from '../../lib/office';
import type {OfficeMember} from '../../lib/office';
import './member-task.css';

type Props = {
  projectPath: string;
  projectName: string;
  models: AgentModelOption[];
  modelConfigs: AgentModelConfig[];
  preferredModel: string;
  members: OfficeMember[];
  assignment: {memberId: string; chatSessionId: string} | null;
  accessMode: 'safe' | 'full';
  onCreated: (task: OfficeTask) => void;
  onClose: () => void;
};

// CreateTaskModal is the one place a user starts work in the office. It only
// submits a goal and an executor model; the server creates the real session.
export function CreateTaskModal({projectPath, projectName, models, modelConfigs, preferredModel, members, assignment, accessMode, onCreated, onClose}: Props) {
  const usable = useMemo(() => models.filter(model => !model.unavailable), [models]);
  const assignable = useMemo(() => {
    const seen = new Set<string>();
    return members.filter(member => member.role !== 'executor' && (!member.taskId || ['completed', 'failed', 'canceled'].includes(member.taskStatus || '')) && !seen.has(member.id) && seen.add(member.id));
  }, [members]);
  const initialMember = assignment?.memberId || 'xiaotao';
  const preferred = members.find(member => member.id === initialMember)?.modelPreference?.modelId || preferredModel;
  const [goal, setGoal] = useState('');
  const [title, setTitle] = useState('');
  const [memberId, setMemberId] = useState(initialMember);
  const [model, setModel] = useState(() => usable.find(item => item.model === preferred)?.model || usable[0]?.model || '');
  const selectedMember = members.find(member => member.id === memberId);
  const executorName = selectedMember?.name || '小涛';
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
        memberId,
        chatSessionId: assignment?.chatSessionId,
      });
      onCreated(task);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }

  return <ModalFrame
    title={`交给${executorName}一个任务`}
    description={`项目：${projectName || projectPath}。确认后，${executorName}会在这个项目里执行任务。`}
    className="office-create-modal"
    onClose={() => { if (!busy) onClose(); }}
    footer={<div className="app-modal-actions">
      <button type="button" className="app-modal-button secondary" disabled={busy} onClick={onClose}>取消</button>
      <button type="button" className="app-modal-button primary" disabled={!canSubmit} onClick={() => void submit()}>
        {busy ? '创建中…' : '确认并开始'}
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
      <span>执行成员</span>
      <Select searchable value={memberId} disabled={busy || !!assignment} aria-label="执行成员" onChange={value => {
        setMemberId(value);
        const next = members.find(member => member.id === value)?.modelPreference?.modelId || preferredModel;
        setModel(usable.find(item => item.model === next)?.model || usable[0]?.model || '');
      }} options={assignable.map(member => ({value: member.id, label: `${member.name}${member.responsibility ? ` · ${member.responsibility}` : ''}`}))}/>
    </label>
    {assignment?.chatSessionId && <div className="member-task-context"><strong>交接当前讨论</strong><p>由服务端读取最近 12 条讨论，总长度不超过 12,000 字符。任务创建后不再同步之后的闲聊。</p></div>}
    <p className="member-task-access">{accessMode === 'safe' ? '当前为安全模式：可以使用项目工具，高危操作需你确认。' : '当前为完全访问模式：可以自主使用项目工具，请确认任务范围。'} 职责和能力标签不会带来额外权限。</p>
    <label className="office-field">
      <span>标题（可选）</span>
      <input value={title} onChange={event => setTitle(event.target.value)} maxLength={100} disabled={busy}
        placeholder="不填就用目标开头自动生成"/>
    </label>
    <label className="office-field">
      <span>执行者模型</span>
      {usable.length
        ? <Select searchable value={model} onChange={setModel} disabled={busy}
            options={usable.map(item => ({value: item.model, label: `${item.label}${item.sourceLabel ? ` · ${item.sourceLabel}` : ''}`}))}/>
        : <p className="office-hint" role="alert">还没有可用模型，先去左下角「模型接口」添加接口和模型。</p>}
    </label>
    <p className="office-hint">执行中不能直接打断执行者聊天。你可以发消息，它会排队，在本轮成功结束后送达；失败或取消后不会自动补发。</p>
    {error && <p className="office-error" role="alert">{error}</p>}
  </ModalFrame>;
}
