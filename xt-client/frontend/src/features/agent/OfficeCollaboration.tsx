import {useState, type FormEvent} from 'react';
import {ArrowUpRight, Bot, MessageCircleQuestion, UserRound, X} from 'lucide-react';
import {WorkspacePanel} from '../../components/WorkspacePanel';
import {officeConversations, type OfficeConversation} from '../../lib/office-conversations';
import {OfficePage} from '../office/OfficePage';
import {OfficeMemberConversation} from './OfficeMemberConversation';
import type {OfficeWorkspaceState} from './useOfficeWorkspace';
import type {AgentModelConfig, AgentModelOption} from '../../lib/api';
import type {OfficeMember} from '../../lib/office';
import './office-collaboration.css';

const statusLabels = {
  idle: '待开始', queued: '排队中', running: '正在处理', waiting: '等待确认',
  completed: '已回复', failed: '处理失败', canceled: '已停止',
};
const needsReply = (item: OfficeConversation) => !!(item.question || item.coordinatorQuestion);
const statusLabel = (item: OfficeConversation) => needsReply(item) ? '等你回复' : statusLabels[item.status];
const memberName = (item: OfficeConversation) => item.memberName || '员工';

export function OfficeDispatchCard({item, office, parent}: {item: OfficeConversation; office: OfficeWorkspaceState; parent: string}) {
  const [answer, setAnswer] = useState('');
  const [error, setError] = useState('');
  const [sending, setSending] = useState(false);
  const open = () => office.openMember(item.memberId, memberName(item), parent, item.historical ? item.taskId : undefined);
  async function reply(event: FormEvent) {
    event.preventDefault();
    if (!answer.trim() || sending) return;
    setSending(true);
    setError('');
    try {
      await officeConversations.reply(parent, item.memberId, {content: answer.trim()});
      setAnswer('');
      await office.refresh();
    } catch (reason) {setError(reason instanceof Error ? reason.message : String(reason));}
    finally {setSending(false);}
  }
  return <div className="agent-office-card content-column">
    <div className="agent-office-card-inner" data-status={needsReply(item) ? 'question' : item.status}>
      <button className="agent-office-card-open" type="button" onClick={open}>
        {needsReply(item) ? <MessageCircleQuestion aria-hidden="true"/> : <UserRound aria-hidden="true"/>}
        <span><strong>{memberName(item)}</strong><small>{item.taskTitle || '员工对话'}</small></span>
        <em>{statusLabel(item)}</em><ArrowUpRight aria-hidden="true"/>
      </button>
      {item.error && <p role="alert">{item.error}</p>}
      {item.status === 'running' && item.latestActivity && <p>{item.latestActivity.title}</p>}
      {item.question && <p>{item.question}</p>}
      {item.coordinatorQuestion && item.coordinatorQuestion !== item.question && <p><Bot aria-label="小涛"/> {item.coordinatorQuestion}</p>}
      {item.reply && <p className="agent-office-reply"><Bot aria-label="小涛"/> {item.reply}</p>}
      {item.question && !item.coordinatorQuestion && <div className="agent-office-card-actions">
        <button type="button" onClick={open}>打开对话回复</button>
        <button type="button" disabled={office.acting === item.memberId} onClick={() => void office.delegate(item)}>让小涛回复</button>
      </div>}
      {item.coordinatorQuestion && <form className="agent-office-card-answer" onSubmit={reply}>
        <input aria-label="回复小涛的问题" value={answer} onChange={event => setAnswer(event.target.value)} placeholder="回复小涛…" />
        <button type="submit" disabled={sending || !answer.trim()}>回复</button>
      </form>}
      {error && <p role="alert">{error}</p>}
    </div>
  </div>;
}

export function OfficeWorkspacePanel({office, parent, project, projectName, model, models, modelConfigs, visible, open, coordinatorRunning, onClose, onOpenMember}: {
  office: OfficeWorkspaceState; parent: string; project: string; projectName: string; model: string;
  models: AgentModelOption[]; modelConfigs: AgentModelConfig[];
  visible: boolean; open: boolean; coordinatorRunning: boolean; onClose: () => void; onOpenMember: (member: OfficeMember) => void;
}) {
  return <WorkspacePanel open={open} onClose={onClose} tabs={<>
      <button type="button" className="workspace-panel-tab" aria-pressed={office.selected === 'office'} onClick={() => office.selectTab('office')}>办公室</button>
      {Array.from(office.tabs).filter(([, tab]) => !tab.hidden).map(([id, tab]) => <div className="workspace-panel-tab-group" key={id}>
        <button type="button" className="workspace-panel-tab" aria-pressed={office.selected === id} onClick={() => office.selectTab(id)}>{tab.name}{tab.taskId ? ' · 历史任务' : ''}</button>
        <button type="button" className="workspace-panel-tab-close" aria-label={`收起${tab.name}对话`} onClick={() => office.closeTab(id)}><X aria-hidden="true"/></button>
      </div>)}
    </>}>
    {office.error && <p className="agent-office-error" role="alert">{office.error}</p>}
    <div className="workspace-panel-content" hidden={office.selected !== 'office'}>
      {open && <OfficePage visible={visible && office.selected === 'office'} projectPath={project} projectName={projectName}
        conversations={office.conversations.filter(item => !item.historical)} coordinatorRunning={coordinatorRunning} onOpenMember={onOpenMember}
        onOpenConversation={item => office.openMember(item.memberId, memberName(item))}/>}
    </div>
    {Array.from(office.tabs, ([id, tab]) => {
      const member = office.members.find(member => member.id === tab.memberId);
      return <div key={`${parent}:${id}`} className="workspace-panel-content" hidden={tab.hidden || office.selected !== id}>
        <OfficeMemberConversation parent={parent} project={project} projectName={projectName} memberId={tab.memberId} name={tab.name} historicalTask={tab.taskId}
          item={office.conversations.find(item => item.memberId === tab.memberId && (tab.taskId ? item.taskId === tab.taskId : !item.historical))} defaultModel={model}
          preferredModel={member?.modelPreference?.modelId} avatarColor={member?.appearance?.primaryColor} models={models} modelConfigs={modelConfigs}
          visible={open && visible && !tab.hidden && office.selected === id} refresh={office.refresh}/>
      </div>;
    })}
  </WorkspacePanel>;
}
