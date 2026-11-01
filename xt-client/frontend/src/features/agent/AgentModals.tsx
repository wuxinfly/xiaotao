import {useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import {createPortal} from 'react-dom';
import {Check, FolderOpen, Search, X} from 'lucide-react';
import {api, type AgentModelOption} from '../../lib/api';

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

interface ModalFrameProps {
  title: string;
  description?: string;
  className?: string;
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
}

function ModalFrame({title, description, className = '', onClose, children, footer}: ModalFrameProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusables = () => Array.from(dialog?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), [tabindex="0"]',
    ) ?? []);
    focusables()[0]?.focus();

    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeRef.current();
        return;
      }
      if (event.key !== 'Tab') return;
      const nodes = focusables();
      if (!nodes.length) return;
      const first = nodes[0], last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    window.addEventListener('keydown', handleKey, true);
    return () => {
      window.removeEventListener('keydown', handleKey, true);
      previous?.focus();
    };
  }, []);

  return createPortal(
    <div className="agent-modal-backdrop" onPointerDown={event => {
      if (event.target === event.currentTarget) onClose();
    }}>
      <div
        ref={dialogRef}
        className={'agent-modal ' + className}
        role="dialog"
        aria-modal="true"
        aria-labelledby="agent-modal-title"
      >
        <header className="agent-modal-header">
          <div>
            <h2 id="agent-modal-title">{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button type="button" className="agent-modal-close" onClick={onClose} aria-label="关闭弹窗" title="关闭">
            <X aria-hidden="true"/>
          </button>
        </header>
        <div className="agent-modal-body">{children}</div>
        <footer className="agent-modal-footer">{footer}</footer>
      </div>
    </div>,
    document.body,
  );
}

interface AgentSettingsModalProps {
  models: AgentModelOption[];
  selected: string[];
  onSave: (models: string[]) => void;
  onClose: () => void;
}

export function AgentSettingsModal({models, selected, onSave, onClose}: AgentSettingsModalProps) {
  const [query, setQuery] = useState('');
  const [chosen, setChosen] = useState(() => new Set(selected));
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
    description="只把常用模型留在输入区，选择切换会更清爽。"
    className="agent-settings-modal"
    onClose={onClose}
    footer={<>
      <span className="agent-modal-count">已选 {selectedCount} 个模型</span>
      <div className="agent-modal-actions">
        <button type="button" className="agent-modal-button secondary" onClick={onClose}>取消</button>
        <button
          type="button"
          className="agent-modal-button primary"
          disabled={!selectedCount}
          onClick={() => onSave(models.filter(item => chosen.has(item.model)).map(item => item.model))}
        >保存设置</button>
      </div>
    </>}
  >
    <label className="agent-model-search">
      <Search aria-hidden="true"/>
      <input
        value={query}
        onChange={event => setQuery(event.target.value)}
        placeholder="搜索模型或来源"
        aria-label="搜索模型"
        autoComplete="off"
      />
      {query && <button type="button" onClick={() => setQuery('')} aria-label="清除搜索"><X aria-hidden="true"/></button>}
    </label>
    <div className="agent-settings-groups">
      {!models.length && <div className="agent-modal-empty">还没有可用模型，先从“账号与配额”接入账号。</div>}
      {models.length > 0 && !visibleModels.length && <div className="agent-modal-empty">没有匹配的模型。</div>}
      {groups.map(([provider, items]) => {
        const allSelected = items.every(item => chosen.has(item.model));
        return <section className="agent-settings-group" key={provider}>
          <div className="agent-settings-group-head">
            <span><strong>{providerLabel(provider)}</strong><small>{items.length} 个</small></span>
            <button type="button" onClick={() => toggleGroup(items)}>{allSelected ? '取消全选' : '全选'}</button>
          </div>
          <div className="agent-model-options">
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
                  <strong>{item.label || name}</strong>
                  <code>{item.model}</code>
                </span>
              </button>;
            })}
          </div>
        </section>;
      })}
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
      setError('当前页面不支持选择文件夹，请直接填写项目目录。');
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
      <button type="button" className="agent-modal-button quiet" onClick={() => {
        setDraftPath('');
        setDraftName('');
        setError('');
      }}>使用默认目录</button>
      <div className="agent-modal-actions">
        <button type="button" className="agent-modal-button secondary" onClick={onClose}>取消</button>
        <button type="button" className="agent-modal-button primary" disabled={checking} onClick={() => void applyProject()}>
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
          onChange={event => setDraftPath(event.target.value)}
          placeholder="选择文件夹，或填写绝对路径"
          autoComplete="off"
          spellCheck={false}
        />
        {(window.xtPickFolderOnly || window.xtPickFolder) && <button type="button" onClick={pickFolder} disabled={picking} title="选择文件夹">
          <FolderOpen aria-hidden="true"/><span>{picking ? '打开中…' : '选择'}</span>
        </button>}
      </div>
    </label>
    <div className="agent-project-note">
      {draftPath ? '小涛只会访问这个目录里的文件。' : '不选文件夹时，任务会使用小涛数据目录里的默认工作区。'}
    </div>
    {error && <div className="agent-modal-error" role="alert">{error}</div>}
  </ModalFrame>;
}
