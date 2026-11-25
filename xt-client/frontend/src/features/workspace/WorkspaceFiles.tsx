import {useEffect, useRef, useState} from 'react';
import {ArrowLeft, ChevronRight, ExternalLink, File, FileImage, FileText, Folder, RefreshCw} from 'lucide-react';
import {api} from '../../lib/api';
import {toast} from '../../components/toast';
import {workspaceApi, type WorkspaceEntry, type WorkspaceFile, type WorkspaceListing} from '../../lib/workspace';
import './workspace-tools.css';

const errorText = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

export function formatSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes < 0) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10240 ? 1 : 0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function partsOf(path: string) {
  return path.split(/[\\/]/).filter(Boolean);
}

function fileIcon(entry: WorkspaceEntry) {
  if (entry.dir) return <Folder aria-hidden="true"/>;
  if (/\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(entry.name)) return <FileImage aria-hidden="true"/>;
  if (/\.(md|txt|json|js|jsx|ts|tsx|css|html|go|py|rs|yaml|yml|toml|ini|xml|sql|sh|ps1|log)$/i.test(entry.name)) return <FileText aria-hidden="true"/>;
  return <File aria-hidden="true"/>;
}

const messageFor = (file: WorkspaceFile | null) => {
  if (!file) return '';
  if (file.kind === 'too-large') return '文件太大，暂不预览。可以用资源管理器打开它。';
  if (file.kind === 'binary') return '这是二进制文件，不能按文本预览。';
  return '';
};

