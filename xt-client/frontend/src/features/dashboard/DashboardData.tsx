import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { api, type Account, type DashboardSnapshot } from '../../lib/api';

interface DashboardDataValue {
  snap: DashboardSnapshot | null;
  accounts: Account[];
  connected: boolean;
  loading: boolean;
  error: string | null;
  nowMs: number;
  reload: () => Promise<void>;
}

const DashboardDataContext = createContext<DashboardDataValue | null>(null);

export function DashboardDataProvider({
  enabled,
  children,
}: {
  enabled: boolean;
  children: ReactNode;
}) {
  const [snap, setSnap] = useState<DashboardSnapshot | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nowMs, setNowMs] = useState(Date.now());
  const snapRef = useRef<DashboardSnapshot | null>(null);
  snapRef.current = snap;

  const applySnapshot = useCallback((next: DashboardSnapshot) => {
    setSnap((previous) => {
      if (previous && next.versionSeq < previous.versionSeq) return previous;
      return next;
    });
  }, []);

  const reload = useCallback(async () => {
    if (!snapRef.current) setLoading(true);
    try {
      const [next, accountResult] = await Promise.all([
        api.dashboard(),
        api.accounts().catch(() => null),
      ]);
      applySnapshot(next);
      if (accountResult) setAccounts(accountResult.accounts ?? []);
      setError(null);
      setLoading(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      setLoading(false);
    }
  }, [applySnapshot]);

  useEffect(() => {
    if (!enabled) {
      setConnected(false);
      return;
    }

    let mounted = true;
    void reload();

    const unsubscribe = api.subscribeDashboard(
      (next) => {
        if (!mounted) return;
        applySnapshot(next);
        setConnected(true);
        setError(null);
        setLoading(false);
      },
      () => {
        if (mounted) setConnected(false);
      },
    );

    return () => {
      mounted = false;
      unsubscribe();
    };
  }, [applySnapshot, enabled, reload]);

  useEffect(() => {
    if (!enabled) return;
    const refreshAccounts = () => {
      api.accounts()
        .then((result) => setAccounts(result.accounts ?? []))
        .catch(() => {
          // Account aliases are decorative. Requests still show their IDs.
        });
    };
    const timer = window.setInterval(refreshAccounts, 30_000);
    window.addEventListener('focus', refreshAccounts);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener('focus', refreshAccounts);
    };
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNowMs(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [enabled]);

  return (
    <DashboardDataContext.Provider
      value={{ snap, accounts, connected, loading, error, nowMs, reload }}
    >
      {children}
    </DashboardDataContext.Provider>
  );
}

export function useDashboardData() {
  const value = useContext(DashboardDataContext);
  if (!value) {
    throw new Error('useDashboardData must be used inside DashboardDataProvider');
  }
  return value;
}
