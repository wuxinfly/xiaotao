import {useEffect, useState} from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import type {AgentActivity, AgentMessage, AgentThought} from '../../lib/api';
import type {OfficeArtifact, OfficeTask, OfficeTaskDetail} from '../../lib/office';
import {formatBytes, formatClock, formatDuration, isTerminalTask, taskStatusLabel} from '../../lib/office';

type Props = {
  projectPath: string;
  detail: OfficeTaskDetail | null;
  loading: boolean;
  error: string;
  onRefresh: () => void;
  onLoadEarlier: () => void;
  onOpenArtifact: (artifact: OfficeArtifact) => void;
  onCancel: () => void;
  onResolveApproval: (approvalId: string, approved: boolean) => void;
  onPostMessage: (text: string) => Promise<void>;
};

function statsLine(task: OfficeTask) {
  const stats = task.stats;
  const parts: string[] = [];
  if (stats.modelRequests) parts.push(`模型请求 ${stats.modelRequests}`);
  if (stats.toolCalls) parts.push(`工具调用 ${stats.toolCalls}`);
  parts.push(stats.filesWritten ? `写入 ${stats.filesWritten}` : '写入 0');
  const duration = formatDuration(stats.durationMs);
  if (duration) parts.push(duration);
  return parts.join(' · ');
}

function usageLine(task: OfficeTask) {
  const stats = task.stats;
  const context = stats.contextTokens
    ? `本轮上下文约 ${stats.contextTokens}${stats.contextWindow ? ' / ' + stats.contextWindow : ''}（估算）`
    : '';
  if (!stats.inputKnown && !stats.outputTokens) {
    return context ? `上游未返回用量；${context}` : '上下文用量未知（上游未返回）';
  }
  const pieces: string[] = [];
  if (stats.inputKnown) pieces.push(`输入 ${stats.inputTokens ?? 0}`);
  if (stats.outputTokens) pieces.push(`输出 ${stats.outputTokens}`);
  if (context) pieces.push(context);
  return pieces.join(' · ');
}

type FeedRow = {at: string; kind: 'message' | 'activity' | 'thought'; message?: AgentMessage; activity?: AgentActivity; thought?: AgentThought};

function buildFeed(detail: OfficeTaskDetail): FeedRow[] {
  const rows: FeedRow[] = [];
  const history = detail.history?.session;
  for (const message of history?.messages ?? []) {
    if (message.role === 'tool') continue;
    rows.push({at: message.createdAt, kind: 'message', message});
  }
  for (const activity of history?.activity ?? []) rows.push({at: activity.startedAt, kind: 'activity', activity});
  for (const thought of history?.thoughts ?? []) rows.push({at: thought.createdAt, kind: 'thought', thought});
  return rows.sort((a, b) => a.at.localeCompare(b.at));
}

function FeedItem({row}: {row: FeedRow}) {
  if (row.kind === 'message' && row.message) {
    const message = row.message;
    return <article className={`office-feed-message role-${message.role}`}>
      <div className="office-feed-head">
        <span>{message.role === 'user' ? '你' : '执行者'}</span>
        <time>{formatClock(message.createdAt)}</time>
      </div>
      <div className="office-feed-body"><Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{message.content}</Markdown></div>
    </article>;
  }
  if (row.kind === 'activity' && row.activity) {
    const activity = row.activity;
    return <article className={`office-feed-activity is-${activity.status}`}>
      <span className="office-feed-kind">{activity.title}</span>
      {activity.detail && <code>{activity.detail}</code>}
      <span className="office-feed-state">{activity.status === 'running' ? '进行中' : activity.status === 'failed' ? '失败' : '完成'}</span>
    </article>;
  }
  if (row.kind === 'thought' && row.thought) {
    return <article className="office-feed-thought">{row.thought.text}</article>;
  }
  return null;
}

