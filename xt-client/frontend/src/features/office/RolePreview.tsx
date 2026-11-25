import type {CSSProperties} from 'react';
import {defaultAppearance, type Appearance} from '../../lib/team';

export function RoleMark({name, appearance = defaultAppearance}: {name: string; appearance?: Appearance}) {
  const colors = {'--role-primary': appearance.primaryColor, '--role-secondary': appearance.secondaryColor} as CSSProperties;
  return <span className="office-role-mark" data-shape={appearance.shape} style={colors} aria-hidden="true">
    {Array.from(name.trim())[0] || '·'}
  </span>;
}

export function RolePreview({appearance, name}: {appearance: Appearance; name: string}) {
  return <div className="role-preview" role="img" aria-label={`${name}外观预览`}>
    <RoleMark appearance={appearance} name={name}/>
  </div>;
}
