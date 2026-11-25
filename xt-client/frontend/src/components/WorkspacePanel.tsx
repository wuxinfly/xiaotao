import type {ReactNode} from 'react';
import {PanelRightClose, PanelRightOpen, X} from 'lucide-react';
import './workspace-panel.css';

const panelId = 'agent-workspace-panel';

export function WorkspacePanelToggle({open, onToggle}: {open: boolean; onToggle: () => void}) {
  const Icon = open ? PanelRightClose : PanelRightOpen;
  return <button type="button" className="agent-context-btn workspace-panel-toggle" aria-label="工作区"
    aria-expanded={open} aria-controls={panelId} title={open ? '收起工作区' : '展开工作区'} onClick={onToggle}>
    <Icon aria-hidden="true"/>
  </button>;
}

export function WorkspacePanel({open, onClose, tabs, children}: {
  open: boolean;
  onClose: () => void;
  tabs: ReactNode;
  children: ReactNode;
}) {
  return <aside id={panelId} className="workspace-panel" aria-label="工作区" hidden={!open}>
    <div className="workspace-panel-header">
      <div className="workspace-panel-tabs" role="group" aria-label="工作区标签">{tabs}</div>
      <button type="button" className="workspace-panel-close" aria-label="收起工作区" onClick={onClose}><X aria-hidden="true"/></button>
    </div>
    {children}
  </aside>;
}
