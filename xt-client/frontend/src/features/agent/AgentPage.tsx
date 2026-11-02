import {useEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {EditorContent, useEditor} from '@tiptap/react';
import type {Editor, JSONContent} from '@tiptap/core';
import {TextSelection} from '@tiptap/pm/state';
import type {EditorState} from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import {
  ArrowUp,
  ListFilter,
  Check,
  ChevronDown,
  Circle,
  Folder,
  LoaderCircle,
  MessageSquarePlus,
  Settings2,
  Trash2,
  Square,
  X,
} from 'lucide-react';
import {api, type AgentActivity, type AgentEvent, type AgentMessage, type AgentModelOption, type AgentSession} from '../../lib/api';
import {AgentSettingsModal, ProjectModal} from './AgentModals';
import './agent.css';
import {useConfirm} from '../../components/confirm/ConfirmProvider';

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

function formatElapsed(start: string, end: string | number) {
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (!Number.isFinite(ms) || ms < 0) return '耗时未知';
  if (ms < 1000) return ms === 0 ? '< 1 毫秒' : `${ms} 毫秒`;
  const seconds = Math.floor(ms / 1000);
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

function ProcessDetails({open, onClose, children}: {open: boolean; onClose: () => void; children: React.ReactNode}) {
  const anchor = useRef<HTMLSpanElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [mounted, setMounted] = useState(open);
  const [shown, setShown] = useState(false);
  const [position, setPosition] = useState({left: 0, top: 0, width: 0, maxHeight: 220});
  useEffect(() => {
    if (open) {
      setMounted(true);
      const frame = requestAnimationFrame(() => setShown(true));
      return () => cancelAnimationFrame(frame);
    }
    setShown(false);
    const timer = window.setTimeout(() => setMounted(false), 180);
    return () => window.clearTimeout(timer);
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const update = () => {
      const rect = anchor.current?.parentElement?.getBoundingClientRect();
      if (!rect) return;
      const below = window.innerHeight - rect.bottom - 12;
      const above = rect.top - 12;
      const upward = below < 160 && above > below;
      const height = Math.max(0, Math.min(220, upward ? above : below));
      setPosition({left: rect.left, top: upward ? rect.top - height - 4 : rect.bottom + 4, width: rect.width, maxHeight: height});
    };
    const dismiss = (event: PointerEvent) => { if (!panel.current?.contains(event.target as Node) && !anchor.current?.parentElement?.contains(event.target as Node)) onClose(); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { onClose(); anchor.current?.parentElement?.querySelector<HTMLButtonElement>('button')?.focus(); } };
    update();
    window.addEventListener('resize', update);
    document.addEventListener('scroll', update, true);
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', escape);
    return () => { window.removeEventListener('resize', update); document.removeEventListener('scroll', update, true); document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', escape); };
  }, [open, onClose]);
  return <><span ref={anchor} hidden/>{mounted && createPortal(<div ref={panel} className={'agent-process-popover' + (shown && open ? ' is-visible' : '')} inert={!open} aria-hidden={!open} style={{left: position.left, top: position.top, width: position.width, maxHeight: position.maxHeight}}><ol>{children}</ol></div>, document.body)}</>;
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
const modelLabelsKey = 'xiaotao.agent.modelLabels';

function readModelLabels(): Record<string, string> {
  try {
    const value = JSON.parse(localStorage.getItem(modelLabelsKey) || '{}');
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch { return {}; }
}

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

export function AgentPage({visible}: {visible: boolean}) {
  const confirm = useConfirm();
  const [sessions, setSessions] = useState<AgentSession[]>([]), [active, setActive] = useState<AgentSession | null>(null);
  const [view, setView] = useState<'project' | 'session'>(() => localStorage.getItem('xiaotao.agent.historyView') === 'session' ? 'session' : 'project');
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => new Set());
  const [viewMenuOpen, setViewMenuOpen] = useState(false);
  const viewMenuRef = useRef<HTMLDivElement>(null);
  const viewButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!viewMenuOpen) return;
    const dismiss = (event: PointerEvent) => {
      if (!viewMenuRef.current?.contains(event.target as Node)) setViewMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setViewMenuOpen(false);
        viewButtonRef.current?.focus();
      }
    };
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', onKeyDown);
    return () => { document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', onKeyDown); };
  }, [viewMenuOpen]);
  const [deleting, setDeleting] = useState('');
  const [enabledModels, setEnabledModels] = useState<string[]>(readEnabledModels);
  const [models, setModels] = useState<AgentModelOption[]>(() => { const labels = readModelLabels(); return readEnabledModels().map(id => ({model: id, label: labels[id] || id})); });
  const [modelsLoading, setModelsLoading] = useState(false), [modelsError, setModelsError] = useState('');
  const [model, setModel] = useState(() => readEnabledModels()[0] || ''), [path, setPath] = useState(''), [projectName, setProjectName] = useState('');
  const [modelMenuOpen, setModelMenuOpen] = useState(false);
  const modelPickerRef = useRef<HTMLDivElement>(null), modelButtonRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!modelMenuOpen) return;
    const dismiss = (event: PointerEvent) => { if (!modelPickerRef.current?.contains(event.target as Node)) setModelMenuOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { setModelMenuOpen(false); modelButtonRef.current?.focus(); } };
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', escape);
    modelPickerRef.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
    return () => { document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', escape); };
  }, [modelMenuOpen]);
  const [prompt, setPrompt] = useState(''), [busy, setBusy] = useState(false), [loading, setLoading] = useState(true);
  useEffect(() => { if (busy || active) setModelMenuOpen(false); }, [busy, active]);
  const [error, setError] = useState(''), [openActivities, setOpenActivities] = useState<Record<string, boolean>>({});
  const [clock, setClock] = useState(() => Date.now());
  const wasBusy = useRef(false);
  useEffect(() => {
    if (wasBusy.current && !busy) setOpenActivities({});
    wasBusy.current = busy;
  }, [busy]);
  useEffect(() => {
    if (!busy) return;
    setClock(Date.now());
    const timer = window.setInterval(() => setClock(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [busy]);
  const [settingsOpen, setSettingsOpen] = useState(false), [projectOpen, setProjectOpen] = useState(false);
  const feed = useRef<HTMLDivElement>(null), composer = useRef<HTMLDivElement>(null), sendRef = useRef<() => void>(() => {});
  const followFeed = useRef(true);
  useEffect(() => {
    const element = composer.current;
    const scrollArea = feed.current;
    if (!element || !scrollArea) return;
    const observer = new ResizeObserver(() => {
      scrollArea.style.setProperty('--composer-height', `${element.offsetHeight}px`);
      if (followFeed.current) scrollArea.scrollTop = scrollArea.scrollHeight;
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const promptRef = useRef(''), canSendRef = useRef(false);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({trailingNode: false}),
      Placeholder.configure({placeholder: '描述你想完成的事…'}),
    ],
    editorProps: {
      attributes: {'aria-label': '给小涛发消息', role: 'textbox', 'aria-multiline': 'true'},
      handleTextInput: (view, from, to, text) => {
        if (text !== ' ' || from !== to || hasActiveNode(view.state, ['codeBlock'])) return false;
        const { $from } = view.state.selection;
        const before = $from.parent.textBetween(0, $from.parentOffset, '\n', '\ufffc');
        const match = /(^|[^`])`([^`\n]+)`$/.exec(before);
        if (!match || !view.state.schema.marks.code) return false;
        const start = from - match[2].length - 2;
        let formatted = false;
        view.state.doc.nodesBetween(start, from, node => { if (node.isText && node.marks.length) formatted = true; });
        if (formatted) return false;
        const code = view.state.schema.text(match[2], [view.state.schema.marks.code.create()]);
        const space = view.state.schema.text(' ');
        view.dispatch(view.state.tr.replaceWith(start, to, [code, space]).scrollIntoView());
        return true;
      },
      handleKeyDown: (view, event) => {
        if (event.key !== 'Enter' || event.isComposing) return false;
        if (event.ctrlKey || event.metaKey) {
          if (!canSendRef.current) return false;
          event.preventDefault();
          sendRef.current();
          return true;
        }
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        if (hasActiveNode(view.state, ['listItem'])) {
          if (isEmptyListItem(view.state)) {
            if (editor && exitEmptyListItem(editor)) { event.preventDefault(); return true; }
          } else if (editor?.commands.splitListItem('listItem')) {
            event.preventDefault();
            return true;
          }
          return false;
        }
        if (event.shiftKey || !canSendRef.current) return false;

        if (hasActiveNode(view.state, ['heading', 'blockquote'])) return false;

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
    api.agentSessions()
      .then(history => { if (live) setSessions(history.sessions); })
      .catch(reason => live && setError(String(reason)))
      .finally(() => live && setLoading(false));
    return () => {live = false;};
  }, []);

  useEffect(() => {
    if (!visible || !settingsOpen) return;
    let live = true;
    setModelsLoading(true);
    setModelsError('');
    api.agentModels()
      .then(available => {
        if (!live) return;
        setModels(current => {
          const selected = new Set(readEnabledModels());
          return Array.from(new Map([...current.filter(item => selected.has(item.model)), ...available.models].map(item => [item.model, item])).values());
        });
      })
      .catch(reason => live && setModelsError(reason instanceof Error ? reason.message : String(reason)))
      .finally(() => live && setModelsLoading(false));
    return () => {live = false;};
  }, [visible, settingsOpen]);

  useEffect(() => {
    if (!visible) { setSettingsOpen(false); setProjectOpen(false); setModelMenuOpen(false); }
  }, [visible]);

  useEffect(() => {
    if (feed.current && followFeed.current) feed.current.scrollTop = feed.current.scrollHeight;
  }, [active?.messages.length, active?.activity.length, busy, visible]);

  async function send(contentOverride?: string) {
    const content = (contentOverride ?? promptRef.current).trim();
    if (!content || busy || !editor || (!active && !model)) return;
    followFeed.current = true;
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
    setOpenActivities({});
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

  async function selectSession(session: AgentSession) {
    if (busy || session.id === active?.id) return;
    setError('');
    setBusy(true);
    try {
      const detail = await api.agentSession(session.id);
      setActive(detail);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      return;
    } finally { setBusy(false); }
    setOpenActivities({});
    editor?.commands.clearContent();
    editor?.commands.focus();
  }

  async function deleteSession(item: AgentSession) {
    if (busy || deleting) return;
    if (!await confirm({title: '删除会话？', description: '“' + item.title + '”的消息和执行记录将一并删除。', confirmLabel: '删除会话'})) return;
    setDeleting(item.id);
    try {
      await api.deleteAgentSession(item.id);
      setSessions(current => current.filter(session => session.id !== item.id));
      if (active?.id === item.id) newChat();
      setError('');
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setDeleting(''); }
  }

  function saveAgentSettings(nextModels: string[]) {
    setEnabledModels(nextModels);
    localStorage.setItem(enabledModelsKey, JSON.stringify(nextModels));
    localStorage.setItem(modelLabelsKey, JSON.stringify(Object.fromEntries(models.filter(item => nextModels.includes(item.model)).map(item => [item.model, item.label]))));
    if (!nextModels.includes(model)) setModel(nextModels[0] || '');
    setSettingsOpen(false);
  }

  canSendRef.current = !busy && !!prompt.trim() && (!!active || !!model);

  const activity = active?.activity ?? [];
  const timeline = [
    ...(active?.messages.filter(item => item.role !== 'tool').map(item => ({kind: 'message' as const, time: item.createdAt, item})) ?? []),
    ...activity.map(item => ({kind: 'activity' as const, time: item.startedAt, item})),
  ].sort((a, b) => a.time.localeCompare(b.time));
  const entries: Array<{kind: 'message'; item: AgentMessage} | {kind: 'activity'; items: AgentActivity[]}> = [];
  for (const entry of timeline) {
    if (entry.kind === 'message') entries.push({kind: 'message', item: entry.item});
    else {
      const last = entries.at(-1);
      if (last?.kind === 'activity') last.items.push(entry.item);
      else entries.push({kind: 'activity', items: [entry.item]});
    }
  }
  const activeActivity = [...activity].reverse().find(item => item.status === 'running') ?? activity.at(-1);
  const activeProjectName = active ? active.project.name : projectName || (path ? path.split(/[\\/]/).filter(Boolean).pop() : '') || '默认工作目录';
  const availablePickerModels = models.filter(item => enabledModels.includes(item.model));
  if (active?.model && !availablePickerModels.some(item => item.model === active.model)) availablePickerModels.push({model: active.model, label: active.model});

  const sortedSessions = [...sessions].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  const groups = new Map<string, {name: string; path: string; items: AgentSession[]}>();
  for (const item of sortedSessions) {
    const path = item.project.isDefault || !item.project.path ? '' : item.project.path;
    const key = path.toLowerCase().replaceAll('\\', '/').replace(/[/]+$/, '');
    const group = groups.get(key) || {name: path ? item.project.name : '最新', path, items: []};
    group.items.push(item);
    groups.set(key, group);
  }
  const renderSession = (item: AgentSession) => <div className="agent-session-row" key={item.id}>
    <button type="button" disabled={busy} className={'agent-session-select ' + (item.id === active?.id ? 'is-active' : '')} onClick={() => selectSession(item)} title={item.title}>
      <span>{item.title}</span>
      {view === 'session' && <small>{item.project.name || '默认工作目录'}</small>}
    </button>
    <button type="button" className="agent-session-delete" disabled={busy || !!deleting} onClick={() => void deleteSession(item)} aria-label={'删除会话 ' + item.title} title="删除会话"><Trash2 aria-hidden="true"/></button>
  </div>;
  const renderGroup = (key: string, group: {name: string; path: string; items: AgentSession[]}) => {
    const open = !collapsedGroups.has(key);
    return <div className={'agent-session-group' + (open ? ' is-open' : '')} key={key}>
      <button type="button" className="agent-group-label" title={group.path || group.name} aria-expanded={open} onClick={() => setCollapsedGroups(current => {
        const next = new Set(current);
        if (next.has(key)) next.delete(key); else next.add(key);
        return next;
      })}>
        {key && <Folder aria-hidden="true"/>}<span>{group.name}</span><ChevronDown className="agent-group-chevron" aria-hidden="true"/>
      </button>
      <div className="agent-group-content" inert={!open}><div>{group.items.map(renderSession)}</div></div>
    </div>;
  };

  return <section className="agent-workspace" aria-label="小涛 Agent">
    <aside className="agent-sidebar">
      <button className="agent-new" type="button" onClick={newChat} disabled={busy}>
        <MessageSquarePlus aria-hidden="true"/><span>新会话</span>
      </button>
      <div className="agent-history-heading" ref={viewMenuRef}>
        <span>{view === 'project' ? '项目' : '会话'}</span>
        <button ref={viewButtonRef} type="button" className="agent-history-switch" aria-label="切换会话排列方式" aria-expanded={viewMenuOpen} aria-controls="agent-history-menu" title="切换排列方式" onClick={() => setViewMenuOpen(open => !open)}><ListFilter aria-hidden="true"/></button>
        {viewMenuOpen && <div id="agent-history-menu" className="agent-history-menu" role="group" aria-label="会话排列方式">
          {(['project', 'session'] as const).map(option => <button type="button" key={option} className={view === option ? 'is-active' : ''} aria-pressed={view === option} onClick={() => {setView(option); localStorage.setItem('xiaotao.agent.historyView', option); setViewMenuOpen(false); viewButtonRef.current?.focus();}}><span>{option === 'project' ? '按项目查看' : '按会话查看'}</span>{view === option && <Check aria-hidden="true"/>}</button>)}
        </div>}
      </div>
      <nav className="agent-session-list" aria-label="会话历史">
        {loading && <p>正在读取…</p>}
        {!loading && !sessions.length && <p>还没有会话</p>}
        {view === 'session' ? sortedSessions.map(renderSession) : <>
          {[...groups.entries()].filter(([key]) => key !== '').map(([key, group]) => renderGroup(key, group))}
          {groups.has('') && renderGroup('', groups.get('')!)}
        </>}
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

      <div className="agent-feed" ref={feed} onScroll={event => { const el = event.currentTarget; followFeed.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80; }}>
        {!active && <div className="agent-empty">
          <div className="agent-empty-mark"><img src="/mascot.png" alt=""/></div>
          <p className="agent-kicker">小涛 Agent</p>
          <h1>今天想做什么？</h1>
          <p className="agent-empty-copy">选一个项目，描述目标，剩下的交给我。</p>
          <div className="agent-suggestions">
            {suggestions.map(suggestion => <button type="button" key={suggestion.label} onClick={() => chooseSuggestion(suggestion.prompt)}>
              {suggestion.label}<ArrowUp aria-hidden="true"/>
            </button>)}
          </div>
        </div>}

        {entries.map((entry, index) => {
          if (entry.kind === 'activity') {
            const id = entry.items[0].id;
            const running = busy && index === entries.length - 1;
            const open = !!openActivities[id];
            const latest = entry.items.at(-1);
            const next = entries[index + 1];
            const previous = entries[index - 1];
            const started = previous?.kind === 'message' && previous.item.role === 'user' ? previous.item.createdAt : entry.items[0].startedAt;
            const ended = next?.kind === 'message' && next.item.role === 'assistant' ? next.item.createdAt : latest?.endedAt;
            const status = running ? '正在执行' : next?.kind === 'message' && next.item.role === 'assistant' ? '执行完成' : active?.status === 'canceled' ? '已停止' : active?.status === 'failed' ? '执行中断' : '步骤已结束';
            return <div key={id} className="agent-process-row"><div className="agent-message-avatar"><AgentDroplet running={running}/></div><div className={'agent-process' + (open ? ' is-open' : '') + (running ? ' is-running' : '')}>
              <button type="button" className="agent-process-summary" onClick={() => setOpenActivities(current => ({...current, [id]: !open}))} aria-expanded={open}>
                <span className="agent-process-icon">{running ? <LoaderCircle className="agent-spin" aria-hidden="true"/> : status === '执行中断' || status === '已停止' ? <X aria-hidden="true"/> : <Check aria-hidden="true"/>}</span>
                <span><strong>{status}</strong><small>{[latest?.title, latest?.detail].filter(Boolean).join(' · ')}</small></span>
                <span className="agent-process-duration">{running ? formatElapsed(started, clock) : ended ? formatElapsed(started, ended) : '耗时未知'}</span>
                <ChevronDown className="agent-process-chevron" aria-hidden="true"/>
              </button>
              <ProcessDetails open={open} onClose={() => setOpenActivities(current => ({...current, [id]: false}))}>
                {entry.items.map(item => <li key={item.id} data-status={item.status}><span>{activityIcon(item)}</span><div><strong>{item.title}</strong>{item.detail && <small title={item.detail}>{item.detail}</small>}</div><time>{item.endedAt ? formatElapsed(item.startedAt, item.endedAt) : item.status === 'running' && running ? formatElapsed(item.startedAt, clock) : ''}</time></li>)}
              </ProcessDetails>
            </div></div>;
          }
          const followsProcess = entry.item.role === 'assistant' && entries[index - 1]?.kind === 'activity';
          return <article key={entry.item.id} className={'agent-message role-' + entry.item.role + (followsProcess ? ' follows-process' : '')}>
          {entry.item.role === 'assistant' && !followsProcess && <div className="agent-message-avatar"><AgentDroplet running={false}/></div>}
          <div className="agent-message-body">
            <div className="agent-message-content"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{entry.item.content}</Markdown></div>
          </div>
        </article>;
        })}

        {busy && entries.at(-1)?.kind !== 'activity' && <div className="agent-working" role="status">
          <AgentDroplet running/>
          <span><strong>{activeActivity?.title || '正在思考'}</strong><small>{activeActivity?.detail || '正在准备下一步…'}</small></span>
        </div>}
        {(error || active?.error) && <div className="agent-error" role="alert"><Circle aria-hidden="true"/><span>{error || active?.error}</span></div>}
      </div>

      <div className="agent-composer-wrap" ref={composer}>
        <div className="agent-project-row">
          <div className={'agent-project-picker' + (path && !active ? ' has-project' : '')}>
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
          {!active && path && <button type="button" className="agent-project-clear" disabled={busy} onClick={() => {setPath(''); setProjectName('');}} aria-label="清空工作区目录" title="清空工作区目录"><X aria-hidden="true"/></button>}
          </div>
        </div>
        <div className="agent-composer">
          <EditorContent editor={editor} className="agent-editor"/>
          <div className="agent-toolbar">
            <span className="agent-toolbar-note">{busy ? '正在运行，草稿会保留' : 'Enter 发送 · Shift + Enter 换行'}</span>
            <div className="agent-composer-actions">
              <div className="agent-model-picker" ref={modelPickerRef}>
                <button ref={modelButtonRef} type="button" className="agent-option agent-model" aria-label="选择模型" aria-haspopup="menu" aria-expanded={modelMenuOpen} disabled={!!active || busy || !availablePickerModels.length} onClick={() => setModelMenuOpen(open => !open)}>
                  <span>{availablePickerModels.find(item => item.model === (active?.model || model))?.label || '先在设置中选择模型'}</span><ChevronDown aria-hidden="true"/>
                </button>
                {modelMenuOpen && <div className="agent-model-menu" role="menu" aria-label="选择模型" onKeyDown={event => {
                  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
                  event.preventDefault();
                  const options = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'));
                  const index = options.indexOf(document.activeElement as HTMLButtonElement);
                  const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length;
                  options[next]?.focus();
                }}>
                  {availablePickerModels.map(item => <button type="button" role="menuitemradio" aria-checked={item.model === model} key={item.model} onClick={() => {setModel(item.model); setModelMenuOpen(false); modelButtonRef.current?.focus();}}><span>{item.label}</span>{item.model === model && <Check aria-hidden="true"/>}</button>)}
                </div>}
              </div>
              {busy
                ? <button type="button" className="agent-send is-stop" onClick={stop} aria-label="停止任务" title="停止任务"><Square aria-hidden="true"/></button>
                : <button type="button" className="agent-send" disabled={!prompt.trim() || (!active && !model)} onClick={() => void send()} aria-label="发送消息" title="发送消息"><ArrowUp aria-hidden="true"/></button>}
            </div>
          </div>
        </div>
        <div className="agent-hint">
          <span>{busy ? '任务执行中 · 输入内容会保留到下次发送' : 'Ctrl/⌘ + Enter 强制发送'}</span>
          {!enabledModels.length && <button type="button" onClick={() => setSettingsOpen(true)}>选择模型</button>}
        </div>
      </div>
    </div>

    {settingsOpen && <AgentSettingsModal
      models={models}
      selected={enabledModels}
      loading={modelsLoading}
      error={modelsError}
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

