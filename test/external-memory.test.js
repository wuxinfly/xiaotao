import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, cpSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
const scripts = resolve('xiaotao/scripts');
function run(script, args, cwd) {
  const result = spawnSync('python', [join(scripts, script), ...args, '--project-root', cwd], {encoding:'utf8'});
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}
test('external document survives duplicate import, rebuild and project move', () => {
  const root = mkdtempSync(join(tmpdir(), 'xiaotao-external-'));
  const source = join(root, 'external.md');
  writeFileSync(source, 'Historical delivery');
  const first = run('external_memory.py', ['import', source], root)[0];
  assert.equal(run('external_memory.py', ['import', source], root)[0].status, 'duplicate');
  const proposal = join(root, 'proposal.json');
  writeFileSync(proposal, JSON.stringify({events:[{occurred_at:'2026-01-02T10:00:00+08:00', title:'交付', summary:'完成历史交付',source_refs:[first.source_ref]}]}));
  run('external_memory.py', ['confirm', first.import_id, proposal], root);
  const index = run('activity_catalog.py', ['build', '--now', '2026-09-24T00:00:00Z'], root);
  assert.equal(index.events, 1);
  run('timeline_catalog.py', ['build'], root);
  run('timeline_catalog.py', ['build'], root);
  const moved = mkdtempSync(join(tmpdir(), 'xiaotao-moved-'));
  cpSync(join(root, '.xiaotao'), join(moved, '.xiaotao'), {recursive:true});
  assert.equal(run('activity_catalog.py', ['build'], moved).events, 1);
  assert.equal(readFileSync(join(moved, first.source_ref), 'utf8'), 'Historical delivery');
});
