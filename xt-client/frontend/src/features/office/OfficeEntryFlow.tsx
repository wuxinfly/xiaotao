import {ArrowRight, UsersRound} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import type {AgentModelOption} from '../../lib/api';
import {useProjectTeam} from './useProjectTeam';
import {TeamOnboarding} from './TeamOnboarding';
import {OfficeRoom, type OfficeRoomProps} from './OfficeRoom';
import {forwardRef, useImperativeHandle, useState} from 'react';
import type {TeamSnapshot} from '../../lib/team';

type Props = Omit<OfficeRoomProps, 'setup'> & {projectPath: string; projectName: string; models: AgentModelOption[]; onTeamChanged: () => void};
export type OfficeEntryHandle = {setTeam: (team: TeamSnapshot) => void};

export const OfficeEntryFlow = forwardRef<OfficeEntryHandle, Props>(function OfficeEntryFlow({projectPath, projectName, models, onTeamChanged, ...room}, ref) {
  const entry = useProjectTeam(projectPath);
  const [onboardingOpen, setOnboardingOpen] = useState(false);
  const showSetup = entry.team?.onboarding === 'pending' && !entry.team.members.length;
  useImperativeHandle(ref, () => ({setTeam: entry.setTeam}));
  function finishSetup(team: TeamSnapshot) {
    entry.setTeam(team);
    setOnboardingOpen(false);
    onTeamChanged();
  }
  const setup = showSetup && <button type="button" className="room-setup" onClick={() => setOnboardingOpen(true)}><UsersRound size={15}/>组建小队<ArrowRight size={14}/></button>;
  const teamError = entry.error && <div className="room-team-error" role="alert">无法读取组队设置 <button type="button" onClick={entry.retry} title={entry.error}>重试</button></div>;
  return <div className="office-entry-flow" data-project-path={projectPath}>
    <OfficeRoom {...room} setup={<>{setup}{teamError}</>}/>
    {onboardingOpen && entry.team && <ModalFrame title="组建项目小队" className="office-onboarding-modal" onClose={() => setOnboardingOpen(false)} footer={null}>
      <TeamOnboarding projectPath={projectPath} projectName={projectName} team={entry.team} models={models} onCreated={finishSetup} onSkip={finishSetup}/>
    </ModalFrame>}
  </div>;
});
