import {Select} from '../../components/select/Select';
import {modelSettingsFor} from '../../lib/agent-model-settings';
import type {AgentModelConfig, AgentModelOption, AgentModelSettings} from '../../lib/api';

const effortLabels = new Map([
  ['low', '低'],
  ['medium', '中'],
  ['high', '高'],
  ['xhigh', '极高'],
  ['max', '最高'],
]);

export type ReasoningOverride = {model: string; value: string};

export function composerModelSettings(configs: AgentModelConfig[], model: string, options: AgentModelOption[], override?: ReasoningOverride): AgentModelSettings | undefined {
  const settings = modelSettingsFor(configs, model, options);
  const option = options.find(item => item.model === model);
  if (!option?.supportsReasoning || !option.reasoningEfforts?.length) return settings;
  const effort = override?.model === model ? override.value : settings?.reasoningEffort;
  const reasoningEffort = effort && option.reasoningEfforts.includes(effort) ? effort : '';
  return {...settings, reasoningEffort};
}

export function ComposerReasoning({option, settings, disabled, onChange}: {
  option?: AgentModelOption;
  settings?: AgentModelSettings;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  if (!option?.supportsReasoning || !option.reasoningEfforts?.length || option.unavailable) return null;

  return <Select className="agent-option agent-reasoning" size="sm" aria-label="思考强度"
    value={settings?.reasoningEffort ?? ''} disabled={disabled} onChange={onChange}
    options={[{value: '', label: '思考 · 上游默认'}, ...option.reasoningEfforts.map(value => ({
      value, label: `思考 · ${effortLabels.get(value) ?? value}`,
    }))]}/>;
}
