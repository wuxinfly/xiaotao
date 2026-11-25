import {useRef, useState} from 'react';
import {FolderOpen, Globe2, Plus} from 'lucide-react';
import {Popover} from '../../components/popover/Popover';
import type {WorkspaceTool} from './useWorkspaceTools';
import './workspace-tools.css';

const options = [
  {tool: 'files' as const, label: '文件管理', hint: '浏览和预览项目文件', Icon: FolderOpen},
  {tool: 'browser' as const, label: '浏览器', hint: '输入网址直接访问', Icon: Globe2},
];

export function WorkspaceAddTab({onPick}: {onPick: (tool: WorkspaceTool) => void}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const pick = (tool: WorkspaceTool) => {
    setOpen(false);
    onPick(tool);
  };
  return <>
    <button ref={trigger} type="button" className="workspace-panel-add" aria-label="打开文件管理或浏览器"
      aria-haspopup="menu" aria-expanded={open} title="打开文件管理或浏览器" onClick={() => setOpen(current => !current)}>
      <Plus aria-hidden="true"/>
    </button>
    <Popover open={open} onClose={() => setOpen(false)} anchorRef={trigger} className="workspace-add-menu" aria-label="打开工具">
      {options.map(({tool, label, hint, Icon}) => <button key={tool} type="button" role="menuitem" onClick={() => pick(tool)}>
        <Icon aria-hidden="true"/>
        <span><strong>{label}</strong><small>{hint}</small></span>
      </button>)}
    </Popover>
  </>;
}
