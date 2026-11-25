import {useId} from 'react';
import {Select} from '../../components/select/Select';
import type {ToolPermissions} from '../../lib/team';

const permissionOptions = [
  ['read', '读取文件'],
  ['write', '修改文件'],
  ['command', '执行命令'],
  ['mcp', '使用浏览器和 MCP'],
] as const;

export function RoleToolPermissions({value, onChange, disabled}: {
  value?: ToolPermissions; onChange: (value: ToolPermissions) => void; disabled: boolean;
}) {
  const label = useId();
  const mode = !value || value.inherit ? 'inherit' : 'custom';
  return <div className="role-tool-permissions">
    <label><span id={label}>任务工具权限</span><Select aria-labelledby={label} value={mode} disabled={disabled}
      onChange={next => onChange(next === 'inherit' ? {inherit: true} : {read: true, write: true, command: true, mcp: true})}
      options={[{value: 'inherit', label: '跟随小涛'}, {value: 'custom', label: '自定义'}]}/></label>
    {mode === 'custom' && <fieldset aria-label="允许使用的工具" disabled={disabled}>
      {permissionOptions.map(([key, name]) => <label key={key}>
        <input type="checkbox" checked={!!value?.[key]} onChange={event => onChange({...value, [key]: event.target.checked})}/>{name}
      </label>)}
    </fieldset>}
    <small>仅在接单后生效，审批沿用小涛设置。命令和 MCP 可自行访问文件。</small>
  </div>;
}
