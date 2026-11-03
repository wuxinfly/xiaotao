import './AgentDroplet.css';
export type DropletState = 'idle' | 'waiting' | 'thinking' | 'tool' | 'reading' | 'writing' | 'completed' | 'error';
const symbols: Record<string, string> = {
error:'<rect fill="currentColor" x="8" y="1" width="8" height="14" rx="3"/><circle fill="currentColor" cx="12" cy="21" r="3"/>' , 
completed:'<path fill="currentColor" d="M2 10h4v12H2zM8 10l5-7c.5-.7 1.5-.7 2-.1.8 1 .5 2.6.1 3.8L14 9h6a2 2 0 0 1 2 2.4l-1.6 8A3 3 0 0 1 17.5 22H8Z"/>',
asking:'<g class="agent-symbol-stroke"><path d="M5 3h14a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-7l-5 3v-3H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z"/><path d="M9 8a3 3 0 0 1 6 0c0 2-3 2-3 4"/><path d="M12 15h.01"/></g>', 
waiting:'<path class="agent-symbol-stroke" d="M5 3.5h14a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H10L5 22v-3.5a2 2 0 0 1-2-2v-11a2 2 0 0 1 2-2Z"/><g fill="currentColor"><circle class="waiting-dot" cx="7" cy="11" r="1.5"/><circle class="waiting-dot" cx="12" cy="11" r="1.5"/><circle class="waiting-dot" cx="17" cy="11" r="1.5"/></g>',
tool:'<g class="tool-gear agent-symbol-stroke"><path d="m9 2 6 0 .6 3 2 .9 2.7-1 3 5.2-2.1 2 .1 2.2 2 2-3 5.2-2.8-1-1.9 1-.6 2.5H9l-.6-2.5-2-1-2.7 1-3-5.2 2.1-2v-2.2l-2.1-2 3-5.2 2.7 1 2-.9Z" transform="translate(1 0) scale(.9)"/><circle cx="12" cy="12" r="3.5"/></g>',
reading:'<g class="agent-symbol-stroke"><path d="M5 2h10l5 5v15H5Z M15 2v6h5"/><path d="M8 11h8M8 15h8M8 19h5" opacity=".35"/><path class="read-scan" d="M2 12h20" stroke-width="3"/></g>',
writing:'<g class="agent-symbol-stroke"><path d="M3 21h8" class="write-line"/><g class="write-pen"><path d="m5 17 2-6L17 1l5 5-10 10-7 1Z M14 4l5 5 M7 11l5 5"/></g></g>',
output:'<g class="agent-symbol-stroke"><path d="M10 3H3v18h7"/><g class="output-arrow"><path d="M8 12h13m-6-6 6 6-6 6" stroke-width="2.8"/></g></g>'
};
export function activityState(kind?: string): DropletState {
  if (kind === 'read_file' || kind === 'list_files' || kind === 'search_files') return 'reading';
  if (kind === 'write_file') return 'writing';
  return 'tool';
}
export function AgentDroplet({running = false, state = running ? 'thinking' : 'idle'}: {running?: boolean; state?: DropletState}) {
  return <span className="agent-droplet" data-state={state} aria-hidden="true">
    <span className="agent-droplet__wave"/><span className="agent-droplet__face"/>
    <span className="agent-droplet__dot agent-droplet__dot--one"/>
    <span className="agent-droplet__dot agent-droplet__dot--two"/>
    <span className="agent-droplet__dot agent-droplet__dot--three"/>
    {symbols[state] && <svg viewBox="0 0 24 24" className="agent-droplet__symbol" dangerouslySetInnerHTML={{__html: symbols[state]}}/>}
  </span>;
}
