import {AgentDroplet, activityState} from './AgentDroplet';
import {Popover} from '../../components/popover/Popover';
import {ModalPresence} from '../../components/modal/ModalFrame';
import {memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {SessionList} from './SessionList';
import {createPortal} from 'react-dom';
import {renderToStaticMarkup} from 'react-dom/server';
import {EditorContent, useEditor} from '@tiptap/react';
import type {Editor, JSONContent} from '@tiptap/core';
import {TextSelection} from '@tiptap/pm/state';
import type {EditorState} from '@tiptap/pm/state';
import {DOMParser as ProseMirrorDOMParser, Fragment, Slice} from '@tiptap/pm/model';
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
  Plus,
  Pencil,
  MoreVertical,
  Search,
  Settings2,
  Trash2,
  Square,
  X,
} from 'lucide-react';
import {api, type AgentActivity, type AgentEvent, type AgentMessage, type AgentModelOption, type AgentSession} from '../../lib/api';
import {AgentSettingsModal, ProjectModal, RenameSessionModal} from './AgentModals';
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

function mergeHistory<T extends {id: string}>(older: T[], newer: T[]): T[] {
  return Array.from(new Map([...older, ...newer].map(item => [item.id, item])).values());
}

function applyEvent(session: AgentSession, event: AgentEvent): AgentSession {
  if ((event.type === 'message' || event.type === 'message_delta' || event.type === 'session') && event.message) {
    const message = event.message;
    const existing = session.messages.find(item => item.id === message.id);
    const next = event.type === 'message_delta' && existing ? {...existing, content: existing.content + message.content} : message;
    return {...session, status: event.status || session.status, error: event.type === 'session' ? '' : session.error, messages: existing ? session.messages.map(item => item.id === message.id ? next : item) : [...session.messages, next]};
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

const MessageRow = memo(function MessageRow({item, followsProcess}: {item: AgentMessage; followsProcess: boolean}) {
  return <article className={'agent-message role-' + item.role + (followsProcess ? ' follows-process' : '')}>
    {item.role === 'assistant' && !followsProcess && <div className="agent-message-avatar"><AgentDroplet running={false}/></div>}
    <div className="agent-message-body"><div className="agent-message-content"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{item.content}</Markdown></div></div>
  </article>;
}, (previous, next) => previous.followsProcess === next.followsProcess && previous.item.id === next.item.id && previous.item.role === next.item.role && previous.item.content === next.item.content);

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


const suggestions = [
  {label: '看看这个项目里有什么', prompt: '先看看这个项目里有什么，再告诉我整体结构。'},
  {label: '帮我找一个问题并修复', prompt: '检查项目里一个明显的问题，说明原因后直接修复，并运行相关验证。'},
  {label: '跑一遍测试看看哪里坏了', prompt: '运行项目现有测试，找出失败原因并修复。'},
];

const enabledModelsKey = 'xiaotao.agent.enabledModels';
const modelLabelsKey = 'xiaotao.agent.modelLabels';
const modelCatalogKey = 'xiaotao.agent.modelCatalog';

function readModelCatalog(): AgentModelOption[] | null {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(modelCatalogKey) || 'null');
    if (!Array.isArray(value) || !value.every(item => item && typeof item.model === 'string' && typeof item.label === 'string')) return null;
    return value;
  } catch { return null; }
}

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
  const [accessMode, setAccessMode] = useState<'safe' | 'full'>('safe');
  const [accessReady, setAccessReady] = useState(false);
  const [modeMenuOpen, setModeMenuOpen] = useState(false);
  const modeButtonRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const refresh = () => { void fetch('/api/agent/access-mode').then(response => {if (!response.ok) throw new Error('读取访问模式失败'); return response.json();}).then(value => {setAccessMode(value.mode === 'full' ? 'full' : 'safe'); setAccessReady(true);}).catch(() => setAccessReady(false)); };
    refresh(); window.addEventListener('focus', refresh); window.addEventListener('storage', refresh);
    return () => {window.removeEventListener('focus', refresh); window.removeEventListener('storage', refresh);};
  }, []);
  async function changeAccessMode(mode: 'safe' | 'full') {
    setModeMenuOpen(false);
    if (mode === accessMode) return;
    if (mode === 'full' && !await confirm({title: '切换到完全访问？', description: '小涛将以当前 Windows 用户权限访问和修改文件，并自主执行命令。该模式对所有会话生效。', confirmLabel: '切换'})) return;
    try {
      const response = await fetch('/api/agent/access-mode', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({mode})});
      if (!response.ok) throw new Error('切换访问模式失败');
      setAccessMode(mode);
      try { localStorage.setItem('xiaotao.agent.accessModeChanged', String(Date.now())); } catch { /* Other windows refresh on focus. */ }
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }

  const [sessions, setSessions] = useState<AgentSession[]>([]), [active, setActive] = useState<AgentSession | null>(null);
  const [view, setView] = useState<'project' | 'session'>(() => localStorage.getItem('xiaotao.agent.historyView') === 'session' ? 'session' : 'project');
  const [viewMenuOpen, setViewMenuOpen] = useState(false);
  const viewButtonRef = useRef<HTMLButtonElement>(null);
  const [deleting, setDeleting] = useState('');
  const [sessionMenu, setSessionMenu] = useState<{item: AgentSession; x: number; y: number} | null>(null);
  const [renaming, setRenaming] = useState<AgentSession | null>(null);
  const sessionMenuTrigger = useRef<HTMLElement | null>(null);
  const [enabledModels, setEnabledModels] = useState<string[]>(readEnabledModels);
  const [models, setModels] = useState<AgentModelOption[]>(() => { const labels = readModelLabels(); return Array.from(new Map([...readEnabledModels().map(id => ({model: id, label: labels[id] || id})), ...(readModelCatalog() || [])].map(item => [item.model, item])).values()); });
  const [modelsLoading, setModelsLoading] = useState(false), [modelsError, setModelsError] = useState('');
  const [modelsAttempt, setModelsAttempt] = useState(0);
  const catalogCached = useRef(readModelCatalog() !== null);
  const lastModelsAttempt = useRef(0);
  const [model, setModel] = useState(() => readEnabledModels()[0] || ''), [path, setPath] = useState(''), [projectName, setProjectName] = useState('');
  const [modelMenuOpen, setModelMenuOpen] = useState(false);
  const modelButtonRef = useRef<HTMLButtonElement>(null);
  const [prompt, setPrompt] = useState(''), [busy, setBusy] = useState(false), [loading, setLoading] = useState(true);
  useEffect(() => {if (!visible || busy) setSessionMenu(null);}, [visible, busy]);
  useEffect(() => { if (busy) setModelMenuOpen(false); }, [busy]);
  const openSessionMenu = useCallback(function openSessionMenu(item: AgentSession, x: number, y: number, trigger: HTMLElement) {
    if (busy || deleting) return;
    sessionMenuTrigger.current = trigger;
    setSessionMenu({item, x: Math.max(8, Math.min(x, window.innerWidth - 168)), y: Math.max(8, Math.min(y, window.innerHeight - 92))});
  }, [busy, deleting]);
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
  const [projectMenuOpen, setProjectMenuOpen] = useState(false), [projectQuery, setProjectQuery] = useState('');
  const projectTriggerRef = useRef<HTMLButtonElement>(null), projectSearchRef = useRef<HTMLInputElement>(null);
  useEffect(() => { if (!visible || busy || active) setProjectMenuOpen(false); }, [visible, busy, active]);
  const feed = useRef<HTMLDivElement>(null), composer = useRef<HTMLDivElement>(null), sendRef = useRef<() => void>(() => {});
  const followFeed = useRef(true);
  const [historyBefore, setHistoryBefore] = useState('');
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const historyRequest = useRef(0);
  const historyInFlight = useRef(false);
  const activeIdRef = useRef(active?.id);
  activeIdRef.current = active?.id;
  const prependAnchor = useRef<{id: string; height: number; top: number} | null>(null);
  useLayoutEffect(() => {
    const anchor = prependAnchor.current; const el = feed.current;
    if (!anchor || !el || anchor.id !== active?.id) return;
    el.scrollTop = anchor.top + el.scrollHeight - anchor.height;
    prependAnchor.current = null;
  }, [active?.messages, active?.activity, historyLoading, historyError]);
  function preserveHistoryScroll(id: string) {
    const el = feed.current;
    if (el) prependAnchor.current = {id, height:el.scrollHeight, top:el.scrollTop};
  }
  async function loadOlder() {
    if (!active || !historyBefore || historyInFlight.current || busy) return;
    const id = active.id; const version = historyRequest.current;
    preserveHistoryScroll(id);
    followFeed.current = false;
    historyInFlight.current = true; setHistoryLoading(true); setHistoryError('');
    try {
      const page = await api.agentHistory(id, historyBefore);
      if (version !== historyRequest.current || activeIdRef.current !== id) return;
      preserveHistoryScroll(id);
      followFeed.current = false;
      setActive(current => current?.id === id ? {...current, messages:mergeHistory(page.session.messages, current.messages), activity:mergeHistory(page.session.activity, current.activity)} : current);
      setHistoryBefore(page.before);
    } catch (reason) {
      if (version === historyRequest.current) { preserveHistoryScroll(id); setHistoryError(reason instanceof Error ? reason.message : String(reason)); }
    } finally {
      if (version === historyRequest.current) { historyInFlight.current = false; setHistoryLoading(false); }
    }
  }
  function resetHistory() {
    historyRequest.current++; historyInFlight.current = false;
    setHistoryBefore(''); setHistoryLoading(false); setHistoryError(''); prependAnchor.current = null;
  }

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
      handlePaste: (view, event) => {
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        const text = event.clipboardData?.getData('text/plain').replace(/\r\n?/g, '\n');
        const match = text?.match(/^\s*(`{3,}|~{3,})(?:markdown|md)[ \t]*\n([\s\S]*?)\n\1[ \t]*\s*$/i);
        const html = event.clipboardData?.getData('text/html') ?? '';
        const source = new DOMParser().parseFromString(html, 'text/html');
        const codeContainer = source.body.querySelector('pre') || Array.from(source.body.querySelectorAll<HTMLElement>('[style]')).find(element => element.style.whiteSpace === 'pre');
        if (!match && text && codeContainer) {
          const nodes = text.split('\n').flatMap((line, index) => [
            ...(index ? [view.state.schema.nodes.hardBreak.create()] : []),
            ...(line ? [view.state.schema.text(line)] : []),
          ]);
          view.dispatch(view.state.tr.replaceSelection(new Slice(Fragment.fromArray(nodes), 0, 0)).scrollIntoView());
          event.preventDefault();
          return true;
        }
        if (!match) return false;
        const container = document.createElement('div');
        container.innerHTML = renderToStaticMarkup(<Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{match[2]}</Markdown>);
        const slice = ProseMirrorDOMParser.fromSchema(view.state.schema).parseSlice(container);
        view.dispatch(view.state.tr.replaceSelection(slice).scrollIntoView());
        event.preventDefault();
        return true;
      },
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
    if (catalogCached.current && lastModelsAttempt.current === modelsAttempt) return;
    lastModelsAttempt.current = modelsAttempt;
    let live = true;
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), 30000);
    setModelsLoading(true);
    setModelsError('');
    api.agentModels(controller.signal)
      .then(available => {
        if (!live) return;
        catalogCached.current = true;
        try { localStorage.setItem(modelCatalogKey, JSON.stringify(available.models)); } catch { /* keep in-memory cache */ }
        setModels(current => {
          const selected = new Set(readEnabledModels());
          return Array.from(new Map([...current.filter(item => selected.has(item.model)), ...available.models].map(item => [item.model, item])).values());
        });
      })
      .catch(reason => live && setModelsError(controller.signal.aborted
        ? '获取模型列表超时，请检查账号状态后重试'
        : reason instanceof TypeError ? '无法连接小涛服务，请检查服务是否正在运行' : reason instanceof Error ? reason.message : String(reason)))
      .finally(() => { window.clearTimeout(timer); if (live) setModelsLoading(false); });
    return () => {live = false; window.clearTimeout(timer); controller.abort();};
  }, [visible, settingsOpen, modelsAttempt]);

  useEffect(() => {
    if (!visible) { setSettingsOpen(false); setProjectOpen(false); setModelMenuOpen(false); }
  }, [visible]);

  useEffect(() => {
    if (feed.current && followFeed.current) feed.current.scrollTop = feed.current.scrollHeight;
  }, [active?.messages, active?.activity.length, busy, visible]);

  async function send(contentOverride?: string) {
    const content = (contentOverride ?? promptRef.current).trim();
    if (!content || busy || !editor || (!active && !model)) return;
    historyRequest.current++; historyInFlight.current = false; setHistoryLoading(false); prependAnchor.current = null;
    followFeed.current = true;
    const submittedContent = editorMarkdown(editor);
    setBusy(true);
    setError('');
    setActive(current => current ? {...current, error: ''} : current);
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
        body: JSON.stringify({content, model: model || session.model}),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => null);
        throw new Error(body?.error?.message || '发送失败（' + response.status + '）');
      }
      await readEventStream(response, event => {
        if (event.type === 'approval' && event.approval) {
          const approval = event.approval;
          void confirm({title:'确认高危操作？', description: `小涛准备${approval.tool === 'run_command' ? '执行以下命令' : '进行以下操作'}：\n\n${approval.detail}`, confirmLabel:'允许执行'}).then(async approved => {
            const response = await fetch('/api/agent/sessions/' + encodeURIComponent(id) + '/approval', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:approval.id, approved})});
            if (!response.ok) throw new Error('确认请求已失效');
          }).catch(reason => setError(reason instanceof Error ? reason.message : String(reason)));
        }
        if (event.type === 'session' && event.message && !submitted) {
          if (editorMarkdown(editor) === submittedContent) editor.commands.clearContent();
          submitted = true;
        }
        setActive(current => current?.id === id ? applyEvent(current, event) : current);
      });
      const page = await api.agentHistory(id);
      const latest = page.session;
      setActive(current => current?.id === id ? {...latest, messages:mergeHistory(current.messages, latest.messages), activity:mergeHistory(current.activity, latest.activity)} : current);
      setSessions(current => [{...latest, messages:[], activity:[]}, ...current.filter(item => item.id !== id)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      if (submitted && session) {
        try {
          const page = await api.agentHistory(session.id);
          const latest = page.session;
          setActive(current => current?.id === latest.id ? {...latest, messages:mergeHistory(current.messages, latest.messages), activity:mergeHistory(current.activity, latest.activity)} : current);
          setSessions(current => [{...latest, messages:[], activity:[]}, ...current.filter(item => item.id !== latest.id)]);
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
    resetHistory();
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

  const selectSession = useCallback(async (session: AgentSession) => {
    if (busy || session.id === active?.id) return;
    setError('');
    setBusy(true);
    try {
      historyRequest.current++; historyInFlight.current = false; setHistoryLoading(false); prependAnchor.current = null;
      const page = await api.agentHistory(session.id);
      const detail = page.session;
      followFeed.current = true;
      setHistoryBefore(page.before);
      setHistoryError('');
      setActive(detail);
      setModel(detail.model);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      return;
    } finally { setBusy(false); }
    setOpenActivities({});
    editor?.commands.clearContent();
    editor?.commands.focus();
  }, [busy, active?.id, editor]);

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
    if (!active && !nextModels.includes(model)) setModel(nextModels[0] || '');
    setSettingsOpen(false);
  }

  canSendRef.current = !busy && !!prompt.trim() && (!!active || !!model);

  const activity = active?.activity ?? [];
  const entries = useMemo(() => {
    const timeline = [
      ...(active?.messages.filter(item => item.role !== 'tool').map(item => ({kind: 'message' as const, time: item.createdAt, item})) ?? []),
      ...activity.map(item => ({kind: 'activity' as const, time: item.startedAt, item})),
    ].sort((a, b) => a.time.localeCompare(b.time));
    const entries: Array<{kind: 'message'; item: AgentMessage} | {kind: 'activity'; items: AgentActivity[]; details: typeof timeline}> = [];
    let turn: typeof timeline = [];
    const finishTurn = () => {
      let lastActivity = -1;
      turn.forEach((entry, index) => { if (entry.kind === 'activity') lastActivity = index; });
      if (lastActivity >= 0) {
        // Tool-round prose arrives with the final history refresh. Keep it inside
        // the same process instead of splitting the already visible card.
        const details = turn.slice(0, lastActivity + 1);
        entries.push({kind: 'activity', items: details.flatMap(entry => entry.kind === 'activity' ? [entry.item] : []), details});
      }
      for (const entry of turn.slice(lastActivity + 1)) {
        if (entry.kind === 'message') entries.push({kind: 'message', item: entry.item});
      }
      turn = [];
    };
    for (const entry of timeline) {
      if (entry.kind === 'message' && entry.item.role === 'user') {
        finishTurn();
        entries.push({kind: 'message', item: entry.item});
      } else turn.push(entry);
    }
    finishTurn();
    return entries;
  }, [active?.messages, active?.activity]);
  const {avatarOwners, workingAvatar} = useMemo(() => {
    const owners = new Set<number>();
    let response: number | undefined, process: number | undefined;
    const finishTurn = (pending: boolean) => {
      const owner = process ?? (pending ? undefined : response);
      if (owner !== undefined) owners.add(owner);
      return owner === undefined;
    };
    entries.forEach((entry, index) => {
      if (entry.kind === 'message' && entry.item.role === 'user') {
        finishTurn(false);
        response = process = undefined;
      } else if (entry.kind === 'activity') {
        process = index;
      } else if (entry.item.role === 'assistant') {
        response ??= index;
      }
    });
    // Prose can precede tools: choose one avatar for the whole user turn.
    const workingAvatar = finishTurn(busy);
    return {avatarOwners: owners, workingAvatar};
  }, [entries, busy]);
  const activeProjectName = active ? active.project.name : projectName || (path ? path.split(/[\\/]/).filter(Boolean).pop() : '') || '默认工作目录';
  const activeProjectPath = active ? active.project.path : path;
  const availablePickerModels = models.filter(item => enabledModels.includes(item.model));
  if (model && !availablePickerModels.some(item => item.model === model)) availablePickerModels.push({model, label: models.find(item => item.model === model)?.label || model});
  if (active?.model && !availablePickerModels.some(item => item.model === active.model)) availablePickerModels.push({model: active.model, label: active.model});

  const projects = useMemo(() => Array.from(new Map([...([...sessions].sort((a,b) => b.updatedAt.localeCompare(a.updatedAt))).filter(item => item.project.path && !item.project.isDefault).map(item => item.project), ...(path ? [{path, name: activeProjectName, isDefault: false}] : [])].map(item => [item.path.toLowerCase().replaceAll('\\', '/').replace(/\/+$/, ''), item])).values()).filter(item => `${item.name} ${item.path}`.toLowerCase().includes(projectQuery.trim().toLowerCase())), [sessions, path, activeProjectName, projectQuery]);

  return <section className="agent-workspace" aria-label="小涛 Agent">
    <aside className="agent-sidebar">
      <button className="agent-new" type="button" onClick={newChat} disabled={busy}>
        <MessageSquarePlus aria-hidden="true"/><span>新会话</span>
      </button>
      <div className="agent-history-heading">
        <span>{view === 'project' ? '项目' : '会话'}</span>
        <button ref={viewButtonRef} type="button" className="agent-history-switch" aria-label="切换会话排列方式" aria-expanded={viewMenuOpen} aria-controls="agent-history-menu" title="切换排列方式" onClick={() => setViewMenuOpen(open => !open)}><ListFilter aria-hidden="true"/></button>
        <Popover open={viewMenuOpen} onClose={() => setViewMenuOpen(false)} anchorRef={viewButtonRef} align="end" id="agent-history-menu" className="agent-history-menu" role="group" aria-label="会话排列方式">
          {(['project', 'session'] as const).map(option => <button type="button" key={option} className={view === option ? 'is-active' : ''} aria-pressed={view === option} onClick={() => {setView(option); localStorage.setItem('xiaotao.agent.historyView', option); setViewMenuOpen(false); viewButtonRef.current?.focus();}}><span>{option === 'project' ? '按项目查看' : '按会话查看'}</span>{view === option && <Check aria-hidden="true"/>}</button>)}
        </Popover>
      </div>
      <SessionList sessions={sessions} activeId={active?.id} view={view} loading={loading} busy={busy} deleting={deleting} menuId={sessionMenu?.item.id} onSelect={selectSession} onMenu={openSessionMenu}/>
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

      <div className="agent-feed" ref={feed} onScroll={event => { const el = event.currentTarget; followFeed.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80; if (el.scrollTop < 120 && !historyError) void loadOlder(); }}>
        {active && historyBefore && <div className="agent-history-load"><button type="button" disabled={historyLoading || busy} onClick={() => void loadOlder()}>{historyLoading ? '正在加载历史…' : historyError ? '加载失败，点击重试' : '加载更早的消息'}</button>{historyError && <span role="status">{historyError}</span>}</div>}
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
            const status = running ? latest?.status === 'running' ? '正在执行' : '继续思考' : next?.kind === 'message' && next.item.role === 'assistant' ? '执行完成' : active?.status === 'canceled' ? '已停止' : active?.status === 'failed' ? '执行中断' : '步骤已结束';
            return <div key={id} className="agent-process-row">{avatarOwners.has(index) && <div className="agent-message-avatar"><AgentDroplet state={running ? activityState(latest?.kind) : status === '执行中断' ? 'error' : status === '已停止' ? 'waiting' : 'completed'}/></div>}<div className={'agent-process' + (open ? ' is-open' : '') + (running ? ' is-running' : '')}>
              <button type="button" className="agent-process-summary" onClick={() => setOpenActivities(current => ({...current, [id]: !open}))} aria-expanded={open}>
                <span className="agent-process-icon">{running ? <LoaderCircle className="agent-spin" aria-hidden="true"/> : status === '执行中断' || status === '已停止' ? <X aria-hidden="true"/> : <Check aria-hidden="true"/>}</span>
                <span><strong>{status}</strong><small>{[running && latest?.status !== 'running' ? '最近操作' : '', latest?.title, latest?.detail].filter(Boolean).join(' · ')}</small></span>
                <span className="agent-process-duration">{running ? formatElapsed(started, clock) : ended ? formatElapsed(started, ended) : '耗时未知'}</span>
                <ChevronDown className="agent-process-chevron" aria-hidden="true"/>
              </button>
              <ProcessDetails open={open} onClose={() => setOpenActivities(current => ({...current, [id]: false}))}>
                {entry.details.map(detail => {
                  if (detail.kind === 'message') return <li key={detail.item.id} className="agent-process-note"><div><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{detail.item.content}</Markdown></div></li>;
                  const item = detail.item;
                  return <li key={item.id} data-status={item.status}><span>{activityIcon(item)}</span><div><strong>{item.title}</strong>{item.detail && <small title={item.detail}>{item.detail}</small>}</div><time>{item.endedAt ? formatElapsed(item.startedAt, item.endedAt) : item.status === 'running' && running ? formatElapsed(item.startedAt, clock) : ''}</time></li>;
                })}
              </ProcessDetails>
            </div></div>;
          }
          const followsProcess = entry.item.role === 'assistant' && !avatarOwners.has(index);
          return <MessageRow key={entry.item.id} item={entry.item} followsProcess={followsProcess}/>;
        })}

        {busy && entries.at(-1)?.kind !== 'activity' && <div className="agent-working-row" role="status">
          {workingAvatar && <div className="agent-message-avatar"><AgentDroplet running/></div>}<div className="agent-working">
          <span><strong>正在思考</strong><small>正在准备下一步…</small></span>
        </div></div>}
        {(error || (!busy && active?.error)) && <div className="agent-error" role="alert"><Circle aria-hidden="true"/><span>{error || active?.error}</span></div>}
      </div>

      <div className="agent-composer-wrap" ref={composer}>
        <div className="agent-project-row">
          <div className={'agent-project-picker' + (path && !active ? ' has-project' : '')}>
          <Popover open={projectMenuOpen} onClose={() => setProjectMenuOpen(false)} anchorRef={projectTriggerRef} initialFocusRef={projectSearchRef} side="top" className="agent-project-menu" role="dialog" aria-label="选择项目">
            <label className="agent-project-search"><Search aria-hidden="true"/><input ref={projectSearchRef} value={projectQuery} onChange={event => setProjectQuery(event.target.value)} placeholder="搜索项目" aria-label="搜索项目"/></label>
            <div className="agent-project-menu-list">
              {projects.map(project => <button type="button" key={project.path} title={project.path} onClick={() => {void api.resolveAgentProject(project.path).then(resolved => {setPath(resolved.path); setProjectName(project.name); setError(''); setProjectMenuOpen(false); setProjectQuery(''); projectTriggerRef.current?.focus();}).catch(reason => {setError(reason instanceof Error ? reason.message : String(reason)); setProjectMenuOpen(false);});}}><Folder aria-hidden="true"/><span>{project.name}</span>{path.toLowerCase() === project.path.toLowerCase() && <Check aria-label="当前项目"/>}</button>)}
              {!projects.length && <p>{projectQuery ? '没有匹配的项目' : '暂无项目，选择文件夹开始'}</p>}
            </div>
            <div className="agent-project-menu-actions"><button type="button" onClick={() => {setProjectMenuOpen(false); setProjectQuery(''); setProjectOpen(true);}}><Plus aria-hidden="true"/>选择新项目</button><button type="button" onClick={() => {setPath(''); setProjectName(''); setProjectMenuOpen(false); setProjectQuery(''); projectTriggerRef.current?.focus();}}><X aria-hidden="true"/>使用默认工作目录{!path && <Check aria-label="当前选择"/>}</button></div>
          </Popover>
          <button
            ref={projectTriggerRef}
            type="button"
            className="agent-project-trigger"
            onClick={() => setProjectMenuOpen(open => !open)}
            aria-expanded={projectMenuOpen}
            disabled={busy || !!active}
            title={active?.project.path || path || '使用默认工作目录'}
          >
            <Folder aria-hidden="true"/>
            <span>{activeProjectName}</span>
          </button>
          {!active && path && <button type="button" className="agent-project-clear" disabled={busy} onClick={() => {setPath(''); setProjectName('');}} aria-label="清空工作区目录" title="清空工作区目录"><X aria-hidden="true"/></button>}
          </div>
        </div>
        <div className="agent-composer">
          <EditorContent editor={editor} className="agent-editor"/>
          <div className="agent-toolbar">
            <div className="agent-mode-picker">
              <button ref={modeButtonRef} type="button" className="agent-option" aria-label="访问模式" aria-expanded={modeMenuOpen} disabled={!accessReady} onClick={() => setModeMenuOpen(open => !open)}>{accessMode === 'safe' ? '安全模式' : '完全访问'}<ChevronDown aria-hidden="true"/></button>
              <Popover open={modeMenuOpen} onClose={() => setModeMenuOpen(false)} anchorRef={modeButtonRef} side="top" className="agent-model-menu" aria-label="访问模式">
                <button type="button" role="menuitemradio" aria-checked={accessMode === 'safe'} onClick={() => void changeAccessMode('safe')}><span>安全模式 · 高危操作需确认</span>{accessMode === 'safe' && <Check aria-hidden="true"/>}</button>
                <button type="button" role="menuitemradio" aria-checked={accessMode === 'full'} onClick={() => void changeAccessMode('full')}><span>完全访问 · 自主执行</span>{accessMode === 'full' && <Check aria-hidden="true"/>}</button>
              </Popover>
            </div>
            <div className="agent-composer-actions">
              <div className="agent-model-picker">
                <button ref={modelButtonRef} type="button" className="agent-option agent-model" aria-label="选择模型" aria-haspopup="menu" aria-expanded={modelMenuOpen} disabled={busy || active?.status === 'running' || !availablePickerModels.length} onClick={() => setModelMenuOpen(open => !open)}>
                  <span>{availablePickerModels.find(item => item.model === (model || active?.model))?.label || '先在设置中选择模型'}</span><ChevronDown aria-hidden="true"/>
                </button>
                <Popover open={modelMenuOpen} onClose={() => setModelMenuOpen(false)} anchorRef={modelButtonRef} side="top" align="end" className="agent-model-menu" aria-label="选择模型">
                  {availablePickerModels.map(item => <button type="button" role="menuitemradio" aria-checked={item.model === model} key={item.model} onClick={() => {setModel(item.model); setModelMenuOpen(false); modelButtonRef.current?.focus();}}><span>{item.label}</span>{item.model === model && <Check aria-hidden="true"/>}</button>)}
                </Popover>
              </div>
              {busy
                ? <button type="button" className="agent-send is-stop" onClick={stop} aria-label="停止任务" title="停止任务"><Square aria-hidden="true"/></button>
                : <button type="button" className="agent-send" disabled={!prompt.trim() || (!active && !model)} onClick={() => void send()} aria-label="发送消息" title="发送消息"><ArrowUp aria-hidden="true"/></button>}
            </div>
          </div>
        </div>
        <div className="agent-hint">
          <span className="agent-project-context" title={activeProjectPath || '小涛数据目录中的默认工作区'}>{activeProjectPath ? `工作目录：${activeProjectPath}` : '工作目录：小涛数据目录中的默认工作区'}</span>
          {!active && !model && <button type="button" onClick={() => setSettingsOpen(true)}>先选择模型，才能发送</button>}
        </div>
      </div>
    </div>

    <Popover open={!!sessionMenu} onClose={() => setSessionMenu(null)} anchorRef={sessionMenuTrigger} point={sessionMenu ? {x: sessionMenu.x, y: sessionMenu.y} : undefined} className="agent-session-menu" aria-label="会话操作"><button type="button" role="menuitem" onClick={() => {if (sessionMenu) setRenaming(sessionMenu.item); setSessionMenu(null);}}><Pencil aria-hidden="true"/>重命名</button><button type="button" role="menuitem" className="is-danger" onClick={() => {if (!sessionMenu) return; const item = sessionMenu.item; setSessionMenu(null); void deleteSession(item);}}><Trash2 aria-hidden="true"/>删除</button></Popover>
    <ModalPresence>{renaming && <RenameSessionModal title={renaming.title} origin={(() => {const rect = sessionMenuTrigger.current?.getBoundingClientRect(); return rect ? {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2} : undefined;})()} onClose={() => {setRenaming(null); sessionMenuTrigger.current?.focus();}} onSave={async title => {const updated = await api.renameAgentSession(renaming.id, title); setSessions(current => current.map(item => item.id === updated.id ? {...item, title: updated.title} : item)); setActive(current => current?.id === updated.id ? {...current, title: updated.title} : current); setRenaming(null); sessionMenuTrigger.current?.focus();}}/>}</ModalPresence>
    <ModalPresence>{settingsOpen && <AgentSettingsModal
      models={models}
      selected={enabledModels}
      loading={modelsLoading}
      error={modelsError}
      onRetry={() => setModelsAttempt(value => value + 1)}
      onAccounts={() => { setSettingsOpen(false); window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail: 'accounts'})); }}
      onSave={saveAgentSettings}
      onClose={() => setSettingsOpen(false)}
    />}</ModalPresence>
    <ModalPresence>{projectOpen && <ProjectModal
      name={projectName}
      path={path}
      onApply={(nextPath, nextName) => {
        setPath(nextPath);
        setProjectName(nextName);
        setError('');
        setProjectOpen(false);
      }}
      onClose={() => setProjectOpen(false)}
    />}</ModalPresence>
  </section>;
}

