import {useEffect, useRef, useState} from 'react';
import {EditorContent, useEditor} from '@tiptap/react';
import type {Editor, JSONContent} from '@tiptap/core';
import {TextSelection} from '@tiptap/pm/state';
import type {EditorState} from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import {
  ArrowUp,
  Check,
  ChevronDown,
  Circle,
  Folder,
  LoaderCircle,
  MessageSquarePlus,
  Settings2,
  Square,
  X,
} from 'lucide-react';
import {api, type AgentActivity, type AgentEvent, type AgentModelOption, type AgentSession} from '../../lib/api';
import {AgentSettingsModal, ProjectModal} from './AgentModals';
import './agent.css';

declare global { interface Window { xtPickFolder?: () => Promise<string>; xtOpenDirectory?: (kind: 'data' | 'install') => Promise<void>; } }

async function readEventStream(response: Response, onEvent: (event: AgentEvent) => void) {
  if (!response.body) throw new Error('当前环境不支持流式消息');
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '', data: string[] = [];
  const dispatch = () => {
    if (data.length) {
      try { onEvent(JSON.parse(data.join('\n')) as AgentEvent); } catch { /* malformed frame */ }
    }
    data = [];
  };
  for (;;) {
    const {done, value} = await reader.read();
    buffer += decoder.decode(value, {stream: !done});
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (!line) dispatch();
      else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
    }
    if (done) {
      if (buffer.startsWith('data:')) data.push(buffer.slice(5).trimStart());
      dispatch();
      return;
    }
  }
}

function applyEvent(session: AgentSession, event: AgentEvent): AgentSession {
  if ((event.type === 'message' || event.type === 'session') && event.message) {
    return {...session, status: event.status || session.status, messages: [...session.messages, event.message]};
  }
  if (event.type === 'activity' && event.activity) {
    const activity = session.activity.filter(item => item.id !== event.activity!.id);
    return {...session, activity: [...activity, event.activity]};
  }
  if (event.type === 'status' || event.type === 'error') {
    return {...session, status: event.status || (event.type === 'error' ? 'failed' : session.status), error: event.error || ''};
  }
  return session;
}

function activityIcon(activity: AgentActivity) {
  if (activity.status === 'running') return <LoaderCircle className="agent-spin" aria-hidden="true"/>;
  if (activity.status === 'failed') return <X aria-hidden="true"/>;
  return <Check aria-hidden="true"/>;
}

function AgentDroplet({running}: {running: boolean}) {
  return <span className={'agent-droplet' + (running ? ' is-running' : '')} aria-hidden="true">
    <span className="agent-droplet__wave"/>
    <span className="agent-droplet__face"/>
    <span className="agent-droplet__dot agent-droplet__dot--one"/>
    <span className="agent-droplet__dot agent-droplet__dot--two"/>
    <span className="agent-droplet__dot agent-droplet__dot--three"/>
  </span>;
}

const suggestions = [
  {label: '看看这个项目里有什么', prompt: '先看看这个项目里有什么，再告诉我整体结构。'},
  {label: '帮我找一个问题并修复', prompt: '检查项目里一个明显的问题，说明原因后直接修复，并运行相关验证。'},
  {label: '跑一遍测试看看哪里坏了', prompt: '运行项目现有测试，找出失败原因并修复。'},
];

const enabledModelsKey = 'xiaotao.agent.enabledModels';

