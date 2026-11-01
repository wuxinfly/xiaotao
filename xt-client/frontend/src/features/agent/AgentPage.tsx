import {useEffect, useRef, useState} from 'react';
import {api, type AgentEvent, type AgentSession, type ProviderInfo} from '../../lib/api';
import './agent.css';

declare global {
  interface Window {
    xtPickFolder?: () => Promise<string>;
    xtOpenDirectory?: (kind: 'data' | 'install') => Promise<void>;
  }
}

function mergeActivity(session: AgentSession, activity: AgentSession['activity'][number]): AgentSession {
  const items = session.activity.slice();
  const index = items.findIndex(item => item.id === activity.id);
  if (index >= 0) items[index] = activity;
  else items.push(activity);
  return {...session, activity: items};
}

function applyEvent(session: AgentSession, event: AgentEvent): AgentSession {
  if (event.type === 'message' && event.message) {
    return {...session, messages: [...session.messages, event.message]};
  }
  if (event.type === 'activity' && event.activity) return mergeActivity(session, event.activity);
  if (event.type === 'status') {
    return {...session, status: event.status || session.status, error: event.error || session.error};
  }
  if (event.type === 'error') return {...session, status: 'failed', error: event.error || '任务执行失败'};
  return session;
}

async function readEventStream(response: Response, onEvent: (event: AgentEvent) => void) {
  if (!response.body) throw new Error('当前环境不支持流式消息');
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let data: string[] = [];
  const dispatch = () => {
    if (data.length) {
      try {
        onEvent(JSON.parse(data.join('\n')) as AgentEvent);
      } catch { /* Ignore malformed intermediate frames. */ }
    }
    data = [];
  };
  for (;;) {
    const {done, value} = await reader.read();
    buffer += decoder.decode(value, {stream: !done});
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (line === '') dispatch();
      else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
    }
    if (done) {
      if (buffer.startsWith('data:')) data.push(buffer.slice(5).trimStart());
      dispatch();
      return;
    }
  }
}

