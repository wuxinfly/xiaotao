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
      <label className="model-catalog-account">
        <span>查看账号</span>
        <select value={selectedId} onChange={e => setAccountId(e.target.value)}>
          {accounts.map(a => <option key={a.id} value={a.id}>{a.alias || a.nickname || a.uid}</option>)}
        </select>
      </label>
      <span className="model-catalog-source">{loading ? '正在获取…' : catalog ? `${({ live: '实时获取', builtin: '内置目录', merged: '实时 + 内置目录', cached: '账号缓存', stale: '刷新失败 · 上次目录', error: '目录不可用' }[catalog.source])} · ${(catalog.models?.length ?? 0)} 个模型` : '获取失败'}</span>
      <button type="button" className="group-add-btn" disabled={loading} onClick={() => setRevision(n => n + 1)}>
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <polyline points="23 4 23 10 17 10" />
          <polyline points="1 20 1 14 7 14" />
          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
        </svg>
        <span>{loading ? '刷新中…' : '刷新列表'}</span>
      </button>
    </div>
    <p className="model-catalog-note">实际调用按来源匹配账号、按当前账号顺序选择，并跳过已知额度用完的账号。内置目录不代表当前账号已验证。</p>
    <div aria-live="polite">
      {loading && !catalog && <div className="model-catalog-state" aria-label="正在获取模型目录"><span className="model-catalog-spinner" aria-hidden="true" />正在获取模型目录…</div>}
      {error && <p className="model-catalog-error" role="alert">{error}，可点击刷新重试。</p>}
      {!loading && catalog?.models.length === 0 && <div className="model-catalog-state">该账号暂无模型目录。</div>}
      <div className="model-catalog-list">{!loading && catalog?.models?.map(model => {
        const id = `${providerId}/${model.id}`;
        return <div className="model-catalog-row" key={id}>
          <div className="model-catalog-copy">
            <strong>{model.displayName || model.id}</strong>
            <code>{id}</code>
          </div>
          <button type="button" className="model-catalog-copy-btn" onClick={() => void copy(id)} aria-label={`复制 ${id}`}>
            {copied === id ? (
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12" /></svg>
            ) : (
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>
            )}
            <span>{copied === id ? '已复制' : '复制 ID'}</span>
          </button>
        </div>;
      })}</div>
    </div>
  </section>;
}
