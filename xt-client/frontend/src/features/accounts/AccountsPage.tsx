import { useCallback, useEffect, useRef, useState } from 'react';
import { api, type Account, type LoginSession, type ProxySettings } from '../../lib/api';

type Flow = 'idle' | 'waiting' | 'error';

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { /* ignore */ }
    ta.remove();
    return ok;
  }
}

function CopyField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false);
  const doCopy = async () => {
    if (await copyText(value)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    }
  };
  return (
    <div className="copy-field">
      <span className="copy-label">{label}</span>
      <button type="button" className={`copy-value${mono ? ' mono' : ''}`} onClick={doCopy} title="点击复制">
        <span className="copy-text">{value}</span>
        <span className={`copy-hint${copied ? ' done' : ''}`}>{copied ? '已复制' : '复制'}</span>
      </button>
    </div>
  );
}

function formatQuota(item: { used?: number | null; total?: number | null; unit: string }) {
  if (item.used == null || item.total == null) return '额度未知';
  const fmt = (n: number) => (item.unit === 'dollar' ? `$${n.toFixed(2)}` : `${Math.round(n)} 积分`);
  return `${fmt(item.total - item.used)} / ${fmt(item.total)}`;
}

export function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [proxy, setProxy] = useState<ProxySettings>({ defaultAccountId: '' });
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [flow, setFlow] = useState<Flow>('idle');
  const [session, setSession] = useState<LoginSession | null>(null);
  const [notice, setNotice] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const load = useCallback(async () => {
    try {
      const data = await api.accounts();
      setAccounts(data.accounts ?? []);
      setProxy(data.proxy);
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => () => clearInterval(pollRef.current), []);
  useEffect(() => {
    api.proxyKey().then(r => setApiKey(r.key)).catch(() => setApiKey(''));
  }, []);

  const startLogin = async () => {
    setFlow('waiting');
    setNotice(null);
    try {
      const sess = await api.startLogin();
      setSession(sess);
      clearInterval(pollRef.current);
      pollRef.current = setInterval(async () => {
        try {
          const state = await api.pollLogin(sess.id);
          if (state.status === 'succeeded') {
            clearInterval(pollRef.current);
            setFlow('idle');
            setSession(null);
            setNotice({ kind: 'ok', text: '账号添加成功' });
            void load();
          } else if (state.status !== 'waiting') {
            clearInterval(pollRef.current);
            setFlow('error');
            setNotice({ kind: 'err', text: state.error ?? '登录未完成' });
          }
        } catch { /* keep polling */ }
      }, 1500);
    } catch (e) {
      setFlow('error');
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    }
  };

  const cancelLogin = async () => {
    clearInterval(pollRef.current);
    if (session) void api.cancelLogin(session.id).catch(() => {});
    setFlow('idle');
    setSession(null);
  };

  const toggle = async (a: Account) => {
    setBusyId(a.id);
    try {
      await api.updateAccount(a.id, { enabled: !a.enabled });
      await load();
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (a: Account) => {
    if (!window.confirm(`确定移除账号「${a.alias || a.nickname || a.uid}」？本地凭据会一并删除。`)) return;
    setBusyId(a.id);
    try {
      await api.deleteAccount(a.id);
      await load();
      setNotice({ kind: 'ok', text: '账号已移除' });
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    } finally {
      setBusyId(null);
    }
  };

  const refresh = async (a: Account) => {
    setBusyId(a.id);
    try {
      await api.refreshQuota(a.id);
      await load();
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    } finally {
      setBusyId(null);
    }
  };

  const setDefault = async (id: string) => {
    try {
      const next = await api.saveProxy({ defaultAccountId: id });
      setProxy(next);
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    }
  };

  const rotateKey = async () => {
    try {
      const r = await api.rotateProxyKey();
      setApiKey(r.key);
      setNotice({ kind: 'ok', text: '代理密钥已重新生成，外部工具需要更新 API Key' });
    } catch (e) {
      setNotice({ kind: 'err', text: String(e instanceof Error ? e.message : e) });
    }
  };

  if (flow === 'waiting') {
    return (
      <section className="accounts-flow" aria-live="polite">
        <div className="flow-card">
          <h2>等待 Trae 授权</h2>
          <p>已在默认浏览器打开授权页面。完成登录后这里会自动更新。</p>
          <div className="flow-spinner" aria-hidden="true" />
          <div className="flow-actions">
            <button className="btn" onClick={cancelLogin}>取消</button>
          </div>
          {notice && <div className={`notice ${notice.kind}`} role="status" style={{ marginTop: 16 }}>{notice.text}</div>}
        </div>
      </section>
    );
  }

  if (loading) return <section className="accounts-flow" aria-busy="true"><p className="muted">加载中…</p></section>;

  if (accounts.length === 0) {
    return (
      <section className="accounts-flow">
        <div className="flow-card">
          <h2>还没有接入账号</h2>
          <p>添加 Trae 国内版账号后，可以在这里查看真实配额，并把它接入本地模型代理。API 配置接入会在后续版本提供。</p>
          <button className="btn primary" onClick={startLogin}>添加 Trae 账号</button>
          {notice && <div className={`notice ${notice.kind}`} role="status" style={{ marginTop: 16 }}>{notice.text}</div>}
        </div>
      </section>
    );
  }

  return (
    <section className="accounts-page">
      {notice && <div className={`notice ${notice.kind}`} role="status">{notice.text}</div>}
      <div className="endpoint-card">
        <div className="endpoint-head">
          <strong>模型接入</strong>
          <small>在支持 OpenAI 接口的工具中填写以下信息即可调用已接入账号的模型</small>
        </div>
        <CopyField label="接口地址" value={`${window.location.origin}/v1`} mono />
        <CopyField label="API 密钥" value={apiKey ?? '加载中…'} mono />
        <div className="copy-field">
          <span className="copy-label">支持协议</span>
          <span className="copy-value static">OpenAI Chat Completions 兼容（/v1/chat/completions · /v1/models）</span>
        </div>
        {apiKey && (
          <div className="endpoint-foot">
            <button className="btn" onClick={rotateKey}>重新生成密钥</button>
          </div>
        )}
      </div>
      <ul className="account-list">
        {accounts.map(a => {
          const quota = a.quota;
          return (
            <li key={a.id} className={a.enabled ? '' : 'disabled'}>
              <div className="acct-head">
                <strong>{a.alias || a.nickname || `Trae ${a.uid}`}</strong>
                <span className={`badge auth-${a.authState}`}>
                  {a.authState === 'valid' ? '登录有效' : '需要重新登录'}
                </span>
              </div>
              <div className="acct-quota">
                {quota?.status === 'ok' && quota.items.length > 0
                  ? quota.items.slice(0, 3).map((it, i) => <span key={i} className="quota-line">{it.label}：{formatQuota(it)}</span>)
                  : quota?.status === 'error'
                    ? <span className="quota-line err">配额查询失败{quota.error ? `：${quota.error}` : ''}</span>
                    : <span className="quota-line muted">尚未查询配额</span>}
              </div>
              <div className="acct-actions">
                <button className="btn" disabled={busyId === a.id} onClick={() => refresh(a)}>刷新配额</button>
                <button className="btn" disabled={busyId === a.id} onClick={() => toggle(a)}>{a.enabled ? '停用' : '启用'}</button>
                <button className="btn" disabled={busyId === a.id} onClick={() => setDefault(a.id)}>{proxy.defaultAccountId === a.id ? '默认账号' : '设为默认'}</button>
                <button className="btn danger" disabled={busyId === a.id} onClick={() => remove(a)}>移除</button>
              </div>
            </li>
          );
        })}
      </ul>
      <div className="add-more">
        <button className="btn primary" onClick={startLogin}>添加 Trae 账号</button>
      </div>
    </section>
  );
}
