import {useState} from 'react';
import {officeApi, type OfficeTaskDetail} from '../../lib/office';
import './delivery-card.css';

export function DeliveryCard({detail, onRefresh}: {detail: OfficeTaskDetail; onRefresh: () => void}) {
  const {task, delivery} = detail;
  const [sending, setSending] = useState(false);
  const [error, setError] = useState('');
  const interrupted = task.status === 'failed' && (task.interrupted || task.error === '服务重启导致任务中断，本轮无法自动恢复');
  async function resume() {
    setSending(true);
    setError('');
    try {
      await officeApi.resumeTask(task.projectPath, task.id);
      onRefresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setSending(false);
    }
  }
  return <section className="office-delivery" aria-label="本轮交付">
    <h3>本轮交付</h3>
    {detail.deliveryError && <p role="alert">{detail.deliveryError}</p>}
    {delivery ? <>
      <h4>工作区变化</h4>
      {delivery.fileNote && <p>{delivery.fileNote}</p>}
      {!!delivery.files?.length && <p>共享工作区中观察到的变化可能包含同步编辑。</p>}
      {delivery.files?.map(file => file.diff
        ? <details key={file.path}><summary>{file.path} · {file.label}</summary><pre>{file.diff}</pre></details>
        : <p key={file.path}>{file.path} · {file.label}</p>)}
      {!delivery.fileNote && !delivery.files?.length && <p>本轮未观察到 Git 文件变化。</p>}
      <h4>命令与验证</h4>
      {delivery.commands?.map(command => <details key={command.id}>
        <summary>{command.status === 'completed' ? '成功' : command.status === 'failed' ? '失败' : '结果待核对'} · <code>{command.command}</code></summary>
        <pre>{command.output || '没有输出记录'}</pre>
      </details>)}
      {delivery.commandsClipped && <p>这里只展示前 100 条命令，完整过程见时间线。</p>}
      {!delivery.commands?.length && <p>本轮没有已记录的命令验证。</p>}
    </> : <p>此轮没有交付记录，可查看下方执行过程和产物。</p>}
    {interrupted && <>
      <h4>中断接续</h4>
      <p>最后确认完成：{delivery?.confirmed || (task.latestActivity?.status === 'completed' && task.latestActivity.detail) || '暂无记录'}</p>
      {Object.entries(delivery?.pending || {}).map(([id, label]) => <p key={id}>结果待核对：{label}</p>)}
      {!delivery && task.latestActivity?.status === 'running' && <p>结果待核对：{task.latestActivity.title} {task.latestActivity.detail}</p>}
      <button type="button" className="office-primary" disabled={sending || !task.sessionId} onClick={() => void resume()}>{sending ? '提交中…' : '检查现场并继续'}</button>
      {error && <p role="alert">{error}</p>}
    </>}
  </section>;
}
