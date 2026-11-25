import {useCallback, useEffect, useRef, useState} from 'react';

export type WorkspaceTool = 'files' | 'browser';

/** Tool tabs share the workspace strip with the office, but keep their own order. */
export function useWorkspaceTools(project: string, selectedTab: string, openVersion: number) {
  const [tabs, setTabs] = useState<WorkspaceTool[]>([]);
  const [active, setActive] = useState<WorkspaceTool | null>(null);
  const lastSelected = useRef(selectedTab);
  const lastOpen = useRef(openVersion);

  useEffect(() => {
    setTabs([]);
    setActive(null);
  }, [project]);

  // Opening an employee from a board card changes the office tab; make sure the
  // tool view yields instead of covering the conversation that was requested.
  useEffect(() => {
    if (lastSelected.current === selectedTab) return;
    lastSelected.current = selectedTab;
    setActive(null);
  }, [selectedTab]);

  // Dispatch cards can request the same employee that is already selected;
  // the office hook still bumps its open request so the tool view yields.
  useEffect(() => {
    if (lastOpen.current === openVersion) return;
    lastOpen.current = openVersion;
    setActive(null);
  }, [openVersion]);

  const open = useCallback((tool: WorkspaceTool) => {
    setTabs(current => current.includes(tool) ? current : [...current, tool]);
    setActive(tool);
  }, []);
  const close = useCallback((tool: WorkspaceTool) => {
    setTabs(current => current.filter(item => item !== tool));
    setActive(current => current === tool ? null : current);
  }, []);
  const clear = useCallback(() => setActive(null), []);

  return {tabs, active, open, close, select: setActive, clear};
}
