import {useMemo, useState} from 'react';
import {MessageRow, RunTimeline} from '../agent/messages/MessageFeed';
import {avatarOwners, conversationEntries, conversationMessages} from '../agent/messages/conversationEvents';
import type {AgentSession} from '../../lib/api';
import type {OfficeTaskDetail} from '../../lib/office';

export function TaskTimeline({session, detail, onRefresh}: {session: AgentSession; detail: OfficeTaskDetail; onRefresh: () => void}) {
  const [openRun, setOpenRun] = useState('');
  const entries = useMemo(() => conversationEntries(session), [session]);
  const owners = avatarOwners(entries);
  const messages = conversationMessages(session.messages);
  return <>
    <div className="agent-feed office-task-timeline">{entries.map((entry, index) => {
      if (entry.kind === 'run') {
        const answer = messages.find(item => item.role === 'assistant' && item.runId === entry.runId);
        return <RunTimeline key={entry.runId} runId={entry.runId} thoughts={session.thoughts} activity={session.activity} stats={entry.stats}
          streamDraft={session.streamDrafts?.find(item => item.runId === entry.runId)} answer={answer} busy={entry.phase === 'live'}
          notice={detail.task.error} onOpen={() => setOpenRun(entry.runId)}/>;
      }
      return <MessageRow key={entry.item.id} item={entry.item} followsProcess={entry.item.role === 'assistant' && !owners.has(index)} stats={entry.stats}/>;
    })}</div>
    {!session.runStats.length && !!session.activity.length && <div className="office-task-process">{session.activity.map(item => <p key={item.id}>{item.title}{item.detail ? ` · ${item.detail}` : ''}</p>)}</div>}
    {openRun && <div className="office-task-process" aria-label="任务思考与操作">
      {session.thoughts.filter(item => item.runId === openRun).map(item => <p key={item.id}>{item.text}</p>)}
      {session.activity.filter(item => item.runId === openRun).map(item => <p key={item.id}>{item.title}{item.detail ? ` · ${item.detail}` : ''}</p>)}
      <button type="button" className="office-ghost" onClick={() => {setOpenRun(''); onRefresh();}}>收起过程</button>
    </div>}
  </>;
}
