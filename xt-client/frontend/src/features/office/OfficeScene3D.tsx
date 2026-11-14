import {forwardRef, useImperativeHandle} from 'react';
import {type SceneMember} from './scene-contract';
import {useOfficeRuntime} from './office-3d-runtime';

type Props = {members: SceneMember[]; active: boolean; onSelect: (id: string) => void; onShelf: () => void; onReady: () => void; onError: (message: string) => void};
export type SceneControls = {zoomIn: () => void; zoomOut: () => void; reset: () => void};

export const OfficeScene3D = forwardRef<SceneControls, Props>(function OfficeScene3D({members, active, onSelect, onShelf, onReady, onError}, ref) {
  const {host, runtime} = useOfficeRuntime({active, members, onSelect, onShelf, onReady, onError});
  useImperativeHandle(ref, () => ({zoomIn: () => runtime.current?.zoomBy(.1), zoomOut: () => runtime.current?.zoomBy(-.1), reset: () => runtime.current?.reset()}), [runtime]);
  return <div className="office-3d-canvas" ref={host}/>;
});
