import {useEffect, useRef, useState} from 'react';
import type {SceneMember} from './scene-contract';
import type {OfficeAssets} from './scene-models';
import type {SceneRuntime} from './runtime-contract';

export function useOfficeRuntime(active: boolean, members: SceneMember[], load: () => Promise<OfficeAssets>, create: (element: HTMLElement, assets: OfficeAssets) => SceneRuntime, onReady: () => void, onError: (message: string) => void) {
  const host = useRef<HTMLDivElement>(null);
  const runtime = useRef<SceneRuntime | null>(null);
  const generation = useRef(0);
  const [ready, setReady] = useState(false);
  const callbacks = useRef({onReady, onError}); callbacks.current = {onReady, onError};
  const initial = useRef({members, active, create, load});
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    const id = ++generation.current;
    let disposed = false;
    setReady(false);
    initial.current.load().then(assets => {
      if (disposed || generation.current !== id) return;
      try {runtime.current = initial.current.create(element, assets); runtime.current.update(initial.current.members); runtime.current.setActive(initial.current.active); runtime.current.render(); setReady(true); callbacks.current.onReady();}
      catch (reason) {callbacks.current.onError(reason instanceof Error ? reason.message : String(reason));}
    }).catch(reason => {if (!disposed && generation.current === id) callbacks.current.onError(reason instanceof Error ? reason.message : String(reason));});
    return () => {disposed = true; if (generation.current === id) generation.current = id + 1; runtime.current?.destroy(); runtime.current = null;};
  }, []);
  useEffect(() => {runtime.current?.update(members);}, [members]);
  useEffect(() => {runtime.current?.setActive(active); if (active) runtime.current?.render();}, [active]);
  return {host, runtime, ready};
}
