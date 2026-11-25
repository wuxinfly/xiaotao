import {ArrowLeft, ChartNoAxesColumn, DoorOpen, UserRoundCog} from 'lucide-react';
import './workspace-sidebar.css';

export type Page = 'agent' | 'office' | 'dashboard' | 'accounts';
type Props = {page: Page; officeOpen?: boolean; onSelect: (page: Page) => void};

const items: {page: Page; label: string; Icon: typeof ChartNoAxesColumn}[] = [
  {page: 'office', label: '办公室', Icon: DoorOpen},
  {page: 'dashboard', label: '仪表盘', Icon: ChartNoAxesColumn},
  {page: 'accounts', label: '模型接口', Icon: UserRoundCog},
];

function WorkspaceNav({page, officeOpen, onSelect, ...props}: Props & {'aria-label'?: string}) {
  return <nav className="workspace-nav" {...props}>
    {items.map(({page: target, label, Icon}) => <button type="button" key={target}
      className={page === target || target === 'office' && officeOpen ? 'is-active' : undefined}
      aria-pressed={target === 'office' ? officeOpen : undefined}
      aria-current={page === target ? 'page' : undefined}
      aria-label={label} title={label} onClick={() => onSelect(target)}>
      <Icon aria-hidden="true"/><span>{label}</span>
    </button>)}
  </nav>;
}

export function WorkspaceDock({page, officeOpen, onSelect}: Props) {
  return <div className="workspace-dock">
    <WorkspaceNav page={page} officeOpen={officeOpen} onSelect={onSelect} aria-label="工作空间导航"/>
  </div>;
}

export function WorkspaceBack({onBack}: {onBack: () => void}) {
  return <button type="button" className="titlebar-back" onClick={onBack}>
    <ArrowLeft aria-hidden="true"/>
    <span>返回小涛</span>
  </button>;
}
