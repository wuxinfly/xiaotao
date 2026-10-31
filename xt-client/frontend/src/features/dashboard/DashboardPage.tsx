import { useEffect, useRef, useState } from 'react';
import { api, type DashboardSnapshot } from '../../lib/api';
import { UsageOverview } from './UsageOverview';
import { RecentRequests } from './RecentRequests';
import { UsageHeatmap } from './UsageHeatmap';
import './dashboard.css';

export function DashboardPage() {
  const [snap, setSnap] = useState<DashboardSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [nowMs, setNowMs] = useState(Date.now());
  const [uptimeSec, setUptimeSec] = useState(0);

  // Keep track of latest snapshot reference
  const snapRef = useRef<DashboardSnapshot | null>(null);
  snapRef.current = snap;

  // 1. Initial snapshot fetch & SSE subscription
  useEffect(() => {
    let mounted = true;

    const loadInitial = async () => {
      try {
        const data = await api.dashboard();
        if (mounted) {
          setSnap(data);
          setUptimeSec(data.launch.uptimeSeconds);
          setLoading(false);
          setError(null);
        }
      } catch (e) {
        if (mounted) {
          setError(String(e instanceof Error ? e.message : e));
          setLoading(false);
        }
      }
    };

    void loadInitial();

    const unsubscribe = api.subscribeDashboard(
      (update) => {
        if (!mounted) return;
        setConnected(true);
        setError(null);
        setSnap((prev) => {
          // Discard older updates based on versionSeq
          if (prev && update.versionSeq < prev.versionSeq) {
            return prev;
          }
          setUptimeSec(update.launch.uptimeSeconds);
          return update;
        });
      },
      () => {
        if (mounted) {
          setConnected(false);
        }
      }
    );

    return () => {
      mounted = false;
      unsubscribe();
    };
  }, []);

  // 2. 1-second live ticker for running durations and uptime
  useEffect(() => {
    const timer = setInterval(() => {
      setNowMs(Date.now());
      setUptimeSec((s) => s + 1);
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  if (loading) {
    return (
      <div className="dashboard-page" role="status" aria-busy="true">
        <div className="flow-spinner" style={{ margin: '80px auto' }} />
      </div>
    );
  }

  if (error && !snap) {
    return (
      <div className="dashboard-page">
        <div className="notice err" role="alert" style={{ marginTop: 40 }}>
          加载仪表盘数据失败：{error}
          <div style={{ marginTop: 12 }}>
            <button
              className="btn primary"
              onClick={() => {
                setLoading(true);
                setError(null);
                api.dashboard()
                  .then((d) => {
                    setSnap(d);
                    setUptimeSec(d.launch.uptimeSeconds);
                    setLoading(false);
                  })
                  .catch((e) => {
                    setError(String(e));
                    setLoading(false);
                  });
              }}
            >
              重新加载
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (!snap) return null;

  const isStorageError = snap.storage.status !== 'healthy';

  return (
    <div className="dashboard-page">
      {/* Page Header */}
      <header className="dashboard-header">
        <div className="dashboard-title-row">
          <h1>仪表盘</h1>
          <span
            className={`dashboard-badge ${
              isStorageError ? 'error' : connected ? 'connected' : 'disconnected'
            }`}
            title={
              isStorageError
                ? snap.storage.message
                : connected
                ? '已连接本地服务实时推送'
                : '正在尝试重新连接服务...'
            }
          >
            <span
              className={`status-dot ${
                isStorageError ? 'red' : connected ? '' : 'amber'
              }`}
              aria-hidden="true"
            />
            {isStorageError
              ? '存储异常'
              : connected
              ? '实时'
              : '重连中...'}
          </span>
        </div>
        <p className="dashboard-subtitle">实时用量与模型请求</p>
      </header>

      {/* Storage Degradation Alert */}
      {isStorageError && (
        <div className="storage-alert-bar" role="alert">
          <svg style={{ width: 16, height: 16, flex: 'none' }} viewBox="0 0 16 16" fill="none">
            <circle cx="8" cy="8" r="7" stroke="currentColor" strokeWidth="1.4" />
            <path d="M8 5V8.5M8 11V11.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
          </svg>
          <span>
            数据持久化异常：{snap.storage.message || 'SQLite 写入受阻'}。当前用量仅在服务内存中统计，恢复后将自动继续写入。
          </span>
        </div>
      )}

      {/* 1. Usage Overview Card */}
      <UsageOverview snap={snap} uptimeSeconds={uptimeSec} />

      {/* 2. Recent Requests Card */}
      <RecentRequests
        recentRequests={snap.recentRequests}
        activeRequests={snap.activeRequests}
        activeCount={snap.activeCount}
        nowMs={nowMs}
      />

      {/* 3. Usage Heatmap */}
      <UsageHeatmap heatmap={snap.heatmap} />
    </div>
  );
}
