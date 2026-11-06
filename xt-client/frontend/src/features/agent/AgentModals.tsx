import {useEffect, useMemo, useState} from 'react';
import {ModelPriceBadge} from '../../components/ModelPriceBadge';
import {MCPSettings} from './MCPSettings';
import {SkillsSettings} from './SkillsSettings';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Check, ChevronDown, FolderOpen, Search, X} from 'lucide-react';
import {api, type AgentConversationReference, type AgentModelOption} from '../../lib/api';

declare global {
  interface Window {
    xtPickFolder?: () => Promise<string>;
    xtPickFolderOnly?: () => void;
    xtFolderPicked?: (payload: {path?: string; error?: string}) => void;
  }
}

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

function projectNameFromPath(path: string) {
  return path.split(/[\\/]/).filter(Boolean).pop() || '';
}

export function RenameSessionModal({title, origin, onSave, onClose, project = false}: {project?: boolean; title: string; origin?: {x: number; y: number}; onSave: (value: string) => Promise<void>; onClose: () => void}) {
 const [value, setValue] = useState(title), [saving, setSaving] = useState(false), [error, setError] = useState('');
 const submit = async () => { if (saving || !value.trim()) return; setSaving(true); setError(''); try { await onSave(value.trim()); } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); } finally {setSaving(false);} };
 return <ModalFrame origin={origin} title={project ? "重命名项目" : "重命名会话"} className="agent-rename-modal" onClose={() => {if (!saving) onClose();}} footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" disabled={saving} onClick={onClose}>取消</button><button type="button" className="app-modal-button primary" disabled={saving || !value.trim() || Array.from(value.trim()).length > 100} onClick={() => void submit()}>{saving ? '保存中…' : '保存'}</button></div>}><label className="agent-field"><span>{project ? "项目名称" : "会话名称"}</span><input value={value} onChange={event => setValue(event.target.value)} onKeyDown={event => {if(event.key === 'Enter' && !event.nativeEvent.isComposing) void submit();}}/></label>{error && <p className="agent-modal-error" role="alert">{error}</p>}</ModalFrame>;
}

interface AgentSettingsModalProps {
  models: AgentModelOption[];
  selected: string[];
  loading: boolean;
  error: string;
  onRetry: () => void;
  onAccounts: () => void;
  onSave: (models: string[]) => void;
  onClose: () => void;
}

