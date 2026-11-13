import {forwardRef, useImperativeHandle} from 'react';
import {OfficeEngine} from './OfficeEngine';
import {type SceneMember} from './scene-contract';
import {loadOfficeAssets} from './scene-models';
import {useOfficeRuntime} from './office-3d-runtime';

type Props = {members: SceneMember[]; active: boolean; onSelect: (id: string) => void; onShelf: () => void; onReady: () => void; onError: (message: string) => void};
export type SceneControls = {zoomIn: () => void; zoomOut: () => void; reset: () => void; pauseMember: (id: string) => void};

export const OfficeScene3D = forwardRef<SceneControls, Props>(function OfficeScene3D({members, active, onSelect, onShelf, onReady, onError}, ref) {
  const {host, runtime} = useOfficeRuntime(active, members, () => loadOfficeAssets(), (element, assets) => {
    const engine = new OfficeEngine(element, {onSelect, onShelf, onError}, assets);
    return {update: list => engine.update(list), setActive: enabled => engine.setActive(enabled), pauseMember: id => engine.pauseMember(id), zoomBy: amount => engine.zoomBy(amount), reset: () => engine.reset(), render: () => engine.render(), destroy: () => engine.destroy()};
  }, onReady, onError);
  useImperativeHandle(ref, () => ({zoomIn: () => runtime.current?.zoomBy(.1), zoomOut: () => runtime.current?.zoomBy(-.1), reset: () => runtime.current?.reset(), pauseMember: id => runtime.current?.pauseMember(id)}), [runtime]);
  return <div className="office-3d-canvas" ref={host}/>;
});
