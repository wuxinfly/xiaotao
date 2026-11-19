import type {RequestSummary, TokenMetrics} from './api';

export interface ContextParts {
  system: number;
  tools: number;
  messages: number;
  toolCalls: number;
  references: number;
  images: number;
}

export interface DashboardConversation {
  id: string;
  title: string;
  source: string;
  active: number;
  context: {window: number; estimated: number; parts: ContextParts; items?: Partial<Record<keyof ContextParts, Array<{label: string; tokens: number}>>>};
  totals: TokenMetrics;
  latestRequestId: string;
  requests: RequestSummary[];
}
