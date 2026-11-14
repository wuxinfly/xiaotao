import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {officeApi, type OfficeMember, type OfficeMemberChat, type OfficeHistory} from '../../lib/office';
import {modelConfigFor, modelSettingsFor, type AgentModelConfig} from '../../lib/agent-model-settings';
import type {AgentModelOption} from '../../lib/api';
import {Select} from '../../components/select/Select';
import {ChatComposer, ChatSendButton} from '../agent/messages/ChatComposer';
import {MessageRow} from '../agent/messages/MessageFeed';
import './member-chat.css';

type Props = {
  projectPath: string; projectName: string; member: OfficeMember; preferredModel: string;
  models: AgentModelOption[]; modelConfigs: AgentModelConfig[];
  onHandoff: (goal: string) => void;
};

export function MemberChat({projectPath, projectName, member, preferredModel, models, modelConfigs, onHandoff}: Props) {
  const [chat, setChat] = useState<OfficeMemberChat | null>(null);
  const [history, setHistory] = useState<OfficeHistory | null>(null);
  const [older, setOlder] = useState<OfficeHistory | null>(null);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');
  const [sending, setSending] = useState(false);
  const [model, setModel] = useState(member.modelPreference?.modelId || preferredModel);
  const sequence = useRef(0);
  const feed = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const previousHeight = useRef(0);
  const usableModels = useMemo(() => models.filter(item => !item.unavailable), [models]);
  const selectedConfig = modelConfigFor(modelConfigs, model, models);
  const load = useCallback(async () => {
    const id = ++sequence.current;
    try {
      const next = await officeApi.memberChat(projectPath, member.id);
      const page = next.sessionId ? await officeApi.memberHistory(projectPath, member.id) : null;
      if (id === sequence.current) {setChat(next); setHistory(page); setError('');}
    } catch (reason) {
      if (id === sequence.current) setError(reason instanceof Error ? reason.message : String(reason));
    }
  }, [projectPath, member.id]);
  useEffect(() => {
    let stopped = false;
    const requests = sequence;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      if (document.visibilityState === 'visible') await load();
      if (!stopped) timer = setTimeout(tick, 2500);
    };
    void tick();
    return () => {stopped = true; requests.current++; clearTimeout(timer);};
  }, [load]);

  const busy = chat?.status === 'running' || chat?.status === 'waiting';
  const messages = [...(older?.session.messages || []), ...(history?.session.messages || [])].filter(item => item.role === 'user' || item.role === 'assistant');
  const before = older ? older.before : history?.before;
  useLayoutEffect(() => {
    const element = feed.current;
    if (!element) return;
    if (previousHeight.current) {
      element.scrollTop += element.scrollHeight - previousHeight.current;
      previousHeight.current = 0;
    } else if (follow.current) element.scrollTop = element.scrollHeight;
  }, [history, older, busy]);
  async function loadEarlier() {
    if (!before) return;
    try {
      const page = await officeApi.memberHistory(projectPath, member.id, before);
      previousHeight.current = feed.current?.scrollHeight || 0;
      setOlder(current => current ? {...page, session: {...page.session, messages: [...page.session.messages, ...current.session.messages]}} : page);
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
  }
  const lastReply = [...messages].reverse().find(item => item.role === 'assistant' && item.content.trim())?.content || '';
  async function send(content: string) {
    if (!content.trim() || !model || !chat || sending || busy) return;
    setSending(true); setError('');
    try {
      await officeApi.sendMemberMessage(projectPath, member.id, {content: content.trim(), model, projectName, modelSettings: modelSettingsFor(modelConfigs, model, models)});
      follow.current = true;
      setDraft('');
      await load();
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setSending(false);}
  }
  async function stop() {
    try {await officeApi.cancelMemberChat(projectPath, member.id); await load();}
    catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
  }
  return <div className="member-chat">
    {member.responsibility && <div className="member-chat-profile"><strong>{member.name}</strong><span>{member.responsibility}</span>{!!member.capabilities?.length && <small>{member.capabilities.join(' · ')}</small>}</div>}
    <div ref={feed} className="agent-feed member-chat-history" aria-label="成员对话记录" onScroll={event => {
      const element = event.currentTarget;
      follow.current = element.scrollHeight - element.scrollTop - element.clientHeight < 60;
    }}>
      {before && <button type="button" className="office-ghost office-load-earlier" onClick={() => void loadEarlier()}>加载更早记录</button>}
      {messages.length ? messages.map(message => <MessageRow key={message.id} item={message} followsProcess={false}/>)
        : <div className="member-chat-welcome"><strong>跟{member.name}聊聊吧</strong><p>从一个想法或问题开始。</p></div>}
      {busy && <p className="office-muted" role="status">{member.name}正在回复，回复结束后可以继续聊。</p>}
    </div>
    {chat?.error && <p className="office-error" role="alert">{chat.error}</p>}
    {error && <p className="office-error" role="alert">{error}</p>}
    {!model && <p className="office-error" role="alert">请先在模型接口中启用模型。</p>}
    <div className="member-chat-compose">
      <ChatComposer controls={<>
        <small className="member-chat-context" title="上下文窗口">{selectedConfig?.contextWindow ? `${selectedConfig.contextWindow.toLocaleString()} tokens` : 'Ctrl + Enter 发送'}</small>
        <div className="agent-composer-actions">
          <Select size="sm" searchable aria-label="模型" value={model} onChange={setModel} disabled={busy || sending}
            options={usableModels.map(item => ({value: item.model, label: `${item.label}${item.sourceLabel ? ` · ${item.sourceLabel}` : ''}`}))}/>
          <ChatSendButton busy={!!busy} disabled={!draft.trim() || !model || !chat || sending} onSend={() => void send(draft)} onStop={() => void stop()} stopLabel="停止回复"/>
        </div>
      </>}>
        <textarea aria-label={`给${member.name}发消息`} value={draft} onChange={event => setDraft(event.target.value)} placeholder="说说你的想法……" rows={3} maxLength={20000} onKeyDown={event => {if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.nativeEvent.isComposing) {event.preventDefault(); void send(draft);}}}/>
      </ChatComposer>
    </div>
    <div className="member-chat-handoff"><p>聊天不能读取或修改项目文件；需要动手时，先整理交接，再由你确认开始。</p><div><button type="button" className="office-ghost" disabled={!!busy || sending || !model || !chat} onClick={() => void send('请根据我们的对话整理一份简短的任务交接，包含目标、需要做的事和验收方式；如果还有关键问题，请先问我。暂时不要执行。')}>整理交接</button><button type="button" className="office-ghost" disabled={!lastReply || !!busy} onClick={() => onHandoff(lastReply)}>查看交接并安排</button></div></div>
  </div>;
}
