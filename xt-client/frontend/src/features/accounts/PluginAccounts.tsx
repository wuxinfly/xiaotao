import {useState, type ReactNode} from 'react';
import {CalendarCheck, CalendarClock, GripVertical, Plus, RefreshCw, UserRound} from 'lucide-react';
import {DisplayLine, QuotaLine, readQuotaItems, type PluginDisplay} from './PluginQuota';
import './plugin-accounts.css';

export type LegacyAccount = {id: string; name: string; uid: string};
export type PluginAccount = {id: string; identity?: string; name: string};

type Props = {
  accounts: PluginAccount[]; busy: boolean; authorizing: boolean; canAuthorize: boolean; canQuota: boolean; canCheckin: boolean; legacy: LegacyAccount[];
  quota: (id: string) => unknown; checkin: (id: string) => ReactNode;
  display: (id: string) => PluginDisplay | undefined; checkinDisplay: (id: string) => PluginDisplay | undefined;
  onImport: (id: string) => void; onAdd: () => void; onCancel: () => void; onOrder: (ids: string[]) => void;
  onAction: (id: string, method: string, feedback: string) => void;
};

function moveAccount(accounts: PluginAccount[], id: string, index: number): string[] | null {
  const ids = accounts.map(account => account.id);
  const from = ids.indexOf(id);
  if (from < 0 || index < 0 || index >= ids.length || from === index) return null;
  ids.splice(from, 1);
  ids.splice(index, 0, id);
  return ids;
}

export function PluginAccounts({accounts, busy, authorizing, canAuthorize, canQuota, canCheckin, legacy, quota, checkin, display, checkinDisplay, onImport, onAdd, onCancel, onOrder, onAction}: Props) {
  const [dragging, setDragging] = useState('');
  const [target, setTarget] = useState('');
  const [expanded, setExpanded] = useState('');
  const pending = legacy.filter(account => !account.uid || !accounts.some(active => active.identity === account.uid));

  function move(id: string, index: number) {
    if (busy) return;
    const ids = moveAccount(accounts, id, index);
    if (ids) onOrder(ids);
  }
  return <section className="plugin-accounts" aria-label="插件账号">
    <header className="plugin-section-head">
      <div className="plugin-section-title"><UserRound size={15} aria-hidden="true"/><h4>账号</h4></div>
      <span className="plugin-section-note">首位账号用于聊天和目录刷新</span>
    </header>
    <div className="plugin-account-list">
      {!accounts.length && <p className="plugin-account-empty">还没有授权账号，添加后即可使用。</p>}
      {accounts.map((account, index) => {
        const value = quota(account.id);
        const items = readQuotaItems(value);
        const open = expanded === account.id;
        return <div className={`plugin-account-row${target === account.id ? ' is-drop-target' : ''}`} key={account.id}
          onDragOver={event => {if (dragging && dragging !== account.id) {event.preventDefault(); setTarget(account.id);}}}
          onDrop={event => {event.preventDefault(); move(dragging, index); setDragging(''); setTarget('');}}>
          <button type="button" className="plugin-account-grip" draggable={!busy && accounts.length > 1} title="拖动调整顺序，或聚焦后按方向键调整"
            aria-label={`调整 ${account.name} 的顺序`}
            onDragStart={event => {event.dataTransfer.setData('text/plain', account.id); event.dataTransfer.effectAllowed = 'move'; setDragging(account.id);}}
            onDragEnd={() => {setDragging(''); setTarget('');}}
            onKeyDown={event => {
              if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;
              event.preventDefault();
              move(account.id, index + (event.key === 'ArrowUp' ? -1 : 1));
            }}>
            <GripVertical size={15} aria-hidden="true"/>
          </button>
          <span className={`plugin-avatar tone-${index % 3}`} aria-hidden="true"><UserRound size={16}/></span>
          <div className="plugin-account-main">
            <div className="plugin-account-name">
              <strong>{account.name}</strong>
              {account.identity && account.name !== `用户 ${account.identity}` && <small>{account.identity}</small>}
              {index === 0 && <span className="plugin-pill">首选</span>}
            </div>
            {items.length ? <QuotaLine items={items} expanded={open} onToggle={() => setExpanded(open ? '' : account.id)}/> :
              display(account.id) ? <DisplayLine display={display(account.id)!}/> :
              <span className="plugin-quota-empty">{value === undefined ? '尚未查询配额' : '暂无配额数据'}</span>}
            {checkin(account.id)}
            {checkinDisplay(account.id) && <DisplayLine display={checkinDisplay(account.id)!}/>}
          </div>
          {(index > 0 || canQuota || canCheckin) && <div className="plugin-account-actions">
            {index > 0 && <button type="button" className="plugin-link-button" disabled={busy} onClick={() => move(account.id, 0)}>设为首选</button>}
            {canQuota && <button type="button" className="plugin-icon-button" aria-label="查询配额" title="查询配额" disabled={busy}
              onClick={() => onAction(account.id, 'quota/get', '配额查询完成')}><RefreshCw size={14} aria-hidden="true"/></button>}
            {canCheckin && <>
              <button type="button" className="plugin-icon-button" aria-label="签到状态" title="刷新签到状态" disabled={busy}
                onClick={() => onAction(account.id, 'checkin/status', '签到状态已更新')}><CalendarClock size={14} aria-hidden="true"/></button>
              <button type="button" className="plugin-icon-button is-accent" aria-label="签到" title="签到" disabled={busy}
                onClick={() => onAction(account.id, 'checkin/claim', '签到请求完成')}><CalendarCheck size={14} aria-hidden="true"/></button>
            </>}
          </div>}
        </div>;
      })}
      {pending.length > 0 && canAuthorize && <><div className="plugin-account-group">当前环境的旧账号</div>
        {pending.map(account => <div className="plugin-account-row is-legacy" key={account.id}>
          <span className="plugin-avatar tone-2" aria-hidden="true"><UserRound size={16}/></span>
          <div className="plugin-account-main">
            <div className="plugin-account-name"><strong>{account.name || account.uid || account.id}</strong><small>{account.uid}</small></div>
            <span className="plugin-quota-empty">尚未加入插件</span>
          </div>
          <button type="button" className="app-modal-button secondary" disabled={busy} onClick={() => onImport(account.id)}>加入插件</button>
        </div>)}
      </>}
      {pending.length > 0 && !canAuthorize && <div className="plugin-account-hint">发现 {pending.length} 个当前环境旧账号，更新插件后可一键加入。</div>}
      <div className="plugin-account-add">
        <button type="button" className="plugin-account-add-button" disabled={busy || !canAuthorize} onClick={onAdd}>
          <span className="plugin-plus" aria-hidden="true"><Plus size={15}/></span>{accounts.length ? '添加另一个账号' : '添加账号'}
        </button>
        {authorizing ? <><span role="status">请在浏览器完成授权</span><button type="button" className="plugin-link-button" onClick={onCancel}>取消授权</button></> : !canAuthorize && <span>更新插件后可添加账号</span>}
      </div>
    </div>
  </section>;
}