export function AgentPage() {
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [active, setActive] = useState<AgentSession | null>(null);
  const [models, setModels] = useState<{model: string; label: string}[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [projectPath, setProjectPath] = useState('');
  const [draftProject, setDraftProject] = useState('');
  const [selectedModel, setSelectedModel] = useState('');
  const [prompt, setPrompt] = useState('');
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [modelLoading, setModelLoading] = useState(true);
  const [sessionLoading, setSessionLoading] = useState(true);
  const [error, setError] = useState('');
  const feedRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const desktop = typeof window.xtPickFolder === 'function';

  const loadModels = async () => {
    setModelLoading(true);
    try {
      const result = await api.agentModels();
      setModels(result.models);
      setSelectedModel(current => current || result.models[0]?.model || '');
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : String(reason);
      setError(message);
      setSessions(current => current.map(item => item.id === active?.id ? {...item, status: 'failed', error: message, updatedAt: new Date().toISOString()} : item));
    } finally {
      setModelLoading(false);
    }
  };

  useEffect(() => {
    let live = true;
    Promise.all([api.providers(), api.agentSessions()])
      .then(async ([providerResult, sessionResult]) => {
        if (!live) return;
        setProviders(providerResult.providers);
        setSessions(sessionResult.sessions);
        if (sessionResult.sessions[0]) {
          const latest = sessionResult.sessions[0];
          setActive(latest);
          setProjectPath(latest.project.path);
          setSelectedModel(latest.model);
        }
        await loadModels();
      })
      .catch(reason => live && setError(reason instanceof Error ? reason.message : String(reason)))
      .finally(() => {
        if (live) setSessionLoading(false);
      });
    return () => { live = false; };
  }, []);

  useEffect(() => {
    const feed = feedRef.current;
    if (feed) feed.scrollTo({top: feed.scrollHeight, behavior: busy ? 'smooth' : 'auto'});
  }, [active?.messages.length, active?.activity.length, busy]);

  async function chooseFolder() {
    if (!window.xtPickFolder) return;
    try {
      const path = await window.xtPickFolder();
      if (active) setDraftProject(path);
      else setProjectPath(path);
      setError('');
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : String(reason);
      if (!/取消|cancel/i.test(message)) setError(message);
    }
  }

  async function createSession(path = projectPath, model = selectedModel): Promise<AgentSession | null> {
    if (busy) return null;
    setCreating(true);
    setError('');
    try {
      const project = await api.resolveAgentProject(path);
      const session = await api.createAgentSession(project.path, model);
      setProjectPath(project.path);
      setActive(session);
      setSessions(current => [session, ...current.filter(item => item.id !== session.id)]);
      setPrompt('');
      requestAnimationFrame(() => textareaRef.current?.focus());
      return session;
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      return null;
    } finally {
      setCreating(false);
    }
  }

  async function send() {
    const content = prompt.trim();
    if (!content || busy) return;
    try {
      setError('');
      const session = active || await createSession();
      if (!session) return;
      setBusy(true);
      setPrompt('');
      const response = await fetch(`/api/agent/sessions/${encodeURIComponent(session.id)}/messages`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({content}),
      });
      if (!response.ok) {
        let message = `发送失败（${response.status}）`;
        try {
          const body = await response.json();
          message = body?.error?.message || message;
        } catch { /* Keep fallback. */ }
        throw new Error(message);
      }
      await readEventStream(response, event => {
        setActive(current => current?.id === session.id ? applyEvent(current, event) : current);
      });
      const latest = await api.agentSession(session.id);
      setActive(latest);
      setSessions(current => [latest, ...current.filter(item => item.id !== latest.id)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
      requestAnimationFrame(() => textareaRef.current?.focus());
    }
  }

  async function cancel() {
    if (!active || !busy) return;
    try {
      await api.cancelAgentSession(active.id);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }

  function newTask() {
    if (busy) return;
    setActive(null);
    setProjectPath('');
    setDraftProject('');
    setPrompt('');
    setError('');
    requestAnimationFrame(() => textareaRef.current?.focus());
  }

  const currentPath = active?.project.path || projectPath;
  const currentName = active?.project.name || '新任务';
  const activity = active?.activity.slice(-7).reverse() ?? [];
  const hasModels = models.length > 0;
  const canCompose = !!active || (!!currentPath && !!selectedModel);
  const canSend = !busy && canCompose && (!!active || !!prompt.trim());

  return <section className="agent-workspace" aria-labelledby="agent-title">
    <aside className="agent-sidebar">
      <div className="agent-side-heading">
        <div><span className="agent-eyebrow">XIAOTAO AGENT</span><h1 id="agent-title">小涛工作台</h1></div>
        <button className="agent-icon-button" type="button" onClick={newTask} disabled={busy} aria-label="新建任务" title="新建任务">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>
        </button>
      </div>
      <div className="agent-session-label"><span>任务</span><span>{sessions.length}</span></div>
      <nav className="agent-session-list" aria-label="任务历史">
        {sessionLoading && <p className="agent-empty-copy">正在读取任务...</p>}
        {!sessionLoading && !sessions.length && <p className="agent-empty-copy">还没有任务。选择项目，把第一件事交给小涛。</p>}
        {sessions.map(session => <button type="button" key={session.id} className={session.id === active?.id ? 'is-active' : ''} onClick={() => !busy && setActive(session)} disabled={busy && session.id !== active?.id}>
          <span className={`agent-session-dot status-${session.status}`} aria-hidden="true"/>
          <span className="agent-session-copy"><strong>{session.title}</strong><small>{session.project.name} · {new Date(session.updatedAt).toLocaleDateString('zh-CN', {month:'numeric', day:'numeric'})}</small></span>
        </button>)}
      </nav>
      <div className="agent-side-footer">
        <span className={`agent-connection ${hasModels ? 'online' : ''}`}><i aria-hidden="true"/>{modelLoading ? '检查模型中' : hasModels ? '模型可用' : '尚未接入模型'}</span>
        <span>仅访问本机项目</span>
      </div>
    </aside>

    <div className="agent-main-panel">
      <header className="agent-context-bar">
        <div className="agent-project-title">
          <span className="agent-project-mark" aria-hidden="true">{active?.project.name ? active.project.name.slice(0, 1).toUpperCase() : '∿'}</span>
          <span><strong>{currentName}</strong><small title={active?.project.path} >{active?.project.path || '选择项目目录后开始'}</small></span>
        </div>
        <div className="agent-context-actions">
          <span className={`agent-run-state status-${active?.status || 'idle'}`}><i aria-hidden="true"/>{active ? statusText(active.status) : '待开始'}</span>
          <span className="agent-model-tag">{active?.model || selectedModel || '未选择模型'}</span>
        </div>
      </header>

      <div className="agent-feed" ref={feedRef}>
        {!active && <AgentEmpty models={models} modelLoading={modelLoading} projectPath={projectPath} selectedModel={selectedModel} onProjectPath={setProjectPath} onModel={setSelectedModel} onChooseFolder={chooseFolder} onStart={() => void createSession()} creating={creating} desktop={desktop}/>}
        {active && !active.messages.length && <div className="agent-waiting"><span className="agent-pulse" aria-hidden="true"/><p>小涛准备好了。说清要做什么，它会自己看代码、改文件、跑验证。</p></div>}
        {active?.messages.filter(message => message.role !== 'tool').map(message => <article key={message.id} className={`agent-message role-${message.role}`}>
          <div className="agent-message-avatar" aria-hidden="true">{message.role === 'user' ? '你' : '涛'}</div>
          <div className="agent-message-body"><span>{message.role === 'user' ? '你' : '小涛'}</span><p>{message.content}</p></div>
        </article>)}
        {busy && <div className="agent-working-line"><span className="agent-pulse" aria-hidden="true"/><span>小涛正在处理...</span><button type="button" onClick={cancel}>停止</button></div>}
        {(active?.error || error) && <div className="agent-error" role="alert"><span>!</span><p>{active?.error || error}</p></div>}
      </div>

      <form className="agent-composer" onSubmit={event => {event.preventDefault(); void (active ? send() : createSession());}}>
        <div className="agent-composer-box">
          <textarea ref={textareaRef} value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => {
            if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing && canSend) {
              event.preventDefault();
              void (active ? send() : createSession());
            }
          }} placeholder={active ? '接着告诉小涛要做什么。Enter 发送，Shift+Enter 换行' : '先选择项目和模型，再描述想做的第一件事'} aria-label="给小涛发消息" rows={3} disabled={busy || !canCompose}/>
          <button type="submit" className="agent-send-button" disabled={!canSend} aria-label={active ? '发送消息' : '创建任务'} title={active ? '发送' : '开始任务'}>
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 14-7-4 14-3-6-7-1Z"/></svg>
          </button>
        </div>
        <div className="agent-composer-meta"><span>改动会直接写入所选项目</span><span>{active?.model || selectedModel || '选择模型'}</span></div>
      </form>
    </div>

    <aside className="agent-inspector">
      <div className="agent-inspector-section">
        <div className="agent-inspector-title"><span>任务设置</span>{busy && <span className="agent-live-badge">LIVE</span>}</div>
        <label className="agent-field"><span>{active ? '下一个项目' : '项目目录'}</span><input value={active ? draftProject : projectPath} onChange={event => active ? setDraftProject(event.target.value) : setProjectPath(event.target.value)} placeholder="D:\\work\\my-project" aria-label="项目目录"/>
          {desktop && <button type="button" onClick={chooseFolder} className="agent-field-action">选择</button>}
        </label>
        {active && draftProject && <button type="button" className="agent-separate-task" onClick={() => {void createSession(draftProject, selectedModel); setDraftProject('');}} disabled={busy || creating}>在这个目录另开任务</button>}
        <label className="agent-field"><span>模型</span><select value={selectedModel} onChange={event => setSelectedModel(event.target.value)} disabled={!!active || busy || !models.length}>
          {!models.length && <option value="">{modelLoading ? '读取中...' : '没有可用模型'}</option>}
          {models.map(model => <option key={model.model} value={model.model}>{model.label}</option>)}
        </select></label>
        {!hasModels && !modelLoading && <button type="button" className="agent-load-models" onClick={() => void loadModels()}>重新检查模型</button>}
      </div>
      <div className="agent-inspector-section agent-activity-section">
        <div className="agent-inspector-title"><span>执行进度</span><span>{active?.activity.length || 0}</span></div>
        {!activity.length && <p className="agent-muted">小涛读文件、改代码和跑命令的过程会显示在这里。</p>}
        <ol className="agent-activity-list">
          {activity.map(item => <li key={item.id} className={`activity-${item.status}`}>
            <span className="agent-activity-icon" aria-hidden="true">{item.kind === 'run_command' ? '>' : item.kind === 'write_file' ? '±' : item.kind === 'search_files' ? '⌕' : '·'}</span>
            <span><strong>{item.title}</strong>{item.detail && <small title={item.detail}>{item.detail}</small>}</span>
          </li>)}
        </ol>
      </div>
      <div className="agent-inspector-section agent-boundary-note">
        <div className="agent-inspector-title"><span>工作边界</span></div>
        <p>只能访问项目目录。危险命令会拦截，命令有超时和输出上限；停止任务会中断当前模型请求与命令。</p>
        {active && window.xtOpenDirectory && <button type="button" className="agent-link-button" onClick={() => void window.xtOpenDirectory?.('data')}>查看小涛数据目录</button>}
      </div>
    </aside>
  </section>;
}

function statusText(status: AgentSession['status']) {
  switch (status) {
    case 'running': return '执行中';
    case 'completed': return '已完成';
    case 'failed': return '需处理';
    case 'canceled': return '已停止';
    default: return '待开始';
  }
}

function AgentEmpty(props: {
  models: {model: string; label: string}[];
  modelLoading: boolean;
  projectPath: string;
  selectedModel: string;
  onProjectPath: (path: string) => void;
  onModel: (model: string) => void;
  onChooseFolder: () => void;
  onStart: () => void;
  creating: boolean;
  desktop: boolean;
}) {
  return <div className="agent-empty-workspace">
    <div className="agent-empty-art" aria-hidden="true"><span/></div>
    <div className="agent-empty-copy">
      <span className="agent-eyebrow">START A TASK</span>
      <h2><span>把一件软件活儿交给小涛。</span></h2>
      <p>它会先看项目，再动手改，最后跑验证。你随时可以停。</p>
      <div className="agent-onboarding">
        <label className="agent-field"><span>项目目录</span><input value={props.projectPath} onChange={event => props.onProjectPath(event.target.value)} placeholder="D:\\work\\my-project" aria-label="项目目录"/>
          {props.desktop && <button className="agent-field-action" type="button" onClick={props.onChooseFolder}>选择</button>}
        </label>
        <label className="agent-field"><span>模型</span><select value={props.selectedModel} onChange={event => props.onModel(event.target.value)} disabled={!props.models.length}>
          {!props.models.length && <option value="">{props.modelLoading ? '读取模型目录...' : '尚未接入模型'}</option>}
          {props.models.map(model => <option key={model.model} value={model.model}>{model.label}</option>)}
        </select></label>
        {!props.models.length && !props.modelLoading && <div className="agent-model-missing"><span>还没有可用模型。</span><a href="#accounts" onClick={event => {event.preventDefault(); window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail:'accounts'}));}}>去接入账号</a></div>}
        <button type="button" className="agent-start-button" onClick={props.onStart} disabled={props.creating || !props.projectPath || !props.models.length}>{props.creating ? '正在创建工作区...' : '创建工作区'}</button>
      </div>
    </div>
  </div>;
}
