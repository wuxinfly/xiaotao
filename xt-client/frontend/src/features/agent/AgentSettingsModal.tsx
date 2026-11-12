import {useState} from 'react';
import {MCPSettings} from './MCPSettings';
import {SkillsSettings} from './SkillsSettings';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {AgentModelLibrary} from './AgentModelLibrary';
import {UtilityModelPicker} from './UtilityModelPicker';
import type {AgentModelOption} from '../../lib/api';
import type {AgentDefaultModels, AgentModelConfig, AgentModelDefaults, AgentModelSettings} from '../../lib/agent-model-settings';

interface AgentSettingsModalProps {
  models: AgentModelOption[];
  selected: string[];
  modelConfigs: AgentModelConfig[];
  defaultModels: AgentDefaultModels;
  defaultModelSettings: AgentModelDefaults;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onAccounts: () => void;
  onSave: (configs: AgentModelConfig[], defaults: AgentDefaultModels, defaultSettings: AgentModelDefaults) => void;
  onClose: () => void;
}

export function AgentSettingsModal({models, selected, modelConfigs, defaultModels, defaultModelSettings, loading, error, onRetry, onAccounts, onSave, onClose}: AgentSettingsModalProps) {
  const [tab, setTab] = useState<'models' | 'defaults' | 'mcp' | 'skills'>('models');
  const [mcpBusy, setMCPBusy] = useState(false);
  const [skillsBusy, setSkillsBusy] = useState(false);
  const settingsBusy = mcpBusy || skillsBusy;
  const [chosen, setChosen] = useState(() => new Set(selected));
  const [configs, setConfigs] = useState<Record<string, AgentModelConfig>>(() => Object.fromEntries(modelConfigs.map(config => [config.model, config])));
  const [formError, setFormError] = useState('');
  const [defaults, setDefaults] = useState<AgentDefaultModels>(defaultModels);
  const [defaultSettings, setDefaultSettings] = useState<AgentModelDefaults>(defaultModelSettings);
  const selectedCount = chosen.size;

  function configFor(model: string, options: AgentModelOption[]) {
    const saved = configs[model];
    return {
      contextWindow: saved?.contextWindow || options.find(item => item.model === model)?.contextWindow,
      maxOutputTokens: saved?.maxOutputTokens || options.find(item => item.model === model)?.maxOutputTokens,
      reasoningEffort: saved?.reasoningEffort,
    };
  }

  function updateReasoningEffort(model: string, raw: string) {
    const value = raw.trim();
    setFormError('');
    setConfigs(current => ({
      ...current,
      [model]: {
        ...current[model],
        model,
        reasoningEffort: value === '' ? undefined : value,
      },
    }));
  }

  function updateSettings(model: string, key: 'contextWindow' | 'maxOutputTokens', raw: string) {
    const value = raw.trim() === '' ? undefined : Number(raw);
    setFormError('');
    setConfigs(current => ({
      ...current,
      [model]: {
        ...current[model],
        model,
        [key]: Number.isFinite(value as number) && (value as number) > 0 ? Math.floor(value as number) : undefined,
      },
    }));
  }

  function validate(configsToSave: AgentModelConfig[]) {
    for (const config of configsToSave) {
      if (config.contextWindow !== undefined && config.contextWindow < 1024) {
        setFormError(`“${models.find(item => item.model === config.model)?.label || config.model}”的上下文窗口不能小于 1024 tokens。`);
        return false;
      }
      if (config.maxOutputTokens !== undefined && config.maxOutputTokens < 16) {
        setFormError(`“${models.find(item => item.model === config.model)?.label || config.model}”的最大输出不能小于 16 tokens。`);
        return false;
      }
      if (config.contextWindow && config.maxOutputTokens && config.maxOutputTokens >= config.contextWindow) {
        setFormError(`“${models.find(item => item.model === config.model)?.label || config.model}”的最大输出必须小于上下文窗口。`);
        return false;
      }
    }
    return true;
  }

  function save() {
    const nextConfigs = Array.from(chosen).map(model => ({
      model,
      contextWindow: configs[model]?.contextWindow || models.find(item => item.model === model)?.contextWindow,
      maxOutputTokens: configs[model]?.maxOutputTokens || models.find(item => item.model === model)?.maxOutputTokens,
      reasoningEffort: configs[model]?.reasoningEffort,
    }));
    const utilityConfigs = (['title', 'review', 'compaction'] as const).flatMap(role => {const model = defaults[role]; return model ? [{model, ...defaultSettingsFor(model, defaultSettings[role])}] : [];});
    if (!validate([...nextConfigs, ...utilityConfigs])) return;
    onSave(nextConfigs, defaults, defaultSettings);
  }

  function toggleModel(model: string) {
    setFormError('');
    setChosen(current => {
      const next = new Set(current);
      if (next.has(model)) next.delete(model);
      else next.add(model);
      return next;
    });
  }

  function updateDefaultSetting(role: keyof AgentModelDefaults, key: 'contextWindow' | 'maxOutputTokens', raw: string) {
    setFormError('');
    const value = raw.trim() === '' ? undefined : Number(raw);
    setDefaultSettings(current => ({
      ...current,
      [role]: {
        ...current[role],
        [key]: Number.isFinite(value as number) && (value as number) > 0 ? Math.floor(value as number) : undefined,
      },
    }));
  }

  function defaultSettingsFor(model: string, saved?: AgentModelSettings) {
    const configured = configs[model];
    return {
      contextWindow: saved?.contextWindow || configured?.contextWindow || models.find(item => item.model === model)?.contextWindow,
      maxOutputTokens: saved?.maxOutputTokens || configured?.maxOutputTokens || models.find(item => item.model === model)?.maxOutputTokens,
    };
  }

  function close() {
    if (settingsBusy) return;
    setChosen(new Set(selected));
    setConfigs(Object.fromEntries(modelConfigs.map(config => [config.model, config])));
    setDefaults(defaultModels); setDefaultSettings(defaultModelSettings); setFormError('');
    onClose();
  }

  return <ModalFrame
    title="Agent 设置"
    description={tab === 'models' ? '配置可选执行模型；新会话会保存实际使用的参数快照。' : tab === 'defaults' ? '为标题、复盘和上下文压缩选择辅助模型。' : tab === 'mcp' ? '管理 Agent 可以使用的外部工具。' : '为小涛添加本地技能，按任务需要使用。'}
    className="agent-settings-modal"
    onClose={close}
    footer={<>
      <span className="app-modal-count">{tab === 'models' ? `已配置 ${selectedCount} 个模型` : ''}</span>
      <div className="app-modal-actions">
        <button type="button" className="app-modal-button secondary" onClick={close}>取消</button>
        <button
          type="button"
          className="app-modal-button primary"
          disabled={!!formError || (tab === 'models' && !selectedCount)}
          onClick={save}
        >保存设置</button>
      </div>
    </>}
  >
    <div className="agent-settings-layout">
    <nav className="agent-settings-nav" aria-label="设置分类"><button type="button" aria-pressed={tab === 'models'} disabled={settingsBusy} onClick={() => setTab('models')}><strong>常用模型</strong><small>执行模型与参数</small></button><button type="button" aria-pressed={tab === 'defaults'} disabled={settingsBusy} onClick={() => setTab('defaults')}><strong>默认模型</strong><small>标题、复盘、压缩</small></button><button type="button" aria-pressed={tab === 'mcp'} disabled={settingsBusy} onClick={() => setTab('mcp')}><strong>MCP 工具</strong><small>外部工具连接</small></button><button type="button" aria-pressed={tab === 'skills'} disabled={settingsBusy} onClick={() => setTab('skills')}><strong>Skills 技能</strong><small>本地技能目录</small></button></nav>
    <div className="agent-settings-content">
    {formError && <p className="agent-modal-error" role="alert">{formError}</p>}
    {tab === 'mcp' && <div className="agent-settings-page"><MCPSettings active onBusyChange={setMCPBusy}/></div>}
    {tab === 'skills' && <div className="agent-settings-page"><SkillsSettings active onBusyChange={setSkillsBusy}/></div>}
    {tab === 'models' && <div className="agent-settings-page">
    <AgentModelLibrary models={models} chosen={chosen} loading={loading} error={error} onRetry={onRetry} onAccounts={onAccounts} onToggle={toggleModel} onAdd={ids => setChosen(current => new Set([...current, ...ids]))} configFor={id => configFor(id, models)} onSetting={updateSettings} onReasoning={updateReasoningEffort}/>
    </div>}
    {tab === 'defaults' && <div className="agent-settings-page agent-defaults">
      <div className="agent-settings-section-head"><div><strong>默认模型</strong><p>只用于辅助任务，不改变新会话的执行模型。</p></div></div>
      {([
        ['title', 'AI 重命名', '根据对话生成会话标题'],
        ['review', '行为复盘', '分析 Agent 的执行过程'],
        ['compaction', '上下文压缩', '压缩长对话，为上下文腾出空间'],
      ] as const).map(([role, title, description]) => {
        const model = defaults[role] || '';
        const settings = defaultSettingsFor(model, defaultSettings[role]);
        return <section className="agent-default-model" key={role}>
          <div><strong>{title}</strong><p>{description}</p></div>
          <UtilityModelPicker label={`${title}模型`} value={model} models={models} onChange={value => {setFormError(''); setDefaults(current => ({...current, [role]: value || undefined})); setDefaultSettings(current => ({...current, [role]: undefined}));}}/>
          {model && <div className="agent-model-execution">
            <span className="agent-model-execution-title">调用参数</span>
            <label><span>上下文窗口</span><div><input type="number" min="1024" step="1024" inputMode="numeric" value={settings.contextWindow ?? ''} onChange={event => updateDefaultSetting(role, 'contextWindow', event.target.value)} aria-label={`${title}上下文窗口`}/><small>tokens</small></div></label>
            <label><span>最大输出</span><div><input type="number" min="16" step="256" inputMode="numeric" value={settings.maxOutputTokens ?? ''} onChange={event => updateDefaultSetting(role, 'maxOutputTokens', event.target.value)} aria-label={`${title}最大输出`}/><small>tokens</small></div></label>
          </div>}
        </section>;
      })}
    </div>}
    </div>
    </div>
  </ModalFrame>;
}
