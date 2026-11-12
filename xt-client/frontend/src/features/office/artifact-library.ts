import type {OfficeArtifact} from '../../lib/office';

export interface ArtifactDirectory {
  id: string;
  label: string;
}

export interface ArtifactBook {
  artifact: OfficeArtifact;
  directory: ArtifactDirectory;
  fileName: string;
  extension: string;
}

function cleanPath(path: string) {
  return path.replaceAll('\\', '/').replace(/^\/+|\/+$/g, '');
}

export function artifactDirectory(path: string): ArtifactDirectory {
  const parts = cleanPath(path).split('/').filter(Boolean);
  const directory = parts.slice(0, -1).join('/');
  return directory
    ? {id: directory.toLowerCase(), label: directory}
    : {id: 'root', label: '项目根目录'};
}

export function artifactBook(artifact: OfficeArtifact): ArtifactBook {
  const clean = cleanPath(artifact.path);
  const fileName = clean.split('/').pop() || artifact.path;
  const dot = fileName.lastIndexOf('.');
  return {
    artifact,
    directory: artifactDirectory(artifact.path),
    fileName,
    extension: dot > 0 ? fileName.slice(dot + 1).toLowerCase() : '',
  };
}

export function filterArtifactBooks(items: ArtifactBook[], query: string) {
  const term = query.trim().toLowerCase();
  if (!term) return items;
  return items.filter(item =>
    item.fileName.toLowerCase().includes(term)
    || item.artifact.path.toLowerCase().includes(term)
    || item.artifact.taskTitle.toLowerCase().includes(term),
  );
}

export function artifactDirectories(items: ArtifactBook[]) {
  const directories = new Map<string, ArtifactDirectory & {count: number}>();
  for (const item of items) {
    const current = directories.get(item.directory.id);
    if (current) current.count += 1;
    else directories.set(item.directory.id, {...item.directory, count: 1});
  }
  return [...directories.values()].sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'));
}

export function groupArtifactBooks(items: ArtifactBook[]) {
  const groups = new Map<string, {directory: ArtifactDirectory; items: ArtifactBook[]}>();
  for (const item of items) {
    const current = groups.get(item.directory.id);
    if (current) current.items.push(item);
    else groups.set(item.directory.id, {directory: item.directory, items: [item]});
  }
  return [...groups.values()].sort((a, b) => a.directory.label.localeCompare(b.directory.label, 'zh-CN'));
}
