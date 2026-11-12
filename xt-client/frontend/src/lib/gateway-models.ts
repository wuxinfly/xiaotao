import {request} from './http';

/** The public gateway catalog, including plugin aliases; never use Agent IDs here. */
export interface GatewayModel {
  id: string;
  label: string;
  source_id: string;
  source_label: string;
  upstream_model_id: string;
  max_input_tokens?: number;
  max_output_tokens?: number;
  supports_images: boolean;
  supports_reasoning: boolean;
  reasoning_efforts?: string[];
}
export const gatewayModels = (signal?: AbortSignal) => request<{models: GatewayModel[]}>('/api/gateway/models', {signal});
export function exportedModel(model: GatewayModel) {
  return {
    model: model.id,
    contextWindow: model.max_input_tokens || undefined,
    maxOutputTokens: model.max_output_tokens || undefined,
    supportsImages: model.supports_images,
    supportsReasoning: model.supports_reasoning,
    reasoningEfforts: model.reasoning_efforts?.length ? model.reasoning_efforts : undefined,
  };
}
