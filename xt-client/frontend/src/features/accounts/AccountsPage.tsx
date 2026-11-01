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
import { ModelCatalogModal } from "./components/ModelCatalogModal";
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
  const [reorderingProvider, setReorderingProvider] = useState<string | null>(null);
  const [loginProvider, setLoginProvider] = useState("trae");
  const [manualCallbackInput, setManualCallbackInput] = useState("");
  const [manualSubmitting, setManualSubmitting] = useState(false);

  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const [selectedModelProvider, setSelectedModelProvider] = useState<string | null>(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [showImportModal, setShowImportModal] = useState(false);
  const [importJsonText, setImportJsonText] = useState("");
  const [importing, setImporting] = useState(false);

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

  const selectedModelAccounts = useMemo(() => {
    if (!selectedModelProvider) return [];
    return groupedAccounts[selectedModelProvider] ?? [];
  }, [groupedAccounts, selectedModelProvider]);

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

  const handleReorderAccounts = async (providerId: string, orderedIDs: string[]) => {
    if (reorderingProvider) return;
    const previous = accounts;
    const providerAccounts = accounts.filter(a => a.provider.toLowerCase() === providerId);
    const byID = new Map(providerAccounts.map(a => [a.id, a]));
    const ordered = orderedIDs.map(id => byID.get(id)).filter((a): a is Account => Boolean(a));
    if (ordered.length !== providerAccounts.length) {
      toast.error("账号顺序不完整，请刷新后重试");
      return;
    }

    let position = 0;
    const optimistic = accounts.map(a => {
      if (a.provider.toLowerCase() !== providerId) return a;
      const next = ordered[position];
      position++;
      return next ?? a;
    });
    setAccounts(optimistic);
    setReorderingProvider(providerId);
    try {
      const res = await api.reorderAccounts(providerId, orderedIDs);
      setAccounts(res.accounts ?? []);
    } catch (e) {
      setAccounts(previous);
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setReorderingProvider(null);
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

  const handleCheckin = async (a: Account) => {
    setBusyId(a.id);
    try {
      const res = await api.claimCheckin(a.id);
      if (res.ok) {
        const bonus = res.extraCredits ? `，获得 +${res.extraCredits} 积分` : '';
        const streak = res.streakDays ? `（已连续签到 ${res.streakDays} 天）` : '';
        toast.success(`签到成功${bonus}${streak}`);
      } else {
        toast.info(res.message || "签到未成功");
      }
      await load();
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId(null);
    }
  };

  const handleCheckinAll = async (providerId?: string) => {
    try {
      toast.info("正在执行签到打卡...");
      const { results } = await api.claimAllCheckin();
      let successCount = 0;
      let totalPoints = 0;
      for (const [id, r] of Object.entries(results)) {
        if (providerId) {
          const acct = accounts.find(a => a.id === id);
          if (acct && acct.provider.toLowerCase() !== providerId.toLowerCase()) {
            continue;
          }
        }
        if (r.ok) {
          successCount++;
          if (r.extraCredits) totalPoints += r.extraCredits;
        }
      }
      if (successCount > 0) {
        const bonus = totalPoints > 0 ? `，共获得 +${totalPoints} 积分` : '';
        toast.success(`已完成 ${successCount} 个账号签到${bonus}`);
      } else {
        toast.info("今日没有需要签到的账号或已全部签过");
      }
      await load();
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
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

  const handleManualCallbackSubmit = async () => {
    if (!session || !manualCallbackInput.trim()) return;
    setManualSubmitting(true);
    try {
      await api.submitCodexCallback(session.id, manualCallbackInput.trim());
      clearInterval(pollRef.current);
      setFlow("idle");
      setSession(null);
      setManualCallbackInput("");
      toast.success("Codex 账号添加成功");
      void load();
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setManualSubmitting(false);
    }
  };

  const handleImportCodex = async () => {
    if (!importJsonText.trim()) return;
    setImporting(true);
    try {
      const res = await api.importCodexAuth(importJsonText.trim());
      setShowImportModal(false);
      setImportJsonText("");
      toast.success("Codex 凭据导入成功: " + (res.nickname || res.accountId));
      void load();
    } catch (e) {
      toast.error(String(e instanceof Error ? e.message : e));
    } finally {
      setImporting(false);
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
            {loginProvider === "codex"
              ? "已在浏览器打开授权页面。浏览器完成授权后将自动回调 localhost:1455；若端口被占用或未自动跳转，可将地址栏完整 URL 或 code 粘贴到下方。"
              : "已在浏览器打开授权页面。完成登录后自动接入；Qoder 授权链接 10 分钟内有效。"}
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
          {loginProvider === "codex" && (
            <div style={{ marginTop: 16, display: "flex", flexDirection: "column", gap: 8, textAlign: "left" }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>手动完成（粘贴回调 URL 或 code）</span>
              <div style={{ display: "flex", gap: 8 }}>
                <input
                  type="text"
                  placeholder="http://localhost:1455/auth/callback?code=..."
                  value={manualCallbackInput}
                  onChange={(e) => setManualCallbackInput(e.target.value)}
                  style={{
                    flex: 1,
                    padding: "8px 12px",
                    borderRadius: 7,
                    border: "1px solid #1b3a4a",
                    background: "#0a1c29",
                    color: "#cfe6ef",
                    fontSize: 12,
                  }}
                />
                <button
                  type="button"
                  className="btn primary"
                  disabled={manualSubmitting || !manualCallbackInput.trim()}
                  onClick={handleManualCallbackSubmit}
                >
                  {manualSubmitting ? "提交中…" : "提交"}
                </button>
              </div>
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
            onImportCodex={() => { setShowAddModal(false); setShowImportModal(true); }}
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
          <span className="sources-section-sub">{providerKeys.length} 个来源 · {accounts.length} 个账号</span>
        </div>
        <p className="sources-order-hint">请求会按各来源内的账号顺序，从上到下选择。</p>

        {providerKeys.map((pKey) => (
          <ProviderGroup
            key={pKey}
            providerId={pKey}
            providerInfo={providerMap.get(pKey)}
            accounts={groupedAccounts[pKey]}
            busyId={busyId}
            isReordering={reorderingProvider === pKey}
            onAddAccount={() => setShowAddModal(true)}
            onToggleEnable={handleToggleEnable}
            onRefreshQuota={handleRefreshQuota}
            onCheckin={handleCheckin}
            onCheckinAll={handleCheckinAll}
            onReorder={handleReorderAccounts}
            onOpenDetail={(acct) => setSelectedAccountId(acct.id)}
            onOpenModels={() => setSelectedModelProvider(pKey)}
          />
        ))}
      </div>

      {selectedModelProvider && selectedModelAccounts.length > 0 && (
        <ModelCatalogModal
          providerId={selectedModelProvider}
          providerInfo={providerMap.get(selectedModelProvider)}
          accounts={selectedModelAccounts}
          onClose={() => setSelectedModelProvider(null)}
        />
      )}

      {selectedAccount && (
        <AccountDetailModal
          account={selectedAccount}
          providerInfo={providerMap.get(selectedAccount.provider.toLowerCase())}
          isBusy={busyId === selectedAccount.id}
          onClose={() => setSelectedAccountId(null)}
          onToggleEnable={handleToggleEnable}
          onRefreshQuota={handleRefreshQuota}
          onCheckin={handleCheckin}
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
