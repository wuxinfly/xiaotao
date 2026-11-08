import {ArrowLeft, ChartNoAxesColumn, UserRoundCog} from 'lucide-react';
import './workspace-sidebar.css';

export type Page = 'agent' | 'dashboard' | 'accounts';
type Props = {page: Page; onSelect: (page: Page) => void};

const items: {page: Page; label: string; Icon: typeof ChartNoAxesColumn}[] = [
  {page: 'dashboard', label: '仪表盘', Icon: ChartNoAxesColumn},
  {page: 'accounts', label: '账号与配额', Icon: UserRoundCog},
];

function WorkspaceNav({page, onSelect, ...props}: Props & {'aria-label'?: string}) {
  return <nav className="workspace-nav" {...props}>
    {items.map(({page: target, label, Icon}) => <button type="button" key={target}
      className={page === target ? 'is-active' : undefined}
      aria-current={page === target ? 'page' : undefined}
      title={label} onClick={() => onSelect(target)}>
      <Icon aria-hidden="true"/><span>{label}</span>
    </button>)}
  </nav>;
}

export function WorkspaceDock({page, onSelect}: Props) {
  return <div className="workspace-dock">
    <WorkspaceNav page={page} onSelect={onSelect} aria-label="工作空间导航"/>
  </div>;
}

export function WorkspaceBack({onBack}: {onBack: () => void}) {
  return <button type="button" className="titlebar-back" onClick={onBack}>
    <ArrowLeft aria-hidden="true"/>
    <span>返回小涛</span>
  </button>;
}
