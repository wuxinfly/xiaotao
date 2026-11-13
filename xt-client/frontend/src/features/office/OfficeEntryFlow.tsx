import {RefreshCw, TriangleAlert} from 'lucide-react';
import type {AgentModelOption, AgentProject} from '../../lib/api';
import type {OfficeMember} from '../../lib/office';
import {useProjectTeam} from './useProjectTeam';
import {TeamOnboarding} from './TeamOnboarding';
import {TeamList} from './TeamList';
import {OfficeRoom} from './OfficeRoom';
import {forwardRef, useImperativeHandle, useState} from 'react';
import type {TeamSnapshot} from '../../lib/team';

type Props = {projectPath: string; project?: AgentProject; visible: boolean; members: OfficeMember[]; models: AgentModelOption[]; artifactCount: number; onSelectMember: (id: string) => void; onOpenLibrary: () => void; onManage: () => void; children?: React.ReactNode};
export type OfficeEntryHandle = {setTeam: (team: TeamSnapshot) => void};

export const OfficeEntryFlow = forwardRef<OfficeEntryHandle, Props>(function OfficeEntryFlow({projectPath, project, visible, members, models, artifactCount, onSelectMember, onOpenLibrary, onManage, children}, ref) {
  const entry = useProjectTeam(projectPath, visible);
  const [attempt, setAttempt] = useState(0);
  useImperativeHandle(ref, () => ({setTeam: entry.setTeam}));
  const showTeam = entry.phase === 'ready' || entry.phase === 'empty' || entry.phase === 'error';
  return <div className="office-entry-flow" data-entry-phase={entry.phase} data-project-path={projectPath}>
    {entry.phase === 'loading' && <section className="office-loading" role="status"><span className="office-loading-mark"/><span className="office-eyebrow">正在打开 {project?.name || '项目办公室'}</span><h1>准备你的工作空间</h1><p>正在读取项目团队与引导设置…</p></section>}
    {entry.phase === 'onboarding' && entry.team && <TeamOnboarding key={projectPath} projectPath={projectPath} projectName={project?.name || ''} team={entry.team} models={models} onCreated={entry.setTeam} onSkip={team => {entry.setTeam(team); entry.setPhase('empty');}}/>}
    {entry.phase === 'preparing' && <section className="office-loading" role="status"><span className="office-loading-mark"/><span className="office-eyebrow">团队已就绪</span><h1>正在准备办公室</h1><p>正在加载本地模型并绘制第一帧，不使用模拟进度。</p></section>}
    {(entry.phase === 'ready' || entry.phase === 'preparing' || entry.phase === 'error') && entry.team && <OfficeRoom key={attempt} projectName={project?.name || ''} members={members} artifactCount={artifactCount} active={visible && entry.phase === 'ready'} onSelectMember={onSelectMember} onOpenLibrary={onOpenLibrary} onReady={() => entry.setPhase('ready')} onError={message => entry.setPhase('error', message)}/>} 
    {(entry.phase === 'error' || entry.phase === 'empty' || entry.phase === 'preparing') && <div className="office-fallback-wrap">
      {entry.phase === 'error' && <div className="office-scene-error" role="alert"><TriangleAlert size={22}/><div><strong>{entry.team ? '办公室场景暂时未能准备完成' : '暂时无法读取项目团队'}</strong><p>{entry.error || '请重试。已保存的团队不会丢失，核心操作仍可继续使用。'}</p></div><button className="office-ghost" onClick={() => {setAttempt(value => value + 1); void entry.prepare();}}><RefreshCw size={15}/>重新加载</button></div>}
      {entry.phase === 'preparing' && <p className="office-loading-note">已保存的团队仍可在下方管理。</p>}
      <TeamList members={members} onSelect={onSelectMember} onManage={onManage}/>
    </div>}
    {children}
    {showTeam && <button className="office-team-manage-shortcut" onClick={onManage}>团队管理</button>}
  </div>;
});
