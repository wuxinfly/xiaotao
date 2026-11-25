import type {CSSProperties} from 'react';
import {UserRound} from 'lucide-react';
import './office-member-avatar.css';

export function OfficeMemberAvatar({name, color}: {name: string; color?: string}) {
  const initial = Array.from(name.trim())[0];
  return <span className="office-member-avatar" title={name} aria-hidden="true"
    style={color ? {'--member-avatar-color': color} as CSSProperties : undefined}>
    {initial || <UserRound/>}
  </span>;
}
