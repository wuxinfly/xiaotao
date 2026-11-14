import {useMemo, useState} from 'react';
import {BookOpen, FileCode2, FileText, Folder, LibraryBig, Search, X} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {formatBytes, type OfficeArtifact} from '../../lib/office';
import {artifactBook, artifactDirectories, filterArtifactBooks, groupArtifactBooks} from './artifact-library';
import './library.css';

type Props = {
  artifacts: OfficeArtifact[];
  covered: boolean;
  onOpenArtifact: (artifact: OfficeArtifact, trigger: HTMLButtonElement) => void;
  onClose: () => void;
};

export function ArtifactLibraryModal({artifacts, covered, onOpenArtifact, onClose}: Props) {
  const [query, setQuery] = useState('');
  const [directoryId, setDirectoryId] = useState('');
  const allBooks = useMemo(() => artifacts.map(artifactBook), [artifacts]);
  const directories = useMemo(() => artifactDirectories(allBooks), [allBooks]);
  const visibleBooks = useMemo(
    () => filterArtifactBooks(allBooks, query).filter(item => !directoryId || item.directory.id === directoryId),
    [allBooks, directoryId, query],
  );
  const groups = useMemo(() => groupArtifactBooks(visibleBooks), [visibleBooks]);

  return <ModalFrame
    title="项目资料馆"
    description="员工已按目录归档，点一本书即可打开原文。"
    className="office-library-modal"
    covered={covered}
    onClose={onClose}
    footer={<button type="button" className="app-modal-button secondary" onClick={onClose}>关闭</button>}
  >
    <div className="office-library-layout">
      <aside className="office-library-index" aria-label="资料目录">
        <label className="office-library-search">
          <Search size={15} aria-hidden="true"/>
          <input value={query} onChange={event => setQuery(event.target.value)} placeholder="查找文件、任务或目录" aria-label="查找资料"/>
          {query && <button type="button" aria-label="清除资料搜索" onClick={() => setQuery('')}><X size={13}/></button>}
        </label>
        <div className="office-library-directory-list">
          <button type="button" className={!directoryId ? 'is-active' : ''} onClick={() => setDirectoryId('')}>
            <LibraryBig size={15}/><span>全部资料</span><b>{allBooks.length}</b>
          </button>
          {directories.map(directory => <button
            key={directory.id}
            type="button"
            className={directory.id === directoryId ? 'is-active' : ''}
            onClick={() => setDirectoryId(directory.id)}
          >
            <Folder size={15}/><span title={directory.label}>{directory.label}</span><b>{directory.count}</b>
          </button>)}
        </div>
        <p className="office-library-note">目录来自项目内文件路径，员工的归档结果会随资料刷新。</p>
      </aside>
      <section className="office-library-books" aria-label="资料书架">
        <header className="office-library-books-head">
          <div><span>整理完成</span><strong>{visibleBooks.length} 本资料</strong></div>
          {directoryId && <button type="button" className="office-ghost" onClick={() => setDirectoryId('')}>查看全部目录</button>}
        </header>
        {groups.map(group => <section className="office-library-shelf" key={group.directory.id}>
          <h3><Folder size={15}/>{group.directory.label}<small>{group.items.length} 本</small></h3>
          <div className="office-library-book-grid">
            {group.items.map((item, index) => <button
              key={item.artifact.id}
              type="button"
              className="office-library-book"
              style={{animationDelay: `${Math.min(index * 45, 320)}ms`}}
              onClick={event => onOpenArtifact(item.artifact, event.currentTarget)}
              aria-haspopup="dialog"
              aria-label={`打开资料 ${item.fileName}`}
            >
              <span className="office-library-book-spine" aria-hidden="true"/>
              <span className="office-library-book-icon">{item.extension === 'md' ? <BookOpen size={18}/> : item.extension === 'json' ? <FileCode2 size={18}/> : <FileText size={18}/>}</span>
              <strong title={item.fileName}>{item.fileName}</strong>
              <small title={item.artifact.path}>{item.artifact.path}</small>
              <span className="office-library-book-meta">{formatBytes(item.artifact.size)} · {item.artifact.taskTitle}</span>
            </button>)}
          </div>
        </section>)}
        {!groups.length && <div className="office-library-empty"><LibraryBig size={28}/><strong>没有找到资料</strong><p>换一个关键词，或者查看全部目录。</p></div>}
      </section>
    </div>
  </ModalFrame>;
}