export function AgentSettingsModal({models, selected, loading, error, onRetry, onAccounts, onSave, onClose}: AgentSettingsModalProps) {
  const [tab, setTab] = useState<'models' | 'mcp' | 'skills'>('models');
  const [mcpBusy, setMCPBusy] = useState(false);
  const [skillsBusy, setSkillsBusy] = useState(false);
  const settingsBusy = mcpBusy || skillsBusy;
  const [query, setQuery] = useState('');
  const [chosen, setChosen] = useState(() => new Set(selected));
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
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
  const availableIDs = useMemo(() => new Set(models.map(item => item.model)), [models]);
  const selectedCount = Array.from(chosen).filter(id => availableIDs.has(id)).length;

  function toggleModel(model: string) {
    setChosen(current => {
      const next = new Set(current);
      if (next.has(model)) next.delete(model);
      else next.add(model);
      return next;
    });
  }

  function toggleGroup(items: AgentModelOption[]) {
    const allSelected = items.every(item => chosen.has(item.model));
    setChosen(current => {
      const next = new Set(current);
      for (const item of items) {
        if (allSelected) next.delete(item.model);
        else next.add(item.model);
      }
      return next;
    });
  }

  return <ModalFrame
    title="Agent 设置"
    description={tab === 'models' ? '只把常用模型留在输入区，选择切换会更清爽。' : tab === 'mcp' ? '管理 Agent 可以使用的外部工具。' : '为小涛添加本地技能，按任务需要使用。'}
    className="agent-settings-modal"
    onClose={() => {if (!settingsBusy) onClose();}}
    footer={tab !== 'models' ? <div className="app-modal-actions"><button type="button" className="app-modal-button secondary" disabled={settingsBusy} onClick={onClose}>关闭</button></div> : <>
      <span className="app-modal-count">已选 {selectedCount} 个模型</span>
      <div className="app-modal-actions">
        <button type="button" className="app-modal-button secondary" onClick={onClose}>取消</button>
        <button
          type="button"
          className="app-modal-button primary"
          disabled={!selectedCount}
          onClick={() => onSave(models.filter(item => chosen.has(item.model)).map(item => item.model))}
        >保存设置</button>
      </div>
    </>}
  >
    <div className="agent-settings-tabs" role="group" aria-label="设置分类"><button type="button" aria-pressed={tab === 'models'} disabled={settingsBusy} onClick={() => setTab('models')}>常用模型</button><button type="button" aria-pressed={tab === 'mcp'} disabled={settingsBusy} onClick={() => setTab('mcp')}>MCP 工具</button><button type="button" aria-pressed={tab === 'skills'} disabled={settingsBusy} onClick={() => setTab('skills')}>Skills 技能</button></div>
    <div hidden={tab !== 'mcp'}><MCPSettings active={tab === 'mcp'} onBusyChange={setMCPBusy}/></div>
    <div hidden={tab !== 'skills'}><SkillsSettings active={tab === 'skills'} onBusyChange={setSkillsBusy}/></div>
    <div hidden={tab !== 'models'}>
    <div className="agent-model-refresh"><span>模型列表已缓存，账号或模型变化后可手动刷新。</span><button type="button" className="app-modal-button secondary" disabled={loading} onClick={onRetry}>{loading ? '刷新中…' : '刷新模型列表'}</button></div>
    <label className="agent-model-search">
      <Search aria-hidden="true"/>
      <input
        value={query}
        onChange={event => { const value = event.target.value; setQuery(value); const term = value.trim().toLowerCase(); setExpanded(new Set(term ? models.filter(item => `${item.label} ${item.model}`.toLowerCase().includes(term)).map(item => splitModel(item.model).provider) : [])); }}
        placeholder="搜索模型或来源"
        aria-label="搜索模型"
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
        const allSelected = items.every(item => chosen.has(item.model));
        const open = expanded.has(provider);
        const groupSelected = items.filter(item => chosen.has(item.model)).length;
        return <section className="agent-settings-group" key={provider}>
          <div className="agent-settings-group-head">
            <button type="button" className="agent-settings-group-toggle" aria-expanded={open} onClick={() => setExpanded(current => { const next = new Set(current); if (open) next.delete(provider); else next.add(provider); return next; })}><ChevronDown className={open ? 'is-open' : ''} aria-hidden="true"/><strong>{providerLabel(provider)}</strong><small>{items.length} 个 · 已选 {groupSelected}</small></button>
            {open && <button type="button" onClick={() => toggleGroup(items)}>{allSelected ? '取消全选' : '全选'}</button>}
          </div>
          {open && <div className="agent-model-options">
            {items.map(item => {
              const {name} = splitModel(item.model);
              const active = chosen.has(item.model);
              return <button
                type="button"
                className={'agent-model-choice' + (active ? ' is-selected' : '')}
                key={item.model}
                aria-pressed={active}
                onClick={() => toggleModel(item.model)}
              >
                <span className="agent-model-check">{active && <Check aria-hidden="true"/>}</span>
                <span className="agent-model-choice-copy">
                  <strong>{item.label || name}<ModelPriceBadge {...item}/></strong>
                  <code>{item.model}</code>
                </span>
              </button>;
            })}
          </div>}
        </section>;
      })}
    </div>
    </div>
  </ModalFrame>;
}

interface ProjectModalProps {
  name: string;
  path: string;
  onApply: (path: string, name: string) => void;
  onClose: () => void;
}

