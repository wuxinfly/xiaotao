import {Box, RefreshCw, ShieldAlert} from 'lucide-react';

export type IndexItem = {
  id: string; name: string; version: string; path: string; resolvedPath: string;
  description?: string; capabilities?: string[]; hashMatches: boolean;
  installed: boolean; installedVersion?: string; enabled: boolean; updateAvailable: boolean; error?: string;
};

export function PluginIndex({items, busy, onInstall, onUpdate}: {
  items: IndexItem[]; busy: boolean;
  onInstall: (item: IndexItem) => void;
  onUpdate: (item: IndexItem) => void;
}) {
  if (!items.length) return null;
  return <div className="plugin-index">
    <div className="plugin-packages-heading">
      <div><strong>插件索引</strong><span>读取 plugins/index.json，只安装你信任的本地包</span></div>
    </div>
    {items.map(item => <div className={`plugin-package${item.error ? ' is-invalid' : ''}`} key={item.id}>
      <span className="plugin-package-icon">{item.error ? <ShieldAlert size={16}/> : <Box size={16}/>}</span>
      <div className="plugin-package-main">
        <strong>{item.name || item.id}<span className="plugin-version">v{item.version}</span></strong>
        {item.description && <small>{item.description}</small>}
        {item.error ? <small className="plugin-package-error">{item.error}</small> :
          <small title={item.resolvedPath}>
            {item.installed ? `已安装 v${item.installedVersion}` : '未安装'}
            {!item.hashMatches && ' · 指纹与索引不一致'}
            {item.capabilities?.length ? ` · ${item.capabilities.join(' / ')}` : ''}
          </small>}
      </div>
      {!item.error && (!item.installed
        ? <button className="app-modal-button primary" disabled={busy || !item.hashMatches} onClick={() => onInstall(item)}>安装</button>
        : item.updateAvailable
          ? <button className="app-modal-button secondary" disabled={busy || !item.hashMatches} onClick={() => onUpdate(item)}><RefreshCw size={13}/>{item.enabled ? '停用并更新' : '更新'}</button>
          : <button className="app-modal-button secondary" disabled>已安装</button>)}
    </div>)}
  </div>;
}