function readEnabledModels() {
  try {
    const value = JSON.parse(localStorage.getItem(enabledModelsKey) || '[]');
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

function inlineMarkdown(node: JSONContent): string {
  if (node.type === 'text') {
    let text = node.text ?? '';
    for (const mark of node.marks ?? []) {
      if (mark.type === 'code') text = '`' + text + '`';
      else if (mark.type === 'bold') text = '**' + text + '**';
      else if (mark.type === 'italic') text = '*' + text + '*';
      else if (mark.type === 'strike') text = '~~' + text + '~~';
      else if (mark.type === 'underline') text = '<u>' + text + '</u>';
      else if (mark.type === 'link') text = '[' + text + '](' + (mark.attrs?.href ?? '') + ')';
    }
    return text;
  }
  if (node.type === 'hardBreak') return '  \n';
  const content = (node.content ?? []).map(inlineMarkdown).join('');
  return content;
}

function blocksMarkdown(nodes: JSONContent[], indent = 0): string {
  return nodes.map(node => blockMarkdown(node, indent)).filter(Boolean).join('\n\n');
}

function listMarkdown(node: JSONContent, indent: number): string {
  const ordered = node.type === 'orderedList';
  const start = Number(node.attrs?.start) || 1;
  return (node.content ?? []).map((item, index) => {
    const children = item.content ?? [];
    const first = children[0];
    const prefix = ' '.repeat(indent) + (ordered ? (start + index) + '. ' : '- ');
    const head = first ? inlineMarkdown(first).replace(/\n/g, '\n' + ' '.repeat(prefix.length)) : '';
    const rest = children.slice(1);
    const nested = rest.length ? '\n' + blocksMarkdown(rest, indent + 2).replace(/\n\n/g, '\n') : '';
    return prefix + head + nested;
  }).join('\n');
}

function blockMarkdown(node: JSONContent, indent: number): string {
  if (node.type === 'paragraph') return ' '.repeat(indent) + inlineMarkdown(node);
  if (node.type === 'heading') {
    const level = Math.min(6, Math.max(1, Number(node.attrs?.level) || 1));
    return ' '.repeat(indent) + '#'.repeat(level) + ' ' + inlineMarkdown(node);
  }
  if (node.type === 'bulletList' || node.type === 'orderedList') return listMarkdown(node, indent);
  if (node.type === 'blockquote') {
    return (node.content ?? []).map(child => blockMarkdown(child, 0).split('\n').map(line => ' '.repeat(indent) + '> ' + line).join('\n')).join('\n\n');
  }
  if (node.type === 'codeBlock') {
    const language = String(node.attrs?.language || '');
    return ' '.repeat(indent) + '```' + language + '\n' + (node.content ?? []).map(child => child.text ?? '').join('') + '\n' + ' '.repeat(indent) + '```';
  }
  if (node.type === 'horizontalRule') return ' '.repeat(indent) + '---';
  return ' '.repeat(indent) + inlineMarkdown(node);
}

function editorMarkdown(editor: Editor): string {
  return blocksMarkdown(editor.getJSON().content ?? []).trim();
}

function hasActiveNode(state: EditorState, names: string[]): boolean {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if (names.includes($from.node(depth).type.name)) return true;
  }
  return false;
}

function isEmptyListItem(state: EditorState): boolean {
  const { $from, $to } = state.selection;
  return $from.parentOffset === 0 && $to.parentOffset === 0 &&
    $from.parent.content.size === 0 && hasActiveNode(state, ['listItem']);
}

function exitEmptyListItem(editor: Editor): boolean {
  const { state } = editor;
  const { $from } = state.selection;
  let listDepth = -1;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if ($from.node(depth).type.name === 'listItem') {
      listDepth = depth;
      break;
    }
  }
  if (listDepth < 0) return false;

  const list = $from.node(listDepth - 1);
  const listStart = $from.before(listDepth - 1);
  const itemIndex = $from.index(listDepth - 1);
  const item = list.child(itemIndex);
  if (itemIndex !== list.childCount - 1 || !item.firstChild?.isTextblock || item.firstChild.content.size) return false;

  const paragraph = state.schema.nodes.paragraph.create();
  const transaction = state.tr;
  if (list.childCount === 1) {
    transaction.replaceWith(listStart, listStart + list.nodeSize, paragraph);
  } else {
    transaction.delete($from.before(listDepth), $from.after(listDepth));
    const afterList = listStart + list.nodeSize - item.nodeSize - 1;
    transaction.insert(afterList, paragraph);
  }
  const after = listStart + list.nodeSize - item.nodeSize + 1;
  transaction.setSelection(TextSelection.near(transaction.doc.resolve(Math.min(after, transaction.doc.content.size))));
  editor.view.dispatch(transaction.scrollIntoView());
  return true;
}

