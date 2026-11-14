import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {ModalFrame} from '../../components/modal/ModalFrame';
import type {OfficeArtifactDetail} from '../../lib/office';

type Props = {detail: OfficeArtifactDetail; stacked: boolean; returnFocus: HTMLElement | null; onClose: () => void};

export function ArtifactModal({detail, stacked, returnFocus, onClose}: Props) {
  const artifact = detail.artifact;
  const markdown = /\.(md|markdown|mdown|mkd)$/i.test(artifact.path);
  return <ModalFrame title={artifact.path.split(/[\\/]/).pop() || artifact.path}
    description={`${artifact.path} · ${artifact.tool} · 来自任务「${artifact.taskTitle}」`}
    className="office-artifact-modal" onClose={onClose} stacked={stacked} returnFocus={returnFocus}
    footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" onClick={onClose}>关闭</button></div>}>
    {detail.binary && <p className="office-muted">这是二进制文件，暂不预览。</p>}
    {detail.error && <p className="office-error" role="alert">{detail.error}</p>}
    {detail.truncated && <p className="office-hint">文件较大，只显示前 256 KB。</p>}
    {!detail.binary && !detail.error && (markdown
      ? <article className="agent-message-content office-artifact-markdown"><Markdown skipHtml remarkPlugins={[remarkGfm]}>{detail.content}</Markdown></article>
      : <pre className="office-artifact-content">{detail.content}</pre>)}
  </ModalFrame>;
}
