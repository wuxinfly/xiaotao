import {useEffect, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import './style.css';
import {AccountsPage} from './features/accounts/AccountsPage';
import {DashboardPage} from './features/dashboard/DashboardPage';
import {ToastContainer} from './components/toast';
import {WindowMenu} from './components/WindowMenu';
import {AgentPage} from './features/agent/AgentPage';
import './navigation.css';

declare global {interface Window {xtWindow?: (action:string)=>Promise<void>}}
type Page='agent'|'dashboard'|'accounts';

// Ctrl + wheel otherwise triggers the browser's page zoom, including in WebView2.
window.addEventListener('wheel', event => {
 if(event.ctrlKey) event.preventDefault();
}, {passive:false});

function App(){
 const [page,setPage]=useState<Page>('dashboard'),[open,setOpen]=useState(false);
 const menu=useRef<HTMLDivElement>(null),trigger=useRef<HTMLButtonElement>(null);
 const closeTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const cancelClose=()=>clearTimeout(closeTimer.current);
 useEffect(()=>()=>clearTimeout(closeTimer.current),[]);
 const desktop=typeof window.xtWindow==='function';
 useEffect(()=>{const dismiss=(e:PointerEvent)=>{if(!menu.current?.contains(e.target as Node))setOpen(false)};document.addEventListener('pointerdown',dismiss);return()=>document.removeEventListener('pointerdown',dismiss)},[]);
 useEffect(()=>{
  const timer=setTimeout(()=>{window.xtWindow?.('ready')},150);
  return()=>clearTimeout(timer);
 },[]);
 const select=(p:Page)=>{setPage(p);setOpen(false)};
 return <div className="app">
  {desktop&&['n','s','w','e','nw','ne','sw','se'].map(edge=><div key={edge} aria-hidden="true" className={'resize-edge resize-'+edge} onPointerDown={e=>{if(e.button===0){e.preventDefault();window.xtWindow?.('resize-'+edge)}}}/>)}
  <div className="workspace-shell">
  <header className="titlebar">
   <div className="brand drag" onPointerDown={e=>{if(!desktop||e.button!==0)return;const x=e.screenX,y=e.screenY;const move=(event:PointerEvent)=>{if(Math.hypot(event.screenX-x,event.screenY-y)>5){cleanup();window.xtWindow?.('drag')}};const cleanup=()=>{window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',cleanup);window.removeEventListener('pointercancel',cleanup)};window.addEventListener('pointermove',move);window.addEventListener('pointerup',cleanup);window.addEventListener('pointercancel',cleanup)}} onDoubleClick={()=>desktop&&window.xtWindow?.('maximize')}><img src="/mascot.png" alt=""/><strong>小涛</strong><span>你的 AI 工作空间</span></div>
   <nav aria-label="主导航" data-active={page==='agent'?'agent':'model'}><span className="nav-indicator" aria-hidden="true"/><button className={page==='agent'?'active':''} aria-current={page==='agent'?'page':undefined} onClick={()=>select('agent')}>小涛 Agent</button>
    <div className="model-menu" ref={menu} onMouseEnter={()=>{cancelClose();setOpen(true)}} onMouseLeave={()=>{cancelClose();closeTimer.current=setTimeout(()=>{if(!menu.current?.contains(document.activeElement))setOpen(false)},180)}} onBlur={e=>{if(!e.currentTarget.contains(e.relatedTarget as Node))setOpen(false)}} onKeyDown={e=>{if(e.key==='Escape'){cancelClose();setOpen(false);trigger.current?.focus()}if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();cancelClose();setOpen(true);const direction=e.key==='ArrowDown'?1:-1;requestAnimationFrame(()=>{const items=Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('.dropdown button')??[]);const index=items.indexOf(document.activeElement as HTMLButtonElement);items[index<0?(direction>0?0:items.length-1):(index+direction+items.length)%items.length]?.focus()})}}}>
     <button ref={trigger} className={page!=='agent'?'active':''} aria-expanded={open} aria-controls="model-options" onClick={()=>{cancelClose();setOpen(!open)}}><img className="nav-icon" src="/model-layers.svg" alt=""/>模型 <span className="chevron" aria-hidden="true"/></button>
     <div className={"dropdown"+(open?" is-open":"")} id="model-options" inert={!open} aria-hidden={!open}><small>模型工作空间</small><button className={(page==='dashboard'?'selected ':'')+'dashboard-option'} aria-current={page==='dashboard'?'page':undefined} onClick={()=>select('dashboard')}><img className="dashboard-icon" src="/dashboard.svg" alt=""/><span>仪表盘</span><small>运行状态与实时用量</small></button><button className={(page==='accounts'?'selected ':'')+'quota-option'} aria-current={page==='accounts'?'page':undefined} onClick={()=>select('accounts')}><img className="dropdown-icon" src="/account-quota-menu.svg" alt=""/><span>账号与配额</span><small>管理渠道、账号与 API</small></button></div>
    </div>
   </nav>
   <div className="title-right">{desktop&&<><WindowMenu/><div className="window-buttons">{[['minimize','−','最小化'],['maximize','□','最大化或还原'],['close','×','收起到托盘']].map(([action,label,title])=><button key={action} aria-label={title} title={title} onClick={()=>window.xtWindow?.(action)}>{label}</button>)}</div></>}</div>
  </header>
  <main aria-label={page} className={page==='agent'?'agent-main':undefined}>
   <div key={page} className="page-view">
    {page === 'agent' && <AgentPage />}
    {page === 'dashboard' && <DashboardPage />}
    {page === 'accounts' && <AccountsPage />}
   </div>
  </main>
  </div>
  <ToastContainer />
 </div>
}
createRoot(document.getElementById('root')!).render(<App/>);
