import {useLayoutEffect, useMemo, useRef, useState} from 'react';
import {CircleAlert, RotateCw} from 'lucide-react';
import type {AgentModelConfig, AgentModelOption} from '../../lib/api';
import {officeApi, type OfficeHistory} from '../../lib/office';
import {officeConversations, type OfficeConversation} from '../../lib/office-conversations';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {ChatComposer, ChatModelPicker, ChatSendButton} from './messages/ChatComposer';
import {ChatEditor, editorMarkdown, useChatEditor} from './messages/ChatEditor';
import {ChatWorkingIndicator, MessageRow, RunTimeline} from './messages/MessageFeed';
import {conversationEntries, conversationMessages, mergeHistory, mergeRunStats} from './messages/conversationEvents';
import {ThoughtDrawer} from './ThoughtDrawer';
import {OfficeMemberAvatar} from '../office/OfficeMemberAvatar';
import {ContextMeter} from './ContextDetailPopover';
import {ComposerReasoning, composerModelSettings, type ReasoningOverride} from './ComposerReasoning';
import {useOfficeConversationHistory} from './useOfficeConversationHistory';
import {pastedImageFiles, useComposerImageDrafts} from './composerImages';
import {ChatImagePicker, ChatImagePreviews} from './messages/ChatImages';

type Props = {
  parent: string; project: string; projectName: string; memberId: string; name: string; item?: OfficeConversation; historicalTask?: string;
  defaultModel: string; preferredModel?: string; avatarColor?: string; models: AgentModelOption[]; modelConfigs: AgentModelConfig[];
  visible: boolean; refresh: () => Promise<void>;
};

