import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  type Account,
  type LoginSession,
  type ProviderInfo,
} from "../../lib/api";
import { toast } from "../../components/toast";
import { ModelEndpointCard } from "./components/ModelEndpointCard";
import { ProviderGroup } from "./components/ProviderGroup";
import { AccountDetailModal } from "./components/AccountDetailModal";
import { AddAccountModal } from "./components/AddAccountModal";
import "./accounts.css";

type Flow = "idle" | "waiting" | "error";

export function AccountsPage() {
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [flow, setFlow] = useState<Flow>("idle");
  const [session, setSession] = useState<LoginSession | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [loginProvider, setLoginProvider] = useState("trae");

  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const [showAddModal, setShowAddModal] = useState(false);

  const pollRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const load = useCallback(async () => {
    try {
      const [acctData, provData] = await Promise.all([
        api.accounts(),
        api.providers().catch(() => ({ providers: [] })),
      ]);
      setAccounts(acctData.accounts ?? []);
      if (provData.providers) {
        setProviders(provData.providers);
      }
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => () => clearInterval(pollRef.current), []);
  useEffect(() => {
    api.proxyKey().then(r => setApiKey(r.key)).catch(() => setApiKey(""));
  }, []);

  const providerMap = useMemo(() => {
    const map = new Map<string, ProviderInfo>();
    for (const p of providers) {
      map.set(p.id.toLowerCase(), p);
    }
    return map;
  }, [providers]);

  const groupedAccounts = useMemo(() => {
    const groups: { [provider: string]: Account[] } = {};
    for (const a of accounts) {
      const p = (a.provider || "unknown").toLowerCase();
      if (!groups[p]) groups[p] = [];
      groups[p].push(a);
    }
    return groups;
  }, [accounts]);

  const selectedAccount = useMemo(() => {
    if (!selectedAccountId) return null;
    return accounts.find(a => a.id === selectedAccountId) || null;
  }, [accounts, selectedAccountId]);

  const startLogin = async (providerId = "trae", region = "cn") => {
    setShowAddModal(false);
    setFlow("waiting");
    try {
      setLoginProvider(providerId);
      const sess = await api.startLogin(providerId, region);
      setSession(sess);
      clearInterval(pollRef.current);
      pollRef.current = setInterval(async () => {
        try {
          const state = await api.pollLogin(sess.id, providerId);
          if (state.status === "succeeded") {
            clearInterval(pollRef.current);
            setFlow("idle");
            setSession(null);
            toast.success("账号添加成功");
            void load();
          } else if (state.status !== "waiting") {
            clearInterval(pollRef.current);
            setFlow("error");
            toast.error(state.error ?? (providerId + " 登录未完成"));
          }
        } catch { /* keep polling */ }
      }, 1500);
    } catch (e) {
      setFlow("error");
      toast.error(String(e instanceof Error ? e.message : e));
    }
  };

  const cancelLogin = async () => {
    clearInterval(pollRef.current);
    if (session) void api.cancelLogin(session.id, loginProvider).catch(() => {});
    setFlow("idle");
    setSession(null);
  };

  const handleToggleEnable = async (a: Account) => {
    setBusyId(a.id);
    try {
      await api.updateAccount(a.id, { enabled: !a.enabled });
      await load();
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId(null);
    }
  };

  const handleDeleteAccount = async (a: Account) => {
    const name = a.alias || a.nickname || a.uid;
    if (!window.confirm("确定移除账号「" + name + "」？本地凭据会一并删除。")) return;
    setBusyId(a.id);
    try {
      await api.deleteAccount(a.id);
      await load();
      if (selectedAccountId === a.id) {
        setSelectedAccountId(null);
      }
      toast.success("账号已移除");
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId(null);
    }
  };

  const handleRefreshQuota = async (a: Account) => {
    setBusyId(a.id);
    try {
      await api.refreshQuota(a.id);
      await load();
      toast.success("配额已刷新");
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId(null);
    }
  };

  const handleRotateKey = async () => {
    try {
      const r = await api.rotateProxyKey();
      setApiKey(r.key);
      toast.success("代理密钥已重新生成，外部工具需要更新 API Key");
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    }
  };

  if (flow === "waiting") {
    return (
      <section className="accounts-flow" aria-live="polite">
        <div className="flow-card">
          <h2>
            等待 {providerMap.get(loginProvider)?.displayName ?? loginProvider} 授权
          </h2>
          <p>
            已在浏览器打开授权页面。完成登录后自动接入；Qoder 授权链接 10 分钟内有效。
          </p>
          {session?.loginUrl && (
            <div className="copy-field">
              <span className="copy-label">授权链接</span>
              <button
                type="button"
                className="copy-value mono"
                onClick={() => void navigator.clipboard.writeText(session.loginUrl)}
                title="点击复制"
              >
                <span className="copy-text">{session.loginUrl}</span>
              </button>
            </div>
          )}
          <div className="flow-spinner" aria-hidden="true" />
          <div className="flow-actions">
            {session?.loginUrl && (
              <button
                type="button"
                className="btn primary"
                onClick={() => window.open(session.loginUrl, "_blank", "noopener,noreferrer")}
              >
                打开授权页面
              </button>
            )}
            <button className="btn" onClick={cancelLogin}>取消</button>
          </div>
        </div>
      </section>
    );
  }

  if (loading) return <section className="accounts-flow" aria-busy="true"><p className="muted">加载中…</p></section>;

  if (accounts.length === 0) {
    return (
      <div className="accounts-page-container">
        <div className="accounts-header-bar">
          <div className="accounts-header-titles">
            <h1 className="accounts-main-title">账号与配额</h1>
            <p className="accounts-main-subtitle">管理不同接入来源的账号与模型调用额度</p>
          </div>
          <button
            type="button"
            className="btn-add-account-primary"
            onClick={() => setShowAddModal(true)}
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            <span>添加账号</span>
          </button>
        </div>

        <ModelEndpointCard
          origin={window.location.origin}
          apiKey={apiKey}
          onRotateKey={handleRotateKey}
        />

        <div className="accounts-flow" style={{ padding: "30px 0" }}>
          <div className="flow-card">
            <h2>还没有接入账号</h2>
            <p>添加来源账号后，即可查看实时配额并接入本地模型代理。</p>
            <button className="btn primary" onClick={() => setShowAddModal(true)}>添加账号</button>
          </div>
        </div>

        {showAddModal && (
          <AddAccountModal
            providers={providers}
            onSelectProvider={(pId, region) => startLogin(pId, region)}
            onClose={() => setShowAddModal(false)}
          />
        )}
      </div>
    );
  }

  const providerKeys = Object.keys(groupedAccounts);

  return (
    <div className="accounts-page-container">
      <div className="accounts-header-bar">
        <div className="accounts-header-titles">
          <h1 className="accounts-main-title">账号与配额</h1>
          <p className="accounts-main-subtitle">管理不同接入来源的账号与模型调用额度</p>
        </div>
        <button
          type="button"
          className="btn-add-account-primary"
          onClick={() => setShowAddModal(true)}
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          <span>添加账号</span>
        </button>
      </div>

      <ModelEndpointCard
        origin={window.location.origin}
        apiKey={apiKey}
        onRotateKey={handleRotateKey}
      />

      <div className="accounts-groups-container">
        <div className="sources-section-title-row">
          <h2 className="sources-section-title">接入来源</h2>
          <span className="sources-section-sub">按反代服务统一管理</span>
        </div>

        {providerKeys.map((pKey) => (
          <ProviderGroup
            key={pKey}
            providerId={pKey}
            providerInfo={providerMap.get(pKey)}
            accounts={groupedAccounts[pKey]}
            busyId={busyId}
            onAddAccount={() => setShowAddModal(true)}
            onToggleEnable={handleToggleEnable}
            onRefreshQuota={handleRefreshQuota}
            onOpenDetail={(acct) => setSelectedAccountId(acct.id)}
          />
        ))}
      </div>

      {selectedAccount && (
        <AccountDetailModal
          account={selectedAccount}
          providerInfo={providerMap.get(selectedAccount.provider.toLowerCase())}
          isBusy={busyId === selectedAccount.id}
          onClose={() => setSelectedAccountId(null)}
          onToggleEnable={handleToggleEnable}
          onRefreshQuota={handleRefreshQuota}
          onDeleteAccount={handleDeleteAccount}
          onRelogin={acct => { setSelectedAccountId(null); void startLogin(acct.provider, acct.region); }}
        />
      )}

      {showAddModal && (
        <AddAccountModal
          providers={providers}
          onSelectProvider={(pId, region) => startLogin(pId, region)}
          onClose={() => setShowAddModal(false)}
        />
      )}
    </div>
  );
}
