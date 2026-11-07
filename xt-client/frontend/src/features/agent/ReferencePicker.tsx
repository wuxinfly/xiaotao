import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {LoaderCircle, Search, X} from 'lucide-react';
import type {Editor} from '@tiptap/react';
import {api, type AgentConversationReference, type AgentReferenceCandidate} from '../../lib/api';
import {ReferenceDetailModal} from './AgentModals';

export function ReferencePicker({anchor, editor, anchorAt, query, scope, results, index, loading, error, onScope, onChoose, onClose}: {
  anchor: React.RefObject<HTMLDivElement | null>;
  editor: Editor | null;
  anchorAt: number;
  query: string;
  scope: 'project' | 'all';
  results: AgentReferenceCandidate[];
  index: number;
  loading: boolean;
  error: string;
  onScope: (scope: 'project' | 'all') => void;
  onChoose: (item: AgentReferenceCandidate) => void;
  onClose: () => void;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({left: 0, top: 0, width: 360});
  const [anchorBottom, setAnchorBottom] = useState(0);
  const [maxHeight, setMaxHeight] = useState(320);
  const [shown, setShown] = useState(false);
  const [origin, setOrigin] = useState('0px 100%');
  useLayoutEffect(() => {
    const place = () => {
      const rect = anchor.current?.getBoundingClientRect();
      if (!rect) return;
      const caret = editor?.view.coordsAtPos(Math.min(anchorAt, editor.state.doc.content.size));
      const width = Math.min(420, Math.max(260, rect.width - 24));
      const height = panel.current?.offsetHeight ?? 280;
      const caretTop = caret?.top ?? rect.top;
      const caretBottom = caret?.bottom ?? rect.top;
      const caretLeft = caret?.left ?? rect.left + 12;
      const above = caretTop - 8;
      const below = window.innerHeight - caretBottom - 8;
      const availableHeight = Math.min(height, 320);
      const upward = above >= availableHeight || above >= below;
      const top = upward ? Math.max(8, caretTop - Math.min(height, availableHeight) - 3)
        : caretBottom + 3;
      const left = Math.max(8, Math.min(rect.left + 8, window.innerWidth - width - 8));
      setOrigin(`${Math.max(0, Math.min(width, caretLeft - left))}px ${upward ? '100%' : '0%'}`);
      setAnchorBottom(upward ? caretTop - 3 : 0);
      setMaxHeight(Math.max(120, Math.min(320, upward ? above : below)));
      setPosition({left, top, width});
    };
    place();
    window.addEventListener('resize', place);
    document.addEventListener('scroll', place, true);
    return () => {window.removeEventListener('resize', place); document.removeEventListener('scroll', place, true);};
  }, [anchor, editor, anchorAt, results.length, loading, error]);
  useLayoutEffect(() => {
    const frame = requestAnimationFrame(() => setShown(true));
    return () => cancelAnimationFrame(frame);
  }, []);
  return createPortal(<div ref={panel} className={'agent-reference-picker' + (shown ? ' is-visible' : '')} style={{...position, bottom: anchorBottom ? window.innerHeight - anchorBottom : 'auto', maxHeight, transformOrigin: origin}} role="dialog" aria-label="引用其他对话">
    <div className="agent-reference-picker-head"><Search aria-hidden="true"/><span>{query ? `搜索“${query}”` : '引用对话'}</span><button type="button" aria-label="关闭引用选择" title="关闭" onClick={event => {event.preventDefault(); onClose();}}><X aria-hidden="true"/></button></div>
    <div className="agent-reference-scope" role="group" aria-label="引用搜索范围">
      <button type="button" aria-pressed={scope === 'project'} onClick={() => onScope('project')}>当前项目</button>
      <button type="button" aria-pressed={scope === 'all'} onClick={() => onScope('all')}>所有项目</button>
    </div>
    <div className="agent-reference-results" role="listbox" aria-label="可引用对话">
      {results.map((item, itemIndex) => <button type="button" role="option" aria-selected={itemIndex === index} key={item.sessionId} onMouseDown={event => event.preventDefault()} onClick={() => onChoose(item)}>
        <span><strong>{item.title}</strong><small>{item.project.name}</small></span>
        <time>{new Date(item.updatedAt).toLocaleDateString('zh-CN')}</time>
      </button>)}
      {loading && <p><LoaderCircle className="agent-spin" aria-hidden="true"/>正在搜索…</p>}
      {!loading && error && <p className="is-error" role="alert">{error}</p>}
      {!loading && !error && !results.length && <p>没有匹配的对话</p>}
    </div>
  </div>, document.body);
}

export function ReferenceModal({reference, onOpenSource, onClose}: {reference: AgentConversationReference; onOpenSource: (id: string) => void; onClose: () => void}) {
  const [summary, setSummary] = useState(reference.summary || '');
  const [sourceAvailable, setSourceAvailable] = useState(reference.sourceAvailable !== false);
  const [newContent, setNewContent] = useState(!!reference.hasNewContent);
  const [error, setError] = useState('');
  useEffect(() => {
    let live = true;
    void api.agentReference(reference.id).then(detail => {
      if (!live) return;
      setSummary(detail.summary);
      setSourceAvailable(!!detail.sourceAvailable);
      setNewContent(!!detail.reference.hasNewContent);
    }).catch(reason => { if (live) setError(reason instanceof Error ? reason.message : String(reason)); });
    return () => {live = false;};
  }, [reference.id]);
  return <>{error && <div className="agent-modal-error" role="alert">{error}</div>}<ReferenceDetailModal reference={reference} summary={summary} sourceAvailable={sourceAvailable} newContent={newContent} onOpenSource={() => onOpenSource(reference.sourceSessionId)} onClose={onClose}/></>;
}