export function TaskDetail({projectPath, detail, loading, error, onRefresh, onLoadEarlier, onOpenArtifact, onCancel, onResolveApproval, onPostMessage}: Props) {
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState('');
  const [notice, setNotice] = useState('');
  const task = detail?.task;
  useEffect(() => { setDraft(''); setSendError(''); setNotice(''); }, [task?.id]);
  if (!task) {
    return <aside className="office-detail office-detail-empty">
      {loading ? <p role="status">读取任务中…</p> : error ? <p className="office-error" role="alert">{error}<button type="button" className="office-link" onClick={onRefresh}>重试</button></p> : <p>点一位伙伴开始聊天，或从左边查看任务。</p>}
    </aside>;
  }
  const feed = buildFeed(detail);
  const terminal = isTerminalTask(task.status);
  const canSend = !!draft.trim() && !sending;

  async function send() {
    if (!canSend) return;
    setSending(true);
    setSendError('');
    setNotice('');
    try {
      await onPostMessage(draft.trim());
      setDraft('');
      setNotice(task?.status === 'running' || task?.status === 'waiting' ? '消息已排队，本轮结束后送达。' : '消息已发出。');
    } catch (reason) {
      setSendError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setSending(false);
    }
  }

  return <aside className="office-detail" aria-label="任务详情">
    <header className="office-detail-head">
      <div>
        <span className={`office-status tone-${task.status}`}><i aria-hidden="true"/>{taskStatusLabel[task.status]}</span>
        <h2>{task.title}</h2>
        <p className="office-detail-goal">{task.goal}</p>
      </div>
      <div className="office-detail-actions">
        <button type="button" className="office-ghost" onClick={onRefresh} disabled={loading}>{loading ? '刷新中…' : '刷新'}</button>
        {!terminal && <button type="button" className="office-ghost danger" onClick={onCancel}>停止任务</button>}
      </div>
    </header>
    {error && <p className="office-error" role="alert">{error}</p>}
    <dl className="office-detail-meta">
      <div><dt>执行者</dt><dd>{task.executor.name} · {task.executor.modelLabel || task.executor.model}</dd></div>
      <div><dt>安排方式</dt><dd>小涛自主安排（单执行者）</dd></div>
      <div><dt>运行统计</dt><dd>{statsLine(task)}</dd></div>
      <div><dt>用量</dt><dd>{usageLine(task)}</dd></div>
      {task.error && <div><dt>失败原因</dt><dd className="office-error-text">{task.error}</dd></div>}
    </dl>
    {task.approval && <section className="office-approval" role="alert">
      <strong>执行者等待确认高危操作</strong>
      <code>{task.approval.detail}</code>
      <div className="office-approval-actions">
        <button type="button" className="office-primary" onClick={() => onResolveApproval(task.approval!.id, true)}>允许执行</button>
        <button type="button" className="office-ghost danger" onClick={() => onResolveApproval(task.approval!.id, false)}>拒绝</button>
      </div>
    </section>}
    <section className="office-transcript" aria-label="执行过程">
      <h3>执行过程</h3>
      {detail.historyError && <p className="office-error" role="alert">读取过程记录失败：{detail.historyError}</p>}
      {detail.history?.before && <button type="button" className="office-ghost office-load-earlier" onClick={onLoadEarlier}>加载更早记录</button>}
      {feed.length ? <div className="office-feed">{feed.map((row, index) => <FeedItem key={`${row.kind}-${index}-${row.at}`} row={row}/>)}</div>
        : <p className="office-muted">{loading ? '读取中…' : '还没有执行记录。'}</p>}
    </section>
    <section className="office-artifacts" aria-label="任务产物">
      <h3>产物</h3>
      {detail.artifacts.length
        ? <ul>{detail.artifacts.map(artifact => <li key={artifact.id}>
            <button type="button" className="office-artifact" onClick={() => onOpenArtifact(artifact)}>
              <span className="office-artifact-name">{artifact.path}</span>
              <small>{artifact.tool} · {formatBytes(artifact.size)} · {formatClock(artifact.updatedAt)}</small>
            </button>
          </li>)}</ul>
        : <p className="office-muted">这个任务还没有识别到文件写入。命令或外部工具创建的文件不会自动记录。</p>}
    </section>
    <section className="office-messages" aria-label="给执行者发消息">
      <h3>给执行者发消息</h3>
      {detail.messages.length > 0 && <ul className="office-message-list">
        {detail.messages.map(message => <li key={message.id} className={`office-message is-${message.status}`}>
          <p>{message.content}</p>
          <small>{message.status === 'delivered' ? `已送达 · ${formatClock(message.deliveredAt)}` : message.status === 'failed' ? `未送达：${message.error || '执行结束'}` : '排队中，本轮结束后送达'}</small>
        </li>)}
      </ul>}
      <div className="office-message-compose">
        <textarea value={draft} onChange={event => setDraft(event.target.value)} rows={3} maxLength={20000} disabled={sending}
          placeholder={terminal ? '任务已结束，发送会作为新一轮继续这个任务。' : '执行中不能即时送达，消息会排队，在本轮成功结束后送达。'}
          onKeyDown={event => { if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.nativeEvent.isComposing) { event.preventDefault(); void send(); } }}/>
        <div className="office-message-actions">
          <small>{draft.length}/20000 · Ctrl+Enter 发送</small>
          <button type="button" className="office-primary" disabled={!canSend} onClick={() => void send()}>{sending ? '发送中…' : '发送'}</button>
        </div>
      </div>
      {notice && <p className="office-hint" role="status">{notice}</p>}
      {sendError && <p className="office-error" role="alert">{sendError}</p>}
    </section>
    <p className="office-muted office-detail-foot">项目：{task.projectName} · {projectPath}</p>
  </aside>;
}
