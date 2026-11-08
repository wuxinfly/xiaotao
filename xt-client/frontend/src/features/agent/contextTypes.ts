export interface ContextBreakdown {
  systemTokens: number;
  toolsTokens: number;
  messagesTokens: number;
  toolResultsTokens: number;
  referenceTokens: number;
  imageTokens: number;
  hasImages?: boolean;
  totalTokens: number;
}
