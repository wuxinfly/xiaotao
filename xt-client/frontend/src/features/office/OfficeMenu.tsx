import {ClipboardList, RefreshCw} from 'lucide-react';
import {createPortal} from 'react-dom';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Select} from '../../components/select/Select';
import type {AgentProject} from '../../lib/api';
import {isTerminalTask, type OfficeSnapshot} from '../../lib/office';
import {TaskCard} from './OfficeRoom';

export function OfficeMenuButton({count, onOpen}: {count: number; onOpen: () => void}) {
  return createPortal(<button className="office-fab" type="button" aria-haspopup="dialog" aria-label="办公室菜单"
    aria-description={count > 0 ? `${count} 个进行中的任务` : undefined} onClick={onOpen}>
    <ClipboardList size={19} aria-hidden="true"/><span>办公室菜单</span>
    {count > 0 && <span className="office-fab-count" aria-hidden="true">{count}</span>}
  </button>, document.body);
}

type Props = {
  projects: AgentProject[];
  selectedPath: string;
  snapshot: OfficeSnapshot | null;
  lastUpdated: number;
  loading: boolean;
  onChange: (path: string) => void;
  onRefresh: () => void;
  onTask: (id: string) => void;
  onClose: () => void;
  onTeam: () => void;
};

export function OfficeMenu({projects, selectedPath, snapshot, lastUpdated, loading, onChange, onRefresh, onTask, onClose, onTeam}: Props) {
  const tasks = snapshot?.tasks ?? [];
  const running = tasks.filter(task => !isTerminalTask(task.status)).length;
  return <ModalFrame title="办公室菜单" className="office-menu-modal" onClose={onClose} footer={snapshot && <p className="office-updated">
    {lastUpdated > 0 && `上次更新 ${new Date(lastUpdated).toLocaleTimeString('zh-CN')} · `}
    {snapshot.accessMode === 'full' ? '完全访问模式，高危操作不再逐条确认' : '安全模式，高危操作需要你确认'}
  </p>}>
    <div className="office-page-actions">
      <label className="office-project-picker"><span>项目</span>
        <Select value={selectedPath} onChange={onChange} options={projects.map(project => ({value: project.path, label: project.name}))}/>
      </label>
      <button type="button" className="office-ghost" onClick={onRefresh} disabled={!selectedPath || loading}>
        <RefreshCw size={14} aria-hidden="true"/>{loading ? '刷新中…' : '刷新'}
      </button>
      <button type="button" className="office-primary" onClick={onTeam} disabled={!selectedPath}>管理团队</button>
    </div>
    <section className="office-board" aria-label="任务">
      <header><h3>任务</h3><span>{running} 个进行中 / 共 {tasks.length} 个</span></header>
      {tasks.length ? <div className="office-task-list" role="list">
        {tasks.map(task => <div role="listitem" key={task.id}><TaskCard task={task} onSelect={() => onTask(task.id)}/></div>)}
      </div> : <p className="office-muted">{loading ? '读取任务…' : '这个项目还没有任务。'}</p>}
    </section>
    {!!snapshot?.capabilities.notes.length && <details className="office-caveats">
      <summary>运行说明</summary><ul>{snapshot.capabilities.notes.map(note => <li key={note}>{note}</li>)}</ul>
    </details>}
  </ModalFrame>;
}
