import {readdir, readFile} from 'node:fs/promises';
import {extname, join, relative, resolve} from 'node:path';

const clientRoot = resolve(import.meta.dirname, '..');
const roots = ['cmd', 'internal', 'tools'].map(path => join(clientRoot, path));
const fileLimit = 600;
const baseline = new Map([
  ['internal/agent/service.go', 1207],
  ['internal/proxy/service.go', 1079],
  ['internal/app/app.go', 961],
  ['internal/usage/service.go', 850],
]);

async function collect(directory) {
  const files = [];
  for (const entry of await readdir(directory, {withFileTypes: true})) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await collect(path));
    else if (extname(entry.name) === '.go' && !entry.name.endsWith('_test.go')) files.push(path);
  }
  return files;
}

const failures = [];
for (const root of roots) {
  for (const path of await collect(root)) {
    const relativePath = relative(clientRoot, path).replaceAll('\\', '/');
    const lines = (await readFile(path, 'utf8')).split('\n').length;
    if (lines <= fileLimit) continue;
    const previous = baseline.get(relativePath);
    const isError = previous === undefined || lines > previous;
    process.stdout.write(`${isError ? 'ERROR' : 'BASELINE'} ${relativePath} ${lines} lines (limit ${fileLimit}${previous !== undefined ? `, baseline ${previous}` : ''})\n`);
    if (isError) failures.push(relativePath);
  }
}

if (failures.length) {
  process.stderr.write(`\n${failures.length} unbaselined Go file size violation(s). Split by responsibility.\n`);
  process.exitCode = 1;
} else {
  process.stdout.write('\nNo new Go file size violations.\n');
}