export function OfficeMemberConversation({parent, project, projectName, memberId, name, item, historicalTask, defaultModel, preferredModel, avatarColor, models, modelConfigs, visible, refresh}: Props) {
  const [older, setOlder] = useState<OfficeHistory | null>(null);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');
  const [acting, setActing] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [openRun, setOpenRun] = useState('');
  const [selection, setSelection] = useState({model: '', open: false});
  const [reasoning, setReasoning] = useState<ReasoningOverride>();
  const [sending, setSending] = useState(false);
  const imageDrafts = useComposerImageDrafts(setError);
  const confirm = useConfirm();
  const feed = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const previousHeight = useRef<number | undefined>(undefined);
  const officeBusy = item?.status === 'running' || item?.status === 'queued' || !!item?.approval;
  const {history, waitForStream, connected} = useOfficeConversationHistory({
    parent, project, projectName, memberId, historicalTask, visible, historyKey: item?.taskId || item?.sessionId,
    status: item?.status, busy: officeBusy, onError: setError,
  });
  const busy = officeBusy || history?.session.status === 'running';
  const before = (older?.session.id === history?.session.id ? older : history)?.before;
  const session = history && (older?.session.id === history.session.id ? {
    ...history.session, messages: mergeHistory(older.session.messages, history.session.messages),
    activity: mergeHistory(older.session.activity, history.session.activity), thoughts: mergeHistory(older.session.thoughts, history.session.thoughts),
    runStats: mergeRunStats(older.session.runStats, history.session.runStats),
  } : history.session);
  const entries = conversationEntries(session || null);
  const runDetails = openRun ? session?.runStats?.find(run => run.runId === openRun) : undefined;
  const replying = !!(item?.question || item?.coordinatorQuestion);
  const replyingToTask = replying && !!item?.taskId;
  const model = replyingToTask ? session?.model || '' : selection.model || preferredModel || session?.model || defaultModel;
  const modelOption = models.find(option => option.model === model);
  const settings = composerModelSettings(modelConfigs, model, models, reasoning);
  const lastRun = session?.runStats?.at(-1);
  const summaryStatus = historicalTask ? undefined : item?.summaryStatus;
  const hasImages = !!imageDrafts.images.length || !!session?.hasImages || !!session?.messages.some(message => message.images?.length);
  const imageBlocked = hasImages && !modelOption?.supportsImages;
  const canSend = !!(!historicalTask && parent && (draft.trim() || imageDrafts.images.length) &&
    !imageBlocked && !imageDrafts.reading && (modelOption && !modelOption.unavailable || replying) &&
    !acting && !busy && summaryStatus !== 'pending');
  const avatar = useMemo(() => <OfficeMemberAvatar name={name} color={avatarColor}/>, [name, avatarColor]);
  const working = (sending || busy && !item?.approval && summaryStatus !== 'pending' && !(connected && history?.session.status === 'completed')) &&
    !entries.some(entry => entry.kind === 'run' && entry.phase === 'live');

  useLayoutEffect(() => {
    const element = feed.current;
    if (!element || !visible) return;
    if (previousHeight.current !== undefined) {
      element.scrollTop += element.scrollHeight - previousHeight.current;
      previousHeight.current = undefined;
    } else if (follow.current) element.scrollTop = element.scrollHeight;
  }, [history, older, busy, sending, visible]);

  async function act(work: () => Promise<unknown>) {
    if (acting) return;
    setActing(true);
    setError('');
    try {await work(); await refresh();}
    catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setActing(false);}
  }
  async function loadEarlier() {
    if (!before || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const page = historicalTask ? await officeApi.taskHistory(project, historicalTask, before) : await officeConversations.history(parent, project, memberId, before);
      previousHeight.current = feed.current?.scrollHeight;
      setOlder(current => current?.session.id === page.session.id ? {...page, session: {...page.session,
        messages: mergeHistory(page.session.messages, current.session.messages), activity: mergeHistory(page.session.activity, current.session.activity),
        thoughts: mergeHistory(page.session.thoughts, current.session.thoughts),
        runStats: mergeRunStats(page.session.runStats, current.session.runStats),
      }} : page);
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setLoadingOlder(false);}
  }
  const editor = useChatEditor({
    label: `给${name}发消息`,
    handlePaste: (_view, event) => {
      const files = pastedImageFiles(event.clipboardData);
      if (!files.length || historicalTask) return false;
      event.preventDefault();
      void imageDrafts.add(files);
      return true;
    },
    onUpdate: editor => setDraft(editorMarkdown(editor)),
    onSend: () => {if (!canSend) return false; send(); return true;},
  });
  function send() {
    if (!editor || !canSend) return;
    const content = editorMarkdown(editor).trim();
    const submitted = imageDrafts.images;
    const input = {
      content, projectName, model: selection.model || (model !== preferredModel ? model : undefined),
      modelSettings: selection.model || reasoning ? settings : undefined,
      images: submitted.length ? submitted.map(({data, mimeType, width, height}) => ({data, mimeType, width, height})) : undefined,
    };
    setSending(true);
    follow.current = true;
    void act(async () => {
      await waitForStream();
      if (replying) await officeConversations.reply(parent, memberId, input);
      else await officeConversations.send(parent, project, memberId, input);
      follow.current = true;
      if (!editor.isDestroyed && editorMarkdown(editor).trim() === content) editor.commands.clearContent();
      imageDrafts.remove(submitted.map(image => image.id));
    }).finally(() => setSending(false));
  }
  const stop = () => void act(() => item?.taskId ? officeApi.cancelTask(project, item.taskId) : officeConversations.cancel(parent, project, memberId));
  function approve() {
    const approval = item?.approval, task = item?.taskId;
    if (!approval || !task) return;
    void act(async () => {
      const approved = await confirm({title: '允许员工继续执行？', description: approval.detail, confirmLabel: '允许执行'});
      await officeApi.resolveApproval(project, task, approval.id, approved);
    });
  }
  return <div className="agent-office-member-chat">
    <header>{avatar}<strong>{name}</strong><small>{historicalTask ? '历史任务' : '临时对话'}</small></header>
    <div ref={feed} className="agent-feed" aria-label={`${name}的对话详情`} onScroll={event => {
      const element = event.currentTarget;
      follow.current = element.scrollHeight - element.scrollTop - element.clientHeight < 64;
    }}>
      {before && <div className="agent-history-load"><button disabled={loadingOlder} type="button" onClick={() => void loadEarlier()}>{loadingOlder ? '正在加载…' : '加载更早的消息'}</button></div>}
      {!entries.length && !busy && !sending && <p className="agent-office-member-empty">给{name}发一条消息</p>}
      {entries.map(entry => entry.kind === 'message'
        ? <MessageRow key={entry.item.id} item={entry.item} followsProcess={false} stats={entry.stats} assistantAvatar={avatar}/>
        : <RunTimeline key={entry.runId} runId={entry.runId} thoughts={session?.thoughts || []} activity={session?.activity || []}
            stats={entry.stats} busy={entry.phase === 'live'} onOpen={() => setOpenRun(entry.runId)} assistantAvatar={avatar}
            streamDraft={session?.streamDrafts?.find(message => message.runId === entry.runId)}
            answer={conversationMessages(session?.messages || []).find(message => message.role === 'assistant' && message.runId === entry.runId)}/>)}
      {working && <ChatWorkingIndicator avatar={avatar}/>}
    </div>
    {item?.question && <div className="agent-office-inline-status"><strong>等你回复</strong><span>{item.question}</span></div>}
    {item?.approval && <div className="agent-office-inline-status"><CircleAlert aria-hidden="true"/><span>{item.approval.detail}</span><button type="button" onClick={approve} disabled={acting}>查看并确认</button></div>}
    {(error || item?.error) && <p className="agent-office-error" role="alert">{error || item?.error}</p>}
    {summaryStatus === 'failed' && <div className="agent-office-inline-status agent-office-memory">
      <CircleAlert aria-hidden="true"/>
      <span role="status"><strong>长期记忆未保存</strong><small>{item?.summaryError || '可以继续聊天，也可以补存。'}</small></span>
      <button type="button" disabled={acting || busy} onClick={() => void act(() => officeConversations.retrySummary(parent, memberId))}>
        <RotateCw aria-hidden="true"/>重试保存
      </button>
    </div>}
    {summaryStatus === 'pending' && !busy && <p className="agent-office-memory-progress" role="status">正在重试保存…</p>}
    {!historicalTask && <ChatComposer controls={<div className="agent-composer-actions">
      <ChatImagePicker onAdd={files => void imageDrafts.add(files)} disabled={acting}/>
      <ContextMeter stats={lastRun} contextWindow={lastRun?.contextWindow ?? settings?.contextWindow ?? modelOption?.contextWindow ?? 0} modelLabel={modelOption?.label || model}/>
      <ChatModelPicker label="员工模型" models={models} value={model} open={selection.open && visible && !busy && !replyingToTask}
        onOpen={open => setSelection(current => ({...current, open}))}
        onSelect={model => {setSelection({model, open: false}); setReasoning(undefined);}} disabled={!!busy || acting || replyingToTask || summaryStatus === 'pending'}/>
      <ComposerReasoning key={model} option={modelOption} settings={settings} disabled={!!busy || acting || replyingToTask}
        onChange={value => setReasoning({model, value})}/>
      <ChatSendButton busy={!!busy && summaryStatus !== 'pending'} disabled={!canSend} onSend={send} onStop={stop}/>
    </div>}>
      <ChatImagePreviews images={imageDrafts.images} onRemove={id => imageDrafts.remove([id])}/>
      {imageBlocked && <p className="agent-image-model-warning" role="status">
        {replyingToTask ? '该任务使用的模型不支持图片' : '当前模型不支持图片，请切换模型'}
      </p>}
      {imageDrafts.reading && <p className="agent-image-model-hint" role="status">正在准备图片…</p>}
      <ChatEditor editor={editor}/>
    </ChatComposer>}
    {session && runDetails && visible && <ThoughtDrawer runId={runDetails.runId} thoughts={session.thoughts || []} activity={session.activity} stats={runDetails} open onClose={() => setOpenRun('')}/>}
  </div>;
}