export function ProjectModal({name, path, onApply, onClose}: ProjectModalProps) {
  const [draftPath, setDraftPath] = useState(path);
  const [draftName, setDraftName] = useState(name);
  const [error, setError] = useState('');
  const [checking, setChecking] = useState(false);
  const [picking, setPicking] = useState(false);

  useEffect(() => {
    const previous = window.xtFolderPicked;
    window.xtFolderPicked = payload => {
      setPicking(false);
      if (payload.error && !/取消|cancel/i.test(payload.error)) setError(payload.error);
      else setError('');
      if (!payload.path) return;
      const previousAutoName = projectNameFromPath(draftPath);
      setDraftPath(payload.path);
      if (!draftName.trim() || draftName === previousAutoName) setDraftName(projectNameFromPath(payload.path));
    };
    return () => {
      if (window.xtFolderPicked === previous) return;
      window.xtFolderPicked = previous;
    };
  }, [draftName, draftPath]);

  function pickFolder() {
    setPicking(true);
    setError('');
    if (window.xtPickFolderOnly) {
      window.xtPickFolderOnly();
      return;
    }
    if (!window.xtPickFolder) {
      setPicking(false);
      setError('浏览器无法获取项目文件夹路径，请在小涛桌面客户端中选择。');
      return;
    }
    void window.xtPickFolder().then(picked => {
      setPicking(false);
      if (!picked) return;
      const previousAutoName = projectNameFromPath(draftPath);
      setDraftPath(picked);
      if (!draftName.trim() || draftName === previousAutoName) setDraftName(projectNameFromPath(picked));
    }).catch(reason => {
      setPicking(false);
      if (!/取消|cancel/i.test(String(reason))) setError(String(reason));
    });
  }

  async function applyProject() {
    setChecking(true);
    setError('');
    try {
      const nextPath = draftPath.trim();
      if (!nextPath) {
        onApply('', draftName.trim());
        return;
      }
      const resolved = await api.resolveAgentProject(nextPath);
      onApply(resolved.path, draftName.trim() || resolved.name);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setChecking(false);
    }
  }

  return <ModalFrame
    title="选择项目"
    description="先选项目文件夹，也可以给这个项目起一个更好认的名字。"
    className="agent-project-modal"
    onClose={onClose}
    footer={<>
      <button type="button" className="app-modal-button quiet" onClick={() => {
        setDraftPath('');
        setDraftName('');
        setError('');
      }}>使用默认目录</button>
      <div className="app-modal-actions">
        <button type="button" className="app-modal-button secondary" onClick={onClose}>取消</button>
        <button type="button" className="app-modal-button primary" disabled={checking} onClick={() => void applyProject()}>
          {checking ? '检查中…' : '使用这个项目'}
        </button>
      </div>
    </>}
  >
    <label className="agent-field">
      <span>项目名称</span>
      <input
        value={draftName}
        onChange={event => setDraftName(event.target.value)}
        placeholder="留空则使用文件夹名称"
        autoComplete="off"
      />
    </label>
    <label className="agent-field">
      <span>项目目录</span>
      <div className="agent-folder-field">
        <input
          value={draftPath}
          readOnly
          aria-label="已选项目目录"
          placeholder="请选择项目文件夹"
          spellCheck={false}
        />
        <button type="button" onClick={pickFolder} disabled={picking} title="选择文件夹">
          <FolderOpen aria-hidden="true"/><span>{picking ? '打开中…' : '选择'}</span>
        </button>
      </div>
    </label>
    <div className="agent-project-note">
      {draftPath ? '小涛只会访问这个目录里的文件。' : '不选文件夹时，任务会使用小涛数据目录里的默认工作区。'}
    </div>
    {error && <div className="agent-modal-error" role="alert">{error}</div>}
  </ModalFrame>;
}

export function ReferenceDetailModal({reference, summary, sourceAvailable, newContent, onOpenSource, onClose}: {
  reference: AgentConversationReference;
  summary: string;
  sourceAvailable: boolean;
  newContent: boolean;
  onOpenSource: () => void;
  onClose: () => void;
}) {
  return <ModalFrame
    title={reference.title}
    description={`引用固定于 ${new Date(reference.createdAt).toLocaleString('zh-CN')}`}
    className="agent-reference-modal"
    onClose={onClose}
    footer={<>
      <button type="button" className="app-modal-button quiet" disabled={!sourceAvailable} onClick={onOpenSource}>打开原对话</button>
      <button type="button" className="app-modal-button primary" onClick={onClose}>关闭</button>
    </>}
  >
    {!sourceAvailable && <div className="agent-reference-status is-warning" role="status">来源对话已删除，已保存的摘要仍可查看。</div>}
    {sourceAvailable && newContent && <div className="agent-reference-status" role="status">来源对话后来有新内容；旧消息仍使用引用时的快照。</div>}
    <pre className="agent-reference-summary">{sourceAvailable ? summary : reference.summary || '摘要不可用'}</pre>
  </ModalFrame>;
}
