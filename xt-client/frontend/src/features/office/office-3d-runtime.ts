import {useEffect, useRef} from 'react';
import type {SceneMember} from './scene-contract';
import {loadOfficeAssets} from './scene-models';
import {OfficeEngine} from './OfficeEngine';

type Options = {
  active: boolean;
  members: SceneMember[];
  onSelect: (id: string) => void;
  onShelf: () => void;
  onReady: () => void;
  onError: (message: string) => void;
};

export function useOfficeRuntime(options: Options) {
  const host = useRef<HTMLDivElement>(null);
  const runtime = useRef<OfficeEngine | null>(null);
  const latest = useRef(options);
  latest.current = options;
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    const controller = new AbortController();
    let failed = false;
    const onError = (message: string) => {failed = true; latest.current.onError(message);};
    loadOfficeAssets(controller.signal).then(assets => {
      if (controller.signal.aborted) {assets.dispose(); return;}
      try {
        const engine = new OfficeEngine(element, {
          onSelect: id => latest.current.onSelect(id),
          onShelf: () => latest.current.onShelf(),
          onError,
        }, assets);
        runtime.current = engine;
        engine.setActive(latest.current.active);
        engine.update(latest.current.members);
        if (!failed) latest.current.onReady();
      } catch (reason) {
        if (runtime.current) {runtime.current.destroy(); runtime.current = null;}
        else assets.dispose();
        onError(reason instanceof Error ? reason.message : String(reason));
      }
    }).catch(reason => {
      if (!controller.signal.aborted) onError(reason instanceof Error ? reason.message : String(reason));
    });
    return () => {controller.abort(); runtime.current?.destroy(); runtime.current = null;};
  }, []);
  useEffect(() => {runtime.current?.update(options.members);}, [options.members]);
  useEffect(() => {runtime.current?.setActive(options.active);}, [options.active]);
  return {host, runtime};
}
