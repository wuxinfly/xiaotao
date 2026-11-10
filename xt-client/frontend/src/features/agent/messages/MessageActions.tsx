import {useRef, useState} from 'react';
import {Copy, GitBranch, MoreHorizontal, PanelRight} from 'lucide-react';
import {Popover} from '../../../components/popover/Popover';
import {toast} from '../../../components/toast';
import {copyText} from '../../../lib/clipboard';
import './message-actions.css';

type Props = {
  messageId?: string;
  content?: string;
  canBranch?: boolean;
  branchDisabled?: boolean;
  onBranch?: (messageId: string) => void;
  onOpenProcess?: () => void;
};

function copyWithToast(content: string) {
  void copyText(content).then(ok => {
    if (ok) toast.success('已复制');
    else toast.error('复制失败，请手动选择文字复制', {duration: 4000});
  });
}

/** Direct copy button for a user query; no menu, so one click is enough. */
export function MessageCopyButton({content}: {content: string}) {
  if (!content.trim()) return null;
  return <div className="agent-message-actions">
    <button
      type="button"
      className="agent-message-actions-trigger"
      aria-label="复制提问"
      title="复制提问"
      onClick={() => copyWithToast(content)}
    >
      <Copy aria-hidden="true"/>
    </button>
  </div>;
}

/**
 * Hover/focus "more" menu for an answer. Copy targets the original Markdown so
 * it can be pasted back into the composer without losing structure; the
 * thought trail lives here instead of adding another icon to the meta row.
 */
export function MessageActions({messageId, content, canBranch = false, branchDisabled = false, onBranch, onOpenProcess}: Props) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const text = content ?? '';
  const hasText = !!text.trim();
  const canCopy = hasText;
  const branchable = canBranch && !!messageId && !!onBranch;
  if (!canCopy && !branchable && !onOpenProcess) return null;

  const close = () => setOpen(false);
  const restoreFocus = () => trigger.current?.focus({preventScroll: true});

  return <div className="agent-message-actions">
    <button
      ref={trigger}
      type="button"
      className="agent-message-actions-trigger"
      aria-label="消息操作"
      title="消息操作"
      aria-haspopup="menu"
      aria-expanded={open}
      onClick={() => setOpen(value => !value)}
    >
      <MoreHorizontal aria-hidden="true"/>
    </button>
    <Popover open={open} onClose={close} anchorRef={trigger} side="top" align="end" className="agent-message-menu" aria-label="消息操作">
      {canCopy && <button type="button" role="menuitem" onClick={() => {close(); restoreFocus(); copyWithToast(text);}}><Copy aria-hidden="true"/>复制</button>}
      {branchable && <button type="button" role="menuitem" onClick={() => {close(); restoreFocus(); onBranch!(messageId!);}} disabled={branchDisabled} title={branchDisabled ? '任务运行中，结束后可创建分支' : '复制这段对话到新会话，之后可以继续聊'}><GitBranch aria-hidden="true"/>创建分支</button>}
      {onOpenProcess && <button type="button" role="menuitem" onClick={() => {close(); restoreFocus(); onOpenProcess();}}><PanelRight aria-hidden="true"/>查看思考过程</button>}
    </Popover>
  </div>;
}