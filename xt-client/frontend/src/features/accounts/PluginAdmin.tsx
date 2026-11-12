import {ChevronDown} from 'lucide-react';
import {useConfirm} from '../../components/confirm/ConfirmProvider';

type AdminPlugin = {
  id: string;
  enabled: boolean;
  manifest: {name: string; version: string; entrypoint: string};
  packageHash?: string;
  diagnostics?: boolean;
  diagnosticsPath?: string;
  previous?: {manifest: {version: string}; packageHash?: string};
};

type Props = {
  plugin: AdminPlugin;
  busy: boolean;
  onUninstall: () => void;
  onRollback: () => void;
  onDiagnostics: (enabled: boolean) => void;
};

export function PluginAdmin({plugin, busy, onUninstall, onRollback, onDiagnostics}: Props) {
  const confirm = useConfirm();
  const previous = plugin.previous;
  async function rollback() {
    if (!previous || !await confirm({title: `回滚到 v${previous.manifest.version}？`, description: '将恢复旧插件包；账号和模型设置保留，模型目录需要重新刷新。', confirmLabel: '回滚'})) return;
    onRollback();
  }
  async function uninstall() {
    if (!await confirm({title: `卸载 ${plugin.manifest.name}？`, description: '插件包、账号凭证和本地状态都会删除，无法撤销。', confirmLabel: '卸载插件'})) return;
    onUninstall();
  }
  return <details className="plugin-meta">
    <summary><ChevronDown className="plugin-chevron" size={13} aria-hidden="true"/>插件详情</summary>
    <div className="plugin-meta-facts">
      <span>插件 ID：{plugin.id}</span>
      <span>入口：{plugin.manifest.entrypoint}</span>
      {plugin.packageHash && <span>包指纹：<code title={plugin.packageHash}>{plugin.packageHash.slice(0, 12)}</code></span>}
      {previous && <span>可回滚：v{previous.manifest.version}</span>}
    </div>
    <div className="plugin-admin">
      <label className="plugin-admin-toggle">
        <input type="checkbox" checked={plugin.diagnostics === true} disabled={busy} onChange={event => onDiagnostics(event.target.checked)}/>
        记录插件诊断输出
      </label>
      <span className="plugin-admin-note">
        诊断日志可能包含账号信息，仅在排查时开启。{plugin.diagnostics && plugin.diagnosticsPath ? <> 日志：<code>{plugin.diagnosticsPath}</code></> : null}
      </span>
      <div className="plugin-admin-actions">
        {previous && <button type="button" className="app-modal-button secondary" disabled={busy || plugin.enabled}
          title={plugin.enabled ? '请先停用插件' : undefined} onClick={() => void rollback()}>回滚到 v{previous.manifest.version}</button>}
        <button type="button" className="app-modal-button secondary plugin-danger-button" disabled={busy || plugin.enabled}
          title={plugin.enabled ? '请先停用插件' : undefined} onClick={() => void uninstall()}>卸载插件</button>
      </div>
    </div>
  </details>;
}