import {useEffect, useRef, useState} from 'react';
import {toast} from './toast';
import './window-menu.css';

declare global {
 interface Window {xtOpenDirectory?: (kind:'install'|'data')=>Promise<void>}
}

export function WindowMenu(){
 const [open,setOpen]=useState(false);
 const menu=useRef<HTMLDivElement>(null),trigger=useRef<HTMLButtonElement>(null);
 const timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const cancelClose=()=>clearTimeout(timer.current);
 const close=()=>{cancelClose();setOpen(false)};
 useEffect(()=>{
  const dismiss=(event:PointerEvent)=>{if(!menu.current?.contains(event.target as Node))close()};
  document.addEventListener('pointerdown',dismiss);
  const blur=()=>close();
  window.addEventListener('blur',blur);
  return()=>{cancelClose();document.removeEventListener('pointerdown',dismiss);window.removeEventListener('blur',blur)};
 },[]);
 const openDirectory=async(kind:'install'|'data')=>{
  close();
  trigger.current?.focus();
  try {
   if(!window.xtOpenDirectory)throw new Error('目录操作暂不可用，请重新打开客户端');
   await window.xtOpenDirectory(kind);
  }catch(error){toast.error(error instanceof Error?error.message:String(error))}
 };
 return <div className="window-menu" ref={menu}
  onMouseEnter={()=>{cancelClose();setOpen(true)}}
  onMouseLeave={()=>{cancelClose();timer.current=setTimeout(()=>{if(!menu.current?.contains(document.activeElement))setOpen(false)},180)}}
  onBlur={event=>{if(!event.currentTarget.contains(event.relatedTarget as Node))close()}}
  onKeyDown={event=>{
   if(event.key==='Escape'){event.preventDefault();close();trigger.current?.focus()}
   if(event.key==='ArrowDown'||event.key==='ArrowUp'||event.key==='Home'||event.key==='End'){
    event.preventDefault();cancelClose();setOpen(true);
    requestAnimationFrame(()=>{
     const items=Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')??[]);
     const index=items.indexOf(document.activeElement as HTMLButtonElement);
     const next=event.key==='Home'?0:event.key==='End'?items.length-1:event.key==='ArrowDown'?(index+1)%items.length:(index<0?items.length-1:(index-1+items.length)%items.length);
     items[next]?.focus();
    });
   }
  }}>
  <button ref={trigger} className="window-menu-trigger" aria-label="更多" aria-haspopup="menu" aria-expanded={open} aria-controls="window-more-menu" onClick={()=>{cancelClose();setOpen(!open)}}>
   <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true"><circle cx="4" cy="10" r="1.5"/><circle cx="10" cy="10" r="1.5"/><circle cx="16" cy="10" r="1.5"/></svg>
  </button>
  <div id="window-more-menu" role="menu" aria-label="更多" className={'window-menu-panel'+(open?' is-open':'')} inert={!open} aria-hidden={!open}>
   <button role="menuitem" onClick={()=>void openDirectory('install')}>打开安装目录</button>
   <button role="menuitem" onClick={()=>void openDirectory('data')}>打开数据目录</button>
  </div>
 </div>;
}
