import {Box, RefreshCw} from 'lucide-react';

export type LocalPackage = {
  path: string;
  manifest: {id: string; name: string; version: string; entrypoint: string; capabilities?: string[]};
  installedVersion?: string;
  updateAvailable: boolean;
  enabled: boolean;
};

export function PluginPackages({packages, busy, onInstall, onUpdate, onRefresh}: {
  packages: LocalPackage[]; busy: boolean;
  onInstall: (item: LocalPackage) => void;
  onUpdate: (item: LocalPackage) => void;
  onRefresh: () => void;
}) {
  return <div className="plugin-packages">
    <div className="plugin-packages-heading">
      <div><strong>发现本机插件包</strong><span>只读取清单，不执行代码</span></div>
      <button type="button" className="plugin-packages-refresh" disabled={busy} onClick={onRefresh}><RefreshCw size={12}/>重新扫描</button>
    </div>
    {!packages.length && <p className="plugin-packages-empty">没有在程序旁的 plugins 或 dist/plugins 找到有效插件包。</p>}
    {packages.map(item => {
      const installed = Boolean(item.installedVersion);
      const current = !installed ? '未安装' : item.updateAvailable ? `已安装 v${item.installedVersion}` : `已安装 v${item.installedVersion}（最新）`;
      return <div className="plugin-package" key={item.path}>
        <span className="plugin-package-icon"><Box size={16}/></span>
        <div className="plugin-package-main">
          <strong>{item.manifest.name}<span className="plugin-version">v{item.manifest.version}</span></strong>
          <small title={item.path}>{current} · {item.path}</small>
        </div>
        {!installed && <button className="app-modal-button primary" disabled={busy} onClick={() => onInstall(item)}>安装</button>}
        {installed && item.updateAvailable && <button className="app-modal-button secondary" disabled={busy} onClick={() => onUpdate(item)}>
          <RefreshCw size={13}/>{item.enabled ? '停用并更新' : '更新'}
        </button>}
        {installed && !item.updateAvailable && <button className="app-modal-button secondary" disabled>已安装</button>}
      </div>;
    })}
  </div>;
}
