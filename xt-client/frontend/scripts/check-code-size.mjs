import {readdir, readFile} from 'node:fs/promises';
import {extname, join, relative, resolve} from 'node:path';

const frontendRoot = resolve(import.meta.dirname, '..');
const sourceRoot = join(frontendRoot, 'src');
const limits = {
  '.ts': {file: 400, component: 150},
  '.tsx': {file: 400, component: 250},
  '.css': {file: 500},
};
const baseline = new Map([
  ['src/features/agent/AgentPage.tsx', 1933],
  ['src/features/accounts/AccountsPage.tsx', 544],
  ['src/features/dashboard/UsageHeatmap.tsx', 332],
  ['src/features/agent/ReviewListModal.tsx', 286],
  ['src/features/accounts/accounts.css', 1444],
  ['src/features/agent/agent.css', 1093],
  ['src/features/dashboard/dashboard.css', 907],
  ['src/ocean-tech.css', 606],
  ['src/lib/api.ts', 599],
]);
const componentBaseline = new Map([
  ['src/features/agent/AgentPage.tsx:AgentPage', 1070],
  ['src/features/accounts/AccountsPage.tsx:AccountsPage', 525],
  ['src/features/dashboard/UsageHeatmap.tsx:UsageHeatmap', 306],
  ['src/features/agent/ReviewListModal.tsx:ReviewListModal', 274],
]);

async function collect(directory) {
  const entries = await readdir(directory, {withFileTypes: true});
  const files = [];
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await collect(path));
    else if (limits[extname(entry.name)]) files.push(path);
  }
  return files;
}

function componentLengths(text) {
  const functions = [...text.matchAll(/^(?:export\s+)?function\s+([A-Z]\w*)\s*\(/gm)];
  return functions.map((match, index) => {
    const start = text.slice(0, match.index).split('\n').length;
    const end = index + 1 < functions.length
      ? text.slice(0, functions[index + 1].index).split('\n').length - 1
      : text.split('\n').length;
    return {name: match[1], start, lines: end - start + 1};
  });
}

const problems = [];
for (const path of await collect(sourceRoot)) {
  const relativePath = relative(frontendRoot, path).replaceAll('\\', '/');
  const text = await readFile(path, 'utf8');
  const lines = text.split('\n').length;
  const extension = extname(path);
  const limit = limits[extension];
  if (lines > limit.file) {
    const previous = baseline.get(relativePath);
    problems.push({
      severity: previous === undefined
        ? 'error'
        : lines > previous ? 'error' : 'baseline',
      path: relativePath,
      lines,
      limit: limit.file,
      previous,
    });
  }
  if (!limit.component) continue;
  for (const component of componentLengths(text)) {
    if (component.lines > limit.component) {
      const previous = componentBaseline.get(`${relativePath}:${component.name}`);
      problems.push({
        severity: previous === undefined
          ? 'error'
          : component.lines > previous ? 'error' : 'baseline',
        path: relativePath,
        name: component.name,
        start: component.start,
        lines: component.lines,
        limit: limit.component,
        previous,
      });
    }
  }
}

const failures = problems.filter(problem => problem.severity === 'error');
for (const problem of problems) {
  const location = problem.start
    ? `${problem.path}:${problem.start} ${problem.name}`
    : problem.path;
  process.stdout.write(
    `${problem.severity.toUpperCase()} ${location} ${problem.lines} lines (limit ${problem.limit}${problem.previous !== undefined ? `, baseline ${problem.previous}` : ''})\n`,
  );
}

if (failures.length) {
  process.stderr.write(`\n${failures.length} unbaselined size violation(s). Split by responsibility.\n`);
  process.exitCode = 1;
} else {
  process.stdout.write('\nNo new file or component size violations.\n');
}
