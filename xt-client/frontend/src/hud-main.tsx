import {useEffect} from 'react';
import {createRoot} from 'react-dom/client';
import {RequestHud} from './features/dashboard/RequestHud';
import {DashboardDataProvider} from './features/dashboard/DashboardData';
import './features/dashboard/request-hud.css';
import './hud-window.css';

declare global {
  interface Window {
    xtHud?: (action: string) => Promise<void>;
  }
}

function HudApp() {
  const desktop = typeof window.xtHud === 'function';
  useEffect(() => {
    if (!desktop) return;
    const drag = (event: PointerEvent) => {
      if (event.button !== 0) return;
      if ((event.target as HTMLElement | null)?.closest('button')) return;
      event.preventDefault();
      void window.xtHud?.('drag');
    };
    const header = document.querySelector<HTMLElement>('.request-hud-header');
    header?.addEventListener('pointerdown', drag as EventListener);
    return () => header?.removeEventListener('pointerdown', drag as EventListener);
  }, []);
  return (
    <DashboardDataProvider enabled>
      <RequestHud enabled agentPage={false} dashboardPage={false} onOpen={() => {}} onClose={() => window.xtHud?.('close')} />
    </DashboardDataProvider>
  );
}

createRoot(document.getElementById('root')!).render(<HudApp />);