export function AgentPage() {
  const [sessions, setSessions] = useState<AgentSession[]>([]), [active, setActive] = useState<AgentSession | null>(null);
  const [models, setModels] = useState<AgentModelOption[]>([]), [enabledModels, setEnabledModels] = useState<string[]>([]);
  const [model, setModel] = useState(''), [path, setPath] = useState(''), [projectName, setProjectName] = useState('');
  const [prompt, setPrompt] = useState(''), [busy, setBusy] = useState(false), [loading, setLoading] = useState(true);
  const [error, setError] = useState(''), [showActivity, setShowActivity] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false), [projectOpen, setProjectOpen] = useState(false);
  const feed = useRef<HTMLDivElement>(null), sendRef = useRef<() => void>(() => {});
  const promptRef = useRef(''), canSendRef = useRef(false);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({trailingNode: false}),
      Placeholder.configure({placeholder: '描述你想完成的事…'}),
    ],
    editorProps: {
      attributes: {'aria-label': '给小涛发消息', role: 'textbox', 'aria-multiline': 'true'},
      handleKeyDown: (view, event) => {
        if (event.key !== 'Enter' || event.isComposing) return false;
        if (event.ctrlKey || event.metaKey) {
          if (!canSendRef.current) return false;
          event.preventDefault();
          sendRef.current();
          return true;
        }
        if (event.shiftKey || !canSendRef.current) return false;

        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        if (hasActiveNode(view.state, ['heading', 'blockquote', 'listItem'])) {
          if (isEmptyListItem(view.state)) {
            event.preventDefault();
            editor && exitEmptyListItem(editor);
            return true;
          }
          return false;
        }

        event.preventDefault();
        sendRef.current();
        return true;
      },
    },
    onUpdate: ({editor}) => {
      const next = editorMarkdown(editor);
      promptRef.current = next;
      setPrompt(next);
    },
  });

  useEffect(() => {
    let live = true;
    Promise.all([api.agentSessions(), api.agentModels()])
      .then(([history, available]) => {
        if (!live) return;
        setSessions(history.sessions);
        const uniqueModels = Array.from(new Map(available.models.map(item => [item.model, item])).values());
        setModels(uniqueModels);
        const availableIDs = new Set(uniqueModels.map(item => item.model));
        const saved = readEnabledModels().filter(id => availableIDs.has(id));
        const initial = saved.length ? saved : uniqueModels.slice(0, 1).map(item => item.model);
        setEnabledModels(initial);
        setModel(initial[0] || '');
      })
      .catch(reason => live && setError(String(reason)))
      .finally(() => live && setLoading(false));
    return () => {live = false;};
  }, []);

  useEffect(() => {
    if (feed.current) feed.current.scrollTop = feed.current.scrollHeight;
  }, [active?.messages.length, active?.activity.length, busy]);

  async function send(contentOverride?: string) {
    const content = (contentOverride ?? promptRef.current).trim();
    if (!content || busy || !editor || (!active && !model)) return;
    const submittedContent = editorMarkdown(editor);
    setBusy(true);
    setError('');
    let session = active;
    let submitted = false;
    try {
      if (!session) {
        session = await api.createAgentSession(path, model, projectName);
        setActive(session);
        setSessions(current => [session!, ...current]);
      }
      const id = session.id;
      const response = await fetch('/api/agent/sessions/' + encodeURIComponent(id) + '/messages', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({content}),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => null);
        throw new Error(body?.error?.message || '发送失败（' + response.status + '）');
      }
      await readEventStream(response, event => {
        if (event.type === 'session' && event.message && !submitted) {
          if (editorMarkdown(editor) === submittedContent) editor.commands.clearContent();
          submitted = true;
        }
        setActive(current => current?.id === id ? applyEvent(current, event) : current);
      });
      const latest = await api.agentSession(id);
      setActive(current => current?.id === id ? latest : current);
      setSessions(current => [latest, ...current.filter(item => item.id !== id)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      if (submitted && session) {
        try {
          const latest = await api.agentSession(session.id);
          setActive(latest);
          setSessions(current => [latest, ...current.filter(item => item.id !== latest.id)]);
        } catch { /* retain visible state */ }
      }
    } finally {
      setBusy(false);
      editor.commands.focus();
    }
  }
  sendRef.current = () => {void send();};

  function newChat() {
    if (busy) return;
    setActive(null);
    setPath('');
    setProjectName('');
    setError('');
    setShowActivity(false);
    editor?.commands.clearContent();
    editor?.commands.focus();
  }

  function stop() {
    if (active) void api.cancelAgentSession(active.id).catch(reason => setError(String(reason)));
  }

  function chooseSuggestion(value: string) {
    editor?.commands.setContent(value, {emitUpdate: true});
    editor?.commands.focus('end');
  }

  function selectSession(session: AgentSession) {
    if (busy || session.id === active?.id) return;
    setActive(session);
    setError('');
    setShowActivity(false);
    editor?.commands.clearContent();
    editor?.commands.focus();
  }

  function saveAgentSettings(nextModels: string[]) {
    setEnabledModels(nextModels);
    localStorage.setItem(enabledModelsKey, JSON.stringify(nextModels));
    if (!nextModels.includes(model)) setModel(nextModels[0] || '');
    setSettingsOpen(false);
  }

  canSendRef.current = !busy && !!prompt.trim() && (!!active || !!model);

  const activity = active?.activity ?? [];
  const timeline = [
    ...(active?.messages.filter(item => item.role !== 'tool').map(item => ({kind: 'message' as const, time: item.createdAt, item})) ?? []),
    ...activity.map(item => ({kind: 'activity' as const, time: item.startedAt, item})),
  ].sort((a, b) => a.time.localeCompare(b.time));
  const activeActivity = [...activity].reverse().find(item => item.status === 'running') ?? activity.at(-1);
  const activeProjectName = active ? active.project.name : projectName || (path ? path.split(/[\\/]/).filter(Boolean).pop() : '') || '默认工作目录';
  const availablePickerModels = models.filter(item => enabledModels.includes(item.model) || item.model === active?.model);

  return <section className="agent-workspace" aria-label="小涛 Agent">
    <aside className="agent-sidebar">
      <button className="agent-new" type="button" onClick={newChat} disabled={busy}>
        <MessageSquarePlus aria-hidden="true"/><span>新会话</span>
      </button>
      <div className="agent-history-label">最近会话</div>
      <nav className="agent-session-list" aria-label="会话历史">
        {loading && <p>正在读取…</p>}
        {!loading && !sessions.length && <p>还没有会话</p>}
        {sessions.map(item => <button
          type="button"
          key={item.id}
          disabled={busy}
          className={item.id === active?.id ? 'is-active' : ''}
          onClick={() => selectSession(item)}
          title={item.title}
        >
          <span>{item.title}</span>
          <small>{item.project.name || '默认工作目录'}</small>
        </button>)}
      </nav>
      <div className="agent-sidebar-foot">
        <span className="agent-status-dot"/>
        <span>本地工作区</span>
      </div>
    </aside>

    <div className={'agent-main-panel ' + (active ? 'has-session' : 'is-empty')} role="main">
      <header className="agent-context-bar">
        <div className="agent-context-title">
          <AgentDroplet running={busy}/>
          <div><strong>{active ? active.title : '新会话'}</strong><small>{busy ? '小涛正在处理' : active ? 'Agent 工作台' : '准备开始'}</small></div>
        </div>
        <button type="button" className="agent-settings-trigger" onClick={() => setSettingsOpen(true)} title="Agent 设置" aria-label="打开 Agent 设置">
          <Settings2 aria-hidden="true"/><span>设置</span>
        </button>
      </header>

      <div className="agent-feed" ref={feed}>
        {!active && <div className="agent-empty">
          <div className="agent-empty-mark"><AgentDroplet running={false}/></div>
          <p className="agent-kicker">小涛 Agent</p>
          <h1>今天想做什么？</h1>
          <p className="agent-empty-copy">选一个项目，描述目标，剩下的交给我。</p>
          <div className="agent-suggestions">
            {suggestions.map(suggestion => <button type="button" key={suggestion.label} onClick={() => chooseSuggestion(suggestion.prompt)}>
              {suggestion.label}<ArrowUp aria-hidden="true"/>
            </button>)}
          </div>
        </div>}

        {active && activity.length > 0 && <div className={'agent-process' + (showActivity ? ' is-open' : '')}>
          <button type="button" className="agent-process-summary" onClick={() => setShowActivity(!showActivity)} aria-expanded={showActivity}>
            <span className="agent-process-icon">{busy ? <LoaderCircle className="agent-spin" aria-hidden="true"/> : <Check aria-hidden="true"/>}</span>
            <span><strong>{busy ? '正在处理' : '已完成'}</strong><small>{activeActivity?.title || `${activity.length} 个步骤`}</small></span>
            <ChevronDown className="agent-process-chevron" aria-hidden="true"/>
          </button>
          <div className="agent-process-body" inert={!showActivity}>
            <ol>
              {activity.map(item => <li key={item.id} data-status={item.status}>
                <span>{activityIcon(item)}</span>
                <div><strong>{item.title}</strong>{item.detail && <small>{item.detail}</small>}</div>
              </li>)}
            </ol>
          </div>
        </div>}

        {timeline.filter(entry => entry.kind === 'message').map(entry => entry.kind === 'message' && <article key={entry.item.id} className={'agent-message role-' + entry.item.role}>
          {entry.item.role === 'assistant' && <div className="agent-message-avatar"><AgentDroplet running={false}/></div>}
          <div className="agent-message-body">
            <span className="agent-message-label">{entry.item.role === 'user' ? '你' : '小涛'}</span>
            <div className="agent-message-content">{entry.item.content}</div>
          </div>
        </article>)}

        {busy && <div className="agent-working" role="status">
          <AgentDroplet running/>
          <span><strong>{activeActivity?.title || '正在思考'}</strong><small>{activeActivity?.detail || '正在准备下一步…'}</small></span>
          <button type="button" onClick={stop}>停止</button>
        </div>}
        {(error || active?.error) && <div className="agent-error" role="alert"><Circle aria-hidden="true"/><span>{error || active?.error}</span></div>}
      </div>

      <div className="agent-composer-wrap">
        <div className="agent-project-row">
          <button
            type="button"
            className="agent-project-trigger"
            onClick={() => !active && setProjectOpen(true)}
            disabled={busy || !!active}
            title={active?.project.path || path || '使用默认工作目录'}
          >
            <Folder aria-hidden="true"/>
            <span>{activeProjectName}</span>
            {!active && <ChevronDown aria-hidden="true"/>}
          </button>
          {!active && path && <small>只在这个目录内操作</small>}
        </div>
        <div className="agent-composer">
          <EditorContent editor={editor} className="agent-editor"/>
          <div className="agent-toolbar">
            <span className="agent-toolbar-note">{busy ? '正在运行，草稿会保留' : 'Enter 发送 · Shift + Enter 换行'}</span>
            <div className="agent-composer-actions">
              <select className="agent-option agent-model" aria-label="选择模型" value={active?.model || model} onChange={event => setModel(event.target.value)} disabled={!!active || busy || !availablePickerModels.length}>
                {!availablePickerModels.length && <option value="">{loading ? '加载模型…' : '先在设置中选择模型'}</option>}
                {availablePickerModels.map(item => <option key={item.model} value={item.model}>{item.label}</option>)}
              </select>
              {busy
                ? <button type="button" className="agent-send is-stop" onClick={stop} aria-label="停止任务" title="停止任务"><Square aria-hidden="true"/></button>
                : <button type="button" className="agent-send" disabled={!prompt.trim() || (!active && !model)} onClick={() => void send()} aria-label="发送消息" title="发送消息"><ArrowUp aria-hidden="true"/></button>}
            </div>
          </div>
        </div>
        <div className="agent-hint">
          <span>{busy ? '任务执行中 · 输入内容会保留到下次发送' : 'Ctrl/⌘ + Enter 强制发送'}</span>
          {!models.length && !loading && <button type="button" onClick={() => window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail: 'accounts'}))}>接入模型</button>}
        </div>
      </div>
    </div>

    {settingsOpen && <AgentSettingsModal
      models={models}
      selected={enabledModels}
      onSave={saveAgentSettings}
      onClose={() => setSettingsOpen(false)}
    />}
    {projectOpen && <ProjectModal
      name={projectName}
      path={path}
      onApply={(nextPath, nextName) => {
        setPath(nextPath);
        setProjectName(nextName);
        setError('');
        setProjectOpen(false);
      }}
      onClose={() => setProjectOpen(false)}
    />}
  </section>;
}
