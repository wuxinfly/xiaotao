import {Archive, ArrowRight, RefreshCw, TriangleAlert, UsersRound} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import type {AgentModelOption, AgentProject} from '../../lib/api';
import type {OfficeMember} from '../../lib/office';
import {useProjectTeam} from './useProjectTeam';
import {TeamOnboarding} from './TeamOnboarding';
import {TeamList} from './TeamList';
import {OfficeRoom} from './OfficeRoom';
import {forwardRef, useImperativeHandle, useState} from 'react';
import type {TeamSnapshot} from '../../lib/team';

type Props = {projectPath: string; project?: AgentProject; visible: boolean; members: OfficeMember[]; models: AgentModelOption[]; artifactCount: number; onSelectMember: (id: string) => void; onOpenLibrary: () => void; onManage: () => void; onTeamChanged: () => void; onShowTeam: () => void};
export type OfficeEntryHandle = {setTeam: (team: TeamSnapshot) => void};

export const OfficeEntryFlow = forwardRef<OfficeEntryHandle, Props>(function OfficeEntryFlow({projectPath, project, visible, members, models, artifactCount, onSelectMember, onOpenLibrary, onManage, onTeamChanged, onShowTeam}, ref) {
  const entry = useProjectTeam(projectPath);
  const [scene, setScene] = useState<'preparing' | 'ready' | 'error'>('preparing');
  const [sceneError, setSceneError] = useState('');
  const [onboardingOpen, setOnboardingOpen] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const showSetup = entry.team?.onboarding === 'pending' && !entry.team.members.length;
  useImperativeHandle(ref, () => ({setTeam: entry.setTeam}));
  function finishSetup(team: TeamSnapshot) {
    entry.setTeam(team);
    setOnboardingOpen(false);
    onTeamChanged();
  }
  const setup = showSetup && <button type="button" className="room-setup" onClick={() => setOnboardingOpen(true)}><UsersRound size={15}/>组建小队<ArrowRight size={14}/></button>;
  const teamError = entry.error && <div className="room-team-error" role="alert">无法读取组队设置 <button type="button" onClick={entry.retry} title={entry.error}>重试</button></div>;
  return <div className="office-entry-flow" data-entry-phase={scene} data-project-path={projectPath}>
    {scene !== 'error' ? <OfficeRoom key={attempt} members={members} artifactCount={artifactCount}
      active={visible && !onboardingOpen} ready={scene === 'ready'} onSelectMember={onSelectMember} onOpenLibrary={onOpenLibrary}
      onShowTeam={onShowTeam} setup={setup || teamError} onReady={() => setScene('ready')} onError={message => {setSceneError(message); setScene('error');}}/>
    : <div className="office-fallback-wrap">
      <div className="office-scene-error" role="alert"><TriangleAlert size={20}/><div><strong>办公室场景暂时未能准备完成</strong><p>{sceneError}</p></div><button type="button" className="office-ghost" onClick={() => {setAttempt(value => value + 1); setScene('preparing');}}><RefreshCw size={15}/>重新加载</button></div>
      <TeamList members={members} onSelect={onSelectMember} onManage={onManage}/>
      <div className="room-fallback-actions">
        <button type="button" className="room-tool" onClick={onOpenLibrary}><Archive size={16}/>项目资料馆</button>
        {setup}{teamError}
      </div>
    </div>}
    {onboardingOpen && entry.team && <ModalFrame title="组建项目小队" className="office-onboarding-modal" onClose={() => setOnboardingOpen(false)} footer={null}>
      <TeamOnboarding projectPath={projectPath} projectName={project?.name || ''} team={entry.team} models={models} onCreated={finishSetup} onSkip={finishSetup}/>
    </ModalFrame>}
  </div>;
});
