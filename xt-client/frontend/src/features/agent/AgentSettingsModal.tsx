import {useMemo, useState} from 'react';
import {ModelPriceBadge} from '../../components/ModelPriceBadge';
import {MCPSettings} from './MCPSettings';
import {SkillsSettings} from './SkillsSettings';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {ChevronDown, Plus, Search, Trash2, X} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import type {AgentDefaultModels, AgentModelConfig, AgentModelDefaults, AgentModelSettings} from '../../lib/agent-model-settings';

const providerNames: Record<string, string> = {
  trae: 'Trae',
  workbuddy: 'WorkBuddy',
  antigravity: 'Antigravity',
  qoder: 'Qoder',
  codex: 'Codex',
};

function splitModel(model: string) {
  const slash = model.indexOf('/');
  if (slash < 0) return {provider: 'other', name: model};
  return {provider: model.slice(0, slash), name: model.slice(slash + 1)};
}

function providerLabel(provider: string) {
  return providerNames[provider] || provider;
}

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
  const [query, setQuery] = useState('');
  const [chosen, setChosen] = useState(() => new Set(selected));
  const [configs, setConfigs] = useState<Record<string, AgentModelConfig>>(() => Object.fromEntries(modelConfigs.map(config => [config.model, config])));
  const [formError, setFormError] = useState('');
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [adding, setAdding] = useState(false);
  const [defaults, setDefaults] = useState<AgentDefaultModels>(defaultModels);
  const [defaultSettings, setDefaultSettings] = useState<AgentModelDefaults>(defaultModelSettings);
  const visibleModels = useMemo(() => {
    const term = query.trim().toLowerCase();
    if (!term) return models;
    return models.filter(item => `${item.label} ${item.model}`.toLowerCase().includes(term));
  }, [models, query]);
  const groups = useMemo(() => {
    const grouped = new Map<string, AgentModelOption[]>();
    for (const item of visibleModels) {
      const provider = splitModel(item.model).provider;
      const list = grouped.get(provider) || [];
      list.push(item);
      grouped.set(provider, list);
    }
    return Array.from(grouped.entries());
  }, [visibleModels]);
  const selectedCount = chosen.size;
  const selectedModels = useMemo(() => models.filter(item => chosen.has(item.model)), [models, chosen]);

  function configFor(model: string, options: AgentModelOption[]) {
    const saved = configs[model];
    return {
      contextWindow: saved?.contextWindow || options.find(item => item.model === model)?.contextWindow,
      maxOutputTokens: saved?.maxOutputTokens,
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
      maxOutputTokens: configs[model]?.maxOutputTokens,
      reasoningEffort: configs[model]?.reasoningEffort,
    }));
    if (!validate(nextConfigs)) return;
    onSave(nextConfigs, defaults, defaultSettings);
  }

  function toggleModel(model: string) {
    setChosen(current => {
      const next = new Set(current);
      if (next.has(model)) next.delete(model);
      else next.add(model);
      return next;
    });
  }

  function updateDefaultSetting(role: keyof AgentModelDefaults, key: 'contextWindow' | 'maxOutputTokens', raw: string) {
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
      maxOutputTokens: saved?.maxOutputTokens || configured?.maxOutputTokens,
    };
  }

  return <ModalFrame
    title="Agent 设置"
    description={tab === 'models' ? '配置可选执行模型；新会话会保存实际使用的参数快照。' : tab === 'defaults' ? '为标题、复盘和上下文压缩选择辅助模型。' : tab === 'mcp' ? '管理 Agent 可以使用的外部工具。' : '为小涛添加本地技能，按任务需要使用。'}
    className="agent-settings-modal"
    onClose={() => {if (!settingsBusy) onClose();}}
    footer={<>
      <span className="app-modal-count">{tab === 'models' ? `已配置 ${selectedCount} 个模型` : ''}</span>
      <div className="app-modal-actions">
        <button type="button" className="app-modal-button secondary" onClick={onClose}>取消</button>
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
    {tab === 'mcp' && <div className="agent-settings-page"><MCPSettings active onBusyChange={setMCPBusy}/></div>}
    {tab === 'skills' && <div className="agent-settings-page"><SkillsSettings active onBusyChange={setSkillsBusy}/></div>}
    {tab === 'models' && <div className="agent-settings-page">
    {formError && <p className="agent-modal-error" role="alert">{formError}</p>}
    <div className="agent-model-refresh"><span>模型列表已缓存，账号或模型变化后可手动刷新。</span><button type="button" className="app-modal-button secondary" disabled={loading} onClick={onRetry}>{loading ? '刷新中…' : '刷新模型列表'}</button></div>
    <div className="agent-settings-section-head"><div><strong>执行模型</strong><p>这些模型会出现在新会话的模型选择中。</p></div><button type="button" className="app-modal-button primary" disabled={!models.length} onClick={() => {setAdding(true); setQuery(''); setExpanded(new Set());}}><Plus aria-hidden="true"/>添加模型</button></div>
    {selectedModels.length ? <div className="agent-configured-models">
      {selectedModels.map(item => {
        const config = configFor(item.model, models);
        const name = item.label || splitModel(item.model).name;
        return <article className="agent-configured-model" key={item.model}>
          <header><div><strong>{name}<ModelPriceBadge {...item}/></strong><code>{item.model}</code></div><button type="button" onClick={() => setChosen(current => {const next = new Set(current); next.delete(item.model); return next;})} aria-label={`移除 ${name}`} title="移除模型"><Trash2 aria-hidden="true"/></button></header>
          <div className="agent-model-execution">
            <span className="agent-model-execution-title">执行参数</span>
            <label><span>上下文窗口</span><div><input type="number" min="1024" step="1024" inputMode="numeric" value={config.contextWindow ?? ''} onChange={event => updateSettings(item.model, 'contextWindow', event.target.value)} placeholder={item.contextWindow ? String(item.contextWindow) : '未知'} aria-label={`${name} 上下文窗口`}/><small>tokens</small></div></label>
            <label><span>最大输出</span><div><input type="number" min="16" step="256" inputMode="numeric" value={config.maxOutputTokens ?? ''} onChange={event => updateSettings(item.model, 'maxOutputTokens', event.target.value)} placeholder="跟随上游" aria-label={`${name} 最大输出`}/><small>tokens</small></div></label>
            {item.supportsReasoning && <label><span>思考等级</span><div><select value={config.reasoningEffort ?? ''} onChange={event => updateReasoningEffort(item.model, event.target.value)} aria-label={`${name} 思考等级`}><option value="">跟随上游</option>{(item.reasoningEfforts || []).map(effort => <option key={effort} value={effort}>{effort}</option>)}</select></div></label>}
          </div>
        </article>;
      })}
    </div> : <div className="agent-modal-empty"><p>还没有配置执行模型。</p><button type="button" className="app-modal-button secondary" disabled={!models.length} onClick={() => setAdding(true)}>添加第一个模型</button></div>}
    {adding && <section className="agent-add-model-panel" aria-label="从模型列表添加">
      <div className="agent-add-model-head"><div><strong>添加执行模型</strong><small>从当前账号的模型目录中选择</small></div><button type="button" onClick={() => setAdding(false)} aria-label="关闭模型添加"><X aria-hidden="true"/></button></div>
    <label className="agent-model-search">
      <Search aria-hidden="true"/>
      <input
        value={query}
        onChange={event => { const value = event.target.value; setQuery(value); const term = value.trim().toLowerCase(); setExpanded(new Set(term ? models.filter(item => `${item.label} ${item.model}`.toLowerCase().includes(term) && !chosen.has(item.model)).map(item => splitModel(item.model).provider) : [])); }}
        placeholder="搜索模型或来源"
        aria-label="搜索待添加模型"
        autoComplete="off"
      />
      {query && <button type="button" onClick={() => {setQuery(''); setExpanded(new Set());}} aria-label="清除搜索"><X aria-hidden="true"/></button>}
    </label>
    <div className="agent-settings-groups">
      {loading && <div className="agent-modal-empty" role="status">正在从已启用的账号获取模型，可能需要一些时间…</div>}
      {error && <div className="agent-modal-empty" role="alert"><p>模型列表加载失败：{error}。已选模型仍会保留。</p><button type="button" className="app-modal-button secondary" onClick={onRetry}>重试</button> <button type="button" className="app-modal-button secondary" onClick={onAccounts}>查看账号与配额</button></div>}
      {!loading && !error && !models.length && <div className="agent-modal-empty"><p>没有获取到可用模型，请检查账号是否已接入并启用。</p><button type="button" className="app-modal-button secondary" onClick={onAccounts}>前往账号与配额</button> <button type="button" className="app-modal-button secondary" onClick={onRetry}>重新获取</button></div>}
      {models.length > 0 && !visibleModels.length && <div className="agent-modal-empty">没有匹配的模型。</div>}
      {groups.map(([provider, items]) => {
        const addable = items.filter(item => !chosen.has(item.model));
        if (!addable.length) return null;
        const open = expanded.has(provider);
        return <section className="agent-settings-group" key={provider}>
          <div className="agent-settings-group-head">
            <button type="button" className="agent-settings-group-toggle" aria-expanded={open} onClick={() => setExpanded(current => { const next = new Set(current); if (open) next.delete(provider); else next.add(provider); return next; })}><ChevronDown className={open ? 'is-open' : ''} aria-hidden="true"/><strong>{providerLabel(provider)}</strong><small>{addable.length} 个可添加</small></button>
          </div>
          {open && <div className="agent-model-options">
            {addable.map(item => {
              const {name} = splitModel(item.model);
              return <div className="agent-model-config-card" key={item.model}>
                <button type="button" className="agent-model-choice" onClick={() => {toggleModel(item.model); setAdding(false);}}>
                  <span className="agent-model-check"><Plus aria-hidden="true"/></span>
                  <span className="agent-model-choice-copy">
                    <strong>{item.label || name}<ModelPriceBadge {...item}/></strong>
                    <code>{item.model}</code>
                  </span>
                </button>
              </div>;
            })}
          </div>}
        </section>;
      })}
    </div>
    </section>}
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
          <label className="agent-default-select"><span>{title}模型</span><select value={model} onChange={event => {const value = event.target.value; setDefaults(current => ({...current, [role]: value || undefined})); setDefaultSettings(current => ({...current, [role]: undefined}));}}><option value="">跟随会话模型</option>{models.map(item => <option key={item.model} value={item.model}>{item.label || item.model}</option>)}</select></label>
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
