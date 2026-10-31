import {useEffect, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import './style.css';

declare global {interface Window {xtWindow?: (action:string)=>Promise<void>}}
type Page='agent'|'dashboard'|'accounts';
function App(){
 const [page,setPage]=useState<Page>('agent'),[open,setOpen]=useState(false);
 const menu=useRef<HTMLDivElement>(null),trigger=useRef<HTMLButtonElement>(null);
 const desktop=typeof window.xtWindow==='function';
 useEffect(()=>{const dismiss=(e:PointerEvent)=>{if(!menu.current?.contains(e.target as Node))setOpen(false)};document.addEventListener('pointerdown',dismiss);return()=>document.removeEventListener('pointerdown',dismiss)},[]);
 const select=(p:Page)=>{setPage(p);setOpen(false)};
 return <div className="app">
  {desktop&&['n','s','w','e','nw','ne','sw','se'].map(edge=><div key={edge} aria-hidden="true" className={'resize-edge resize-'+edge} onPointerDown={e=>{if(e.button===0){e.preventDefault();window.xtWindow?.('resize-'+edge)}}}/>)}
  <header className="titlebar">
   <div className="brand drag" onPointerDown={e=>{if(!desktop||e.button!==0)return;const x=e.screenX,y=e.screenY;const move=(event:PointerEvent)=>{if(Math.hypot(event.screenX-x,event.screenY-y)>5){cleanup();window.xtWindow?.('drag')}};const cleanup=()=>{window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',cleanup);window.removeEventListener('pointercancel',cleanup)};window.addEventListener('pointermove',move);window.addEventListener('pointerup',cleanup);window.addEventListener('pointercancel',cleanup)}} onDoubleClick={()=>desktop&&window.xtWindow?.('maximize')}><img src="/mascot.png" alt=""/><strong>小涛</strong><span>你的 AI 工作空间</span></div>
   <nav aria-label="主导航"><button className={page==='agent'?'active':''} onClick={()=>select('agent')}>小涛 Agent</button>
    <div className="model-menu" ref={menu} onMouseEnter={()=>setOpen(true)} onMouseLeave={()=>setOpen(false)} onBlur={e=>{if(!e.currentTarget.contains(e.relatedTarget as Node))setOpen(false)}} onKeyDown={e=>{if(e.key==='Escape'){setOpen(false);trigger.current?.focus()}if(e.key==='ArrowDown'){e.preventDefault();setOpen(true);requestAnimationFrame(()=>menu.current?.querySelector<HTMLButtonElement>('.dropdown button')?.focus())}}}>
     <button ref={trigger} className={page!=='agent'?'active':''} aria-expanded={open} aria-controls="model-options" onClick={()=>setOpen(!open)}>模型 <span className="chevron">⌄</span></button>
     <div className={"dropdown"+(open?" is-open":"")} id="model-options" inert={!open} aria-hidden={!open}><small>模型工作空间</small><button onClick={()=>select('dashboard')}><span>仪表盘</span><small>运行状态与实时用量</small></button><button onClick={()=>select('accounts')}><span>账号与配额</span><small>管理渠道、账号与 API</small></button></div>
    </div>
   </nav>
   <div className="title-right">{desktop&&<div className="window-buttons">{[['minimize','−','最小化'],['maximize','□','最大化或还原'],['close','×','收起到托盘']].map(([action,label,title])=><button key={action} aria-label={title} title={title} onClick={()=>window.xtWindow?.(action)}>{label}</button>)}</div>}</div>
  </header>
  <main aria-label={page} />
 </div>
}
createRoot(document.getElementById('root')!).render(<App/>);
