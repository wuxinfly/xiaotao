import {ModalFrame} from '../../components/modal/ModalFrame';
import type {OfficeArtifactDetail} from '../../lib/office';

export function ArtifactModal({detail, onClose}: {detail: OfficeArtifactDetail; onClose: () => void}) {
  const artifact = detail.artifact;
  return <ModalFrame title={artifact.path.split('/').pop() || artifact.path}
    description={`${artifact.path} · ${artifact.tool} · 来自任务「${artifact.taskTitle}」`}
    className="office-artifact-modal" onClose={onClose}
    footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" onClick={onClose}>关闭</button></div>}>
    {detail.binary && <p className="office-muted">这是二进制文件，暂不预览。</p>}
    {detail.error && <p className="office-error" role="alert">{detail.error}</p>}
    {detail.truncated && <p className="office-hint">文件较大，只显示前 256 KB。</p>}
    {!detail.binary && !detail.error && <pre className="office-artifact-content">{detail.content}</pre>}
  </ModalFrame>;
}
