import {useEffect, useRef} from 'react';
import {createRoot} from 'react-dom/client';
import {RequestHud} from './features/dashboard/RequestHud';
import {DashboardDataProvider} from './features/dashboard/DashboardData';
import './features/dashboard/request-hud.css';
import './hud-window.css';
import './shallow-sea.css';
import './lib/theme';

declare global {
  interface Window {
    xtHud?: (action: string) => Promise<void>;
  }
}

function HudApp() {
  const desktop = typeof window.xtHud === 'function';
  const ready = useRef(false);
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
  }, [desktop]);
  useEffect(() => {
    if (!desktop || ready.current) return;
    const frame = window.requestAnimationFrame(() => {
      if (ready.current) return;
      ready.current = true;
      void window.xtHud?.('ready');
    });
    return () => window.cancelAnimationFrame(frame);
  }, [desktop]);
  return (
    <DashboardDataProvider enabled>
      <RequestHud enabled agentPage={false} dashboardPage={false} onOpen={() => {}} onClose={() => window.xtHud?.('close')} />
    </DashboardDataProvider>
  );
}

createRoot(document.getElementById('root')!).render(<HudApp />);