export function WorkspaceFiles({projectPath}: {projectPath: string}) {
  const [path, setPath] = useState('');
  const [listing, setListing] = useState<WorkspaceListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [file, setFile] = useState<WorkspaceFile | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileError, setFileError] = useState('');
  const [selected, setSelected] = useState('');
  const [tick, setTick] = useState(0);
  const listRequest = useRef(0);
  const fileRequest = useRef(0);

  useEffect(() => {
    setPath('');
    setListing(null);
    setFile(null);
    setSelected('');
    setError('');
    setFileError('');
  }, [projectPath]);

  useEffect(() => {
    if (!projectPath) return;
    const controller = new AbortController();
    const id = ++listRequest.current;
    setLoading(true);
    setError('');
    workspaceApi.list(projectPath, path, controller.signal)
      .then(result => { if (id === listRequest.current) setListing(result); })
      .catch(reason => { if (id === listRequest.current && !controller.signal.aborted) setError(errorText(reason)); })
      .finally(() => { if (id === listRequest.current) setLoading(false); });
    return () => controller.abort();
  }, [projectPath, path, tick]);

  function navigate(next: string) {
    fileRequest.current++;
    setSelected('');
    setFile(null);
    setFileError('');
    if (next === path) setTick(current => current + 1);
    else setPath(next);
  }

  function openFile(entry: WorkspaceEntry) {
    const id = ++fileRequest.current;
    setSelected(entry.path);
    setFile(null);
    setFileError('');
    setFileLoading(true);
    workspaceApi.file(projectPath, entry.path)
      .then(result => { if (id === fileRequest.current) setFile(result); })
      .catch(reason => { if (id === fileRequest.current) setFileError(errorText(reason)); })
      .finally(() => { if (id === fileRequest.current) setFileLoading(false); });
  }

  function closeFile() {
    fileRequest.current++;
    setSelected('');
    setFile(null);
    setFileError('');
    setFileLoading(false);
  }

  function reveal() {
    api.openAgentProject(projectPath).catch(reason => toast.error(errorText(reason)));
  }

  const rootName = listing?.root.split(/[\\/]/).filter(Boolean).at(-1) || '项目目录';
  const parts = partsOf(path);

  return <section className="workspace-file-pane" aria-label="文件管理">
    <header className="workspace-tool-head">
      {selected && <button type="button" className="workspace-tool-icon" aria-label="返回文件列表" title="返回文件列表" onClick={closeFile}>
        <ArrowLeft aria-hidden="true"/>
      </button>}
      <nav className="workspace-crumbs" aria-label="文件路径">
        <button type="button" onClick={() => navigate('')} aria-current={!path ? 'page' : undefined}>{rootName}</button>
        {parts.map((part, index) => <span key={`${index}:${part}`}>
          <ChevronRight aria-hidden="true"/>
          <button type="button" onClick={() => navigate(parts.slice(0, index + 1).join('/'))}
            aria-current={index === parts.length - 1 ? 'page' : undefined}>{part}</button>
        </span>)}
      </nav>
      <button type="button" className="workspace-tool-icon" aria-label="刷新文件列表" title="刷新" onClick={() => setTick(current => current + 1)}>
        <RefreshCw aria-hidden="true"/>
      </button>
      <button type="button" className="workspace-tool-icon" aria-label="在资源管理器中打开项目" title="在资源管理器中打开" onClick={reveal}>
        <ExternalLink aria-hidden="true"/>
      </button>
    </header>

    {selected ? <FilePreview projectPath={projectPath} file={file} loading={fileLoading} error={fileError}
      truncated={!!file?.truncated} onRetry={() => {
        const entry = listing?.entries.find(item => item.path === selected);
        if (entry) openFile(entry);
      }}/> : <div className="workspace-file-list" aria-busy={loading}>
      {error ? <div className="workspace-tool-state is-error" role="alert">
        <strong>没法读取这个文件夹</strong><span>{error}</span>
        <button type="button" onClick={() => setTick(current => current + 1)}>重试</button>
      </div> : loading && !listing ? <p className="workspace-tool-state" role="status">正在读取文件…</p>
      : listing && !listing.entries.length ? <div className="workspace-tool-state">
        <Folder aria-hidden="true"/><strong>这个文件夹是空的</strong><span>可以把文件拖进项目目录后再刷新。</span>
      </div> : <ul>
        {listing?.entries.map(entry => <li key={entry.path}>
          <button type="button" className="workspace-file-row" onClick={() => entry.dir ? navigate(entry.path) : openFile(entry)}>
            <span className="workspace-file-icon" data-dir={entry.dir ? '' : undefined}>{fileIcon(entry)}</span>
            <span className="workspace-file-name" title={entry.name}>{entry.name}</span>
            <span className="workspace-file-meta">{entry.dir ? '文件夹' : formatSize(entry.size)}</span>
            {entry.dir && <ChevronRight aria-hidden="true"/>}
          </button>
        </li>)}
      </ul>}
      {listing?.truncated && <p className="workspace-file-note">文件太多，只显示前 2000 项。</p>}
    </div>}
  </section>;
}

function FilePreview({projectPath, file, loading, error, truncated, onRetry}: {
  projectPath: string; file: WorkspaceFile | null; loading: boolean; error: string; truncated: boolean; onRetry: () => void;
}) {
  if (loading) return <p className="workspace-tool-state" role="status">正在打开文件…</p>;
  if (error) return <div className="workspace-tool-state is-error" role="alert">
    <strong>没法打开这个文件</strong><span>{error}</span><button type="button" onClick={onRetry}>重试</button>
  </div>;
  if (!file) return null;
  const message = messageFor(file);
  return <div className="workspace-file-preview">
    <div className="workspace-preview-meta">
      <strong title={file.name}>{file.name}</strong>
      <span>{formatSize(file.size)}{file.modified ? ` · ${new Date(file.modified).toLocaleDateString()}` : ''}</span>
    </div>
    {message ? <div className="workspace-tool-state"><File aria-hidden="true"/><strong>{message}</strong></div>
      : file.kind === 'image' ? <div className="workspace-image-wrap">
        <img src={workspaceApi.rawUrl(projectPath, file.path, file.modified)} alt={file.name}/>
      </div>
      : <pre className="workspace-text-preview" tabIndex={0}>{file.content || ''}</pre>}
    {truncated && <p className="workspace-file-note">文件较大，只显示前 256 KB。</p>}
  </div>;
}
