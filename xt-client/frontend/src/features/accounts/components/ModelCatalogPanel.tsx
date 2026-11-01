import { useEffect, useState } from 'react';
import { api, type Account, type ModelCatalog } from '../../../lib/api';
import { toast } from '../../../components/toast';

export function ModelCatalogPanel({ accounts, providerId }: { accounts: Account[]; providerId: string }) {
  const [accountId, setAccountId] = useState(accounts[0]?.id ?? '');
  const [catalog, setCatalog] = useState<ModelCatalog | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [copied, setCopied] = useState('');
  const selectedId = accounts.some(a => a.id === accountId) ? accountId : accounts[0]?.id ?? '';

  useEffect(() => {
    let active = true;
    setLoading(true); setError(''); setCatalog(null);
    api.accountModels(selectedId).then(data => { if (active) { setCatalog(data); setError(data.error ?? ""); } })
      .catch(e => { if (active) setError(e instanceof Error ? e.message : String(e)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [selectedId, revision]);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(''), 1600);
    return () => clearTimeout(timer);
  }, [copied]);

  async function copy(id: string) {
    try { await navigator.clipboard.writeText(id); setCopied(id); toast.success('模型 ID 已复制'); }
    catch { toast.error('复制失败，请手动选择模型 ID'); }
  }

  return <section className="model-catalog" aria-label="来源模型目录" aria-busy={loading}>
    <div className="model-catalog-toolbar">
      <label>账号 <select value={selectedId} onChange={e => setAccountId(e.target.value)}>
        {accounts.map(a => <option key={a.id} value={a.id}>{a.alias || a.nickname || a.uid}</option>)}
      </select></label>
      <span className="model-catalog-source">{loading ? '正在获取…' : catalog ? `${({ live: '实时获取', builtin: '内置目录', merged: '实时 + 内置目录', cached: '账号缓存', stale: '刷新失败 · 上次目录', error: '目录不可用' }[catalog.source])} · ${(catalog.models?.length ?? 0)} 个模型` : '获取失败'}</span>
      <button className="group-add-btn" disabled={loading} onClick={() => setRevision(n => n + 1)}>刷新列表</button>
    </div>
    <p className="model-catalog-note">复制完整模型 ID 到调用工具中；实际调用按模型来源匹配账号，按添加顺序选择，并跳过已知额度用完的账号。内置目录不代表已通过当前账号验证。</p>
    <div aria-live="polite">
      {error && <p role="alert">{error}，可点击刷新重试。</p>}
      {catalog?.models.length === 0 && <p>该账号暂无模型目录。</p>}
      <div className="model-catalog-list">{catalog?.models?.map(model => {
        const id = `${providerId}/${model.id}`;
        return <div className="model-catalog-row" key={id}>
          <div><strong>{model.displayName || model.id}</strong><code>{id}</code></div>
          <button className="group-add-btn" onClick={() => void copy(id)} aria-label={`复制 ${id}`}>{copied === id ? '已复制' : '复制 ID'}</button>
        </div>;
      })}</div>
    </div>
  </section>;
}
