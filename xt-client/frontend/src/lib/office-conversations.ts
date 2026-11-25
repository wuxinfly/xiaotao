import {request} from './http';
import type {OfficeActivity, OfficeApproval, OfficeHistory, OfficeTaskStatus} from './office';
import type {AgentModelSettings} from './api';

export interface OfficeConversation {
  memberId: string;
  memberName?: string;
  sessionId?: string;
  taskId?: string;
  taskTitle?: string;
  status: OfficeTaskStatus | 'idle';
  error?: string;
  question?: string;
  coordinatorQuestion?: string;
  summaryStatus?: 'pending' | 'saved' | 'failed';
  summaryError?: string;
  approval?: OfficeApproval;
  reply?: string;
  request?: string;
  latestActivity?: OfficeActivity;
  historical?: boolean;
  sourceMessageId?: string;
}

type TurnInput = {content: string; model?: string; modelSettings?: AgentModelSettings; projectName?: string;
  images?: Array<{data: string; mimeType: string; width?: number; height?: number}>};
const scope = (parent: string) => `/api/office/conversations/${encodeURIComponent(parent)}`;
const memberChat = (parent: string, project: string, member: string, action = '', before = '') => {
  const query = new URLSearchParams({projectPath: project, parentSessionId: parent});
  if (before) query.set('before', before);
  return `/api/office/members/${encodeURIComponent(member)}/chat${action}?${query}`;
};
const post = <T>(path: string, body?: object) => request<T>(path, {method: 'POST', body: body && JSON.stringify(body)});

export const officeConversations = {
  events: (parent: string, project: string, member: string, signal: AbortSignal) =>
    fetch(scope(parent) + '/members/' + encodeURIComponent(member) + '/events?' + new URLSearchParams({projectPath: project}),
      {signal, headers: {Accept: 'text/event-stream'}}),
  list: (parent: string, signal?: AbortSignal) => request<OfficeConversation[]>(scope(parent), {signal}),
  dispatch: (parent: string, memberId: string, input: TurnInput) =>
    post<OfficeConversation>(scope(parent) + '/dispatch', {...input, memberId}),
  reply: (parent: string, memberId: string, input: Partial<TurnInput> & {delegate?: boolean}) =>
    post<OfficeConversation>(scope(parent) + '/reply', {...input, memberId}),
  close: (parent: string, choices: {force?: boolean; discardSummary?: boolean} = {}) =>
    post<{ok: boolean}>(scope(parent) + '/close', choices),
  remove: (parent: string, choices: {force?: boolean; discardSummary?: boolean}) => {
    const query = new URLSearchParams();
    if (choices.force) query.set('force', 'true');
    if (choices.discardSummary) query.set('discardSummary', 'true');
    return request<{ok: boolean}>(`/api/agent/sessions/${encodeURIComponent(parent)}${query.size ? '?' + query : ''}`, {method: 'DELETE'});
  },
  retrySummary: (parent: string, memberId: string) =>
    post<OfficeConversation>(scope(parent) + '/retry-summary', {memberId}),
  history: (parent: string, project: string, member: string, before = '', signal?: AbortSignal) =>
    request<OfficeHistory>(memberChat(parent, project, member, '/history', before), {signal}),
  send: (parent: string, project: string, member: string, input: TurnInput) =>
    post<OfficeConversation>(memberChat(parent, project, member, '/messages'), input),
  cancel: (parent: string, project: string, member: string) =>
    post<{ok: boolean}>(memberChat(parent, project, member, '/cancel')),
};
