import {useEffect, useRef, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {FolderOpen, Sparkles} from 'lucide-react';
import {api, type AgentConversationReference} from '../../lib/api';

declare global {
  interface Window {
    xtPickFolder?: () => Promise<string>;
    xtPickFolderOnly?: () => void;
    xtFolderPicked?: (payload: {path?: string; error?: string}) => void;
  }
}

function projectNameFromPath(path: string) {
  return path.split(/[\\/]/).filter(Boolean).pop() || '';
}

export function RenameSessionModal({title, origin, onSave, onClose, project = false, onGenerateTitle}: {project?: boolean; title: string; origin?: {x: number; y: number}; onSave: (value: string) => Promise<void>; onClose: () => void; onGenerateTitle?: (signal: AbortSignal) => Promise<string>}) {
 const [value, setValue] = useState(title), [saving, setSaving] = useState(false), [generating, setGenerating] = useState(false), [error, setError] = useState('');
 const busy = saving || generating;
 const generation = useRef<AbortController | null>(null);
 const cancelGeneration = () => {generation.current?.abort(); generation.current = null; setGenerating(false);};
 const close = () => { if (saving) return; cancelGeneration(); onClose(); };
 useEffect(() => () => generation.current?.abort(), []);
 const submit = async () => { if (busy || !value.trim()) return; setSaving(true); setError(''); try { await onSave(value.trim()); } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); } finally {setSaving(false);} };
 const generate = async () => {
   if (!onGenerateTitle || busy) return;
   const controller = new AbortController();
   generation.current = controller;
   setGenerating(true); setError('');
   try {
     const generated = await onGenerateTitle(controller.signal);
     if (generated.trim()) setValue(generated.trim());
   } catch (reason) {
     if (!(reason instanceof DOMException && reason.name === 'AbortError')) setError(reason instanceof Error ? reason.message : String(reason));
   } finally {
     if (generation.current === controller) {
       generation.current = null;
       setGenerating(false);
     }
   }
 };
 return <ModalFrame origin={origin} title={project ? "重命名项目" : "重命名会话"} className="agent-rename-modal" onClose={close} footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" disabled={saving} onClick={close}>取消</button><button type="button" className="app-modal-button primary" disabled={busy || !value.trim() || Array.from(value.trim()).length > 100} onClick={() => void submit()}>{saving ? '保存中…' : generating ? '生成中…' : '保存'}</button></div>}><label className="agent-field"><span>{project ? "项目名称" : "会话名称"}</span><div className="agent-title-input-row"><input value={value} onChange={event => setValue(event.target.value)} disabled={saving} onKeyDown={event => {if(event.key === 'Enter' && !event.nativeEvent.isComposing) void submit();}}/>{onGenerateTitle && <button type="button" className="agent-title-generate" disabled={busy} onClick={() => void generate()} aria-label="AI生成标题" title="根据最新会话内容生成标题"><Sparkles aria-hidden="true"/>{generating ? '生成中…' : 'AI生成'}</button>}</div></label>{error && <p className="agent-modal-error" role="alert">{error}</p>}</ModalFrame>;
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
      const name = draftName.trim() || resolved.name;
      await api.saveAgentProject(resolved.path, name);
      onApply(resolved.path, name);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setChecking(false);
    }
  }

  return <ModalFrame
    title="选择项目"
    description="先选项目文件夹，也可以给这个项目起一个更好认的名字。使用后会保存到项目列表。"
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
