import {useState} from 'react';
import {Settings2} from 'lucide-react';
import {Select} from '../../components/select/Select';

export type SettingField = {key: string; label: string; description?: string; placeholder?: string; default?: string; options?: string[]};

export function PluginSettings({fields, values, busy, onSave}: {
  fields: SettingField[]; values: Record<string, string>; busy: boolean;
  onSave: (values: Record<string, string>) => Promise<boolean>;
}) {
  const [draft, setDraft] = useState<Record<string, string> | null>(null);
  const current = draft ?? values;
  if (!fields.length) return null;
  return <section className="plugin-settings" aria-label="插件设置">
    <div className="plugin-section-head">
      <div className="plugin-section-title"><Settings2 size={14} aria-hidden="true"/><h4>插件设置</h4></div>
      <span className="plugin-section-note">保存后下次插件操作生效</span>
    </div>
    <div className="plugin-settings-grid">
      {fields.map(field => <label className="plugin-setting" key={field.key}>
        <span>{field.label}</span>
        {field.options?.length
          ? <Select value={current[field.key] ?? ''} disabled={busy} onChange={next => setDraft({...current, [field.key]: next})}
              options={field.options.map(option => ({value: option, label: option}))}/>
          : <input value={current[field.key] ?? ''} disabled={busy} placeholder={field.placeholder}
            maxLength={200} onChange={event => setDraft({...current, [field.key]: event.target.value})}/>}
        {field.description && <small>{field.description}</small>}
      </label>)}
    </div>
    <div className="plugin-settings-actions">
      {draft && <button type="button" className="plugin-link-button" disabled={busy} onClick={() => setDraft(null)}>取消更改</button>}
      <button type="button" className="app-modal-button secondary" disabled={busy || !draft} onClick={() => void onSave(current).then(saved => {if (saved) setDraft(null);})}>保存设置</button>
    </div>
  </section>;
}
