import {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import './style.css';
import {AccountsPage} from './features/accounts/AccountsPage';
import {DashboardPage} from './features/dashboard/DashboardPage';
import {DashboardDataProvider} from './features/dashboard/DashboardData';
import {OfficePage} from './features/office/OfficePage';
import {ToastContainer} from './components/toast';
import {WindowMenu} from './components/WindowMenu';
import {AgentPage} from './features/agent/AgentPage';
import {ConfirmProvider} from './components/confirm/ConfirmProvider';
import './navigation.css';
import './ocean-tech.css';
import './shallow-sea.css';
import {WorkspaceDock, WorkspaceBack, type Page} from './components/WorkspaceSidebar';

declare global {interface Window {xtWindow?: (action:string)=>Promise<void>}}

// Ctrl + wheel otherwise triggers the browser's page zoom, including in WebView2.
window.addEventListener('wheel', event => {
 if(event.ctrlKey) event.preventDefault();
}, {passive:false});

// Scroll events do not bubble, so capture them for nested and dynamically mounted areas.
const scrollbarTimers = new WeakMap<Element, ReturnType<typeof setTimeout>>();
document.addEventListener('scroll', event => {
 const target = event.target;
 if (!(target instanceof HTMLElement)) return;
 target.classList.add('is-scrolling');
 clearTimeout(scrollbarTimers.get(target));
 scrollbarTimers.set(target, setTimeout(() => target.classList.remove('is-scrolling'), 900));
}, true);

function App(){
 const [page,setPage]=useState<Page>('agent');
 const desktop=typeof window.xtWindow==='function';
 useEffect(()=>{document.documentElement.dataset.page=page;return()=>{delete document.documentElement.dataset.page}},[page]);
 useEffect(()=>{
  let cancelled=false;
  let timer:ReturnType<typeof setTimeout>|undefined;
  const images=Array.from(document.images).map(image=>image.decode().catch(()=>{}));
  void Promise.all([document.fonts.ready,...images]).then(()=>{
   // A timer also runs while the native host is hidden (rAF may be suspended).
   if(!cancelled)timer=setTimeout(()=>{if(!cancelled)void window.xtWindow?.('ready')},150);
  });
  return()=>{cancelled=true;clearTimeout(timer)};
 },[]);
 useEffect(()=>{
  const navigate=(event:Event)=>{const next=(event as CustomEvent<Page>).detail;if(next==='agent'||next==='office'||next==='dashboard'||next==='accounts')setPage(next)};
  window.addEventListener('xiaotao:navigate',navigate);
  return()=>window.removeEventListener('xiaotao:navigate',navigate);
 },[]);
 const select=(p:Page)=>setPage(p);
 return <div className="app">
  {desktop&&['n','s','w','e','nw','ne','sw','se'].map(edge=><div key={edge} aria-hidden="true" className={'resize-edge resize-'+edge} onPointerDown={e=>{if(e.button===0){e.preventDefault();window.xtWindow?.('resize-'+edge)}}}/>)}
  <div className="workspace-shell">
  <header className="titlebar">
   {page !== 'agent' && <WorkspaceBack onBack={() => setPage('agent')}/>}
   <div className="brand drag" onPointerDown={e=>{if(!desktop||e.button!==0)return;const x=e.screenX,y=e.screenY;const move=(event:PointerEvent)=>{if(Math.hypot(event.screenX-x,event.screenY-y)>5){cleanup();window.xtWindow?.('drag')}};const cleanup=()=>{window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',cleanup);window.removeEventListener('pointercancel',cleanup)};window.addEventListener('pointermove',move);window.addEventListener('pointerup',cleanup);window.addEventListener('pointercancel',cleanup)}} onDoubleClick={()=>desktop&&window.xtWindow?.('maximize')}><img src="/mascot.png" alt=""/><strong>小涛</strong><span>你的 AI 工作空间</span></div>
   <div className="title-right">
    <WindowMenu/>
    {desktop&&<div className="window-buttons">{[['minimize','−','最小化'],['maximize','□','最大化或还原'],['close','×','收起到托盘']].map(([action,label,title])=><button key={action} aria-label={title} title={title} onClick={()=>window.xtWindow?.(action)}>{label}</button>)}</div>}
   </div>
  </header>
  <main aria-label={page} className={page==='agent'?'agent-main':undefined}>
   <div className="page-view" style={{display: page === 'agent' ? undefined : 'none'}}>
    <AgentPage visible={page === 'agent'} />
   </div>
  {page !== 'agent' && <div className="workspace-page-layout">
    <div key={page} className="page-view workspace-page-view">
     <div className="workspace-page-content">
      {page === 'office' && <OfficePage visible={page === 'office'}/>}
      {page === 'dashboard' && <DashboardDataProvider enabled><DashboardPage /></DashboardDataProvider>}
      {page === 'accounts' && <AccountsPage />}
     </div>
    </div>
   </div>}
  </main>
  {page === 'agent' && <WorkspaceDock page={page} onSelect={select}/>}
  </div>
  <ToastContainer />
 </div>
}
createRoot(document.getElementById('root')!).render(<ConfirmProvider><App/></ConfirmProvider>);
