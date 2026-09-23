import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(import.meta.dirname, '..');
const python = process.platform === 'win32' ? 'python' : 'python3';
const catalogScript = path.join(repositoryRoot, 'xiaotao', 'scripts', 'memory_catalog.py');

function runCatalog(projectRoot, args, { env } = {}) {
  return execFileAsync(python, [catalogScript, '--project-root', projectRoot, ...args], {
    cwd: repositoryRoot,
    windowsHide: true,
    env: { ...process.env, PYTHONIOENCODING: 'utf-8', XIAOTAO_CURRENT_TIME: '2026-09-23T12:00:00Z', ...env },
  });
}

function sha256(content) {
  return crypto.createHash('sha256').update(content).digest('hex');
}

async function writeProjectFile(projectRoot, relativePath, content) {
  const target = path.join(projectRoot, ...relativePath.split('/'));
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, content, 'utf8');
}

function entryFile(entry, { revision = 1, updatedAt = '2026-09-23T12:00:00Z' } = {}) {
  return `---
revision: ${revision}
updated_at: ${updatedAt}
updated_by: xiao-tao/maintainer
---

# Long-term Memory Entry

\`\`\`xiaotao-memory-entry
${JSON.stringify(entry, null, 2)}
\`\`\`
`;
}

async function createBenchmarkProject(t) {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-knowledge-benchmark-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));

  // Create tracked code files
  const pkgContent = '{\n  "name": "benchmark-project",\n  "version": "1.0.0"\n}\n';
  const skillContent = '# Benchmark Skill\nCore semantics for XiaoTao.\n';
  const binContent = '#!/usr/bin/env node\nconsole.log("XiaoTao benchmark");\n';

  await writeProjectFile(projectRoot, 'package.json', pkgContent);
  await writeProjectFile(projectRoot, 'xiaotao/SKILL.md', skillContent);
  await writeProjectFile(projectRoot, 'bin/xiaotao.js', binContent);

  const pkgHash = sha256(pkgContent);
  const skillHash = sha256(skillContent);
  const binHash = sha256(binContent);

  // Write project.overview entry
  const overviewEntry = {
    entry_id: 'project.overview',
    title: 'XiaoTao 架构全景与模块组织概览',
    memory_kind: 'fact',
    content: 'XiaoTao 是面向结果的 AI 协作 Skill，按模块分为语义核心 xiaotao/、跨宿主安装与命令行 cli/、宿主适配器 adapters/（包括 deepseek-harness、antigravity、codex）以及自动化测试 test/。核心入口由 bin/xiaotao.js 与 xiaotao/SKILL.md 承载。',
    tags: ['architecture', 'overview', 'modules', 'structure', 'guide'],
    aliases: [
      '项目结构',
      '架构概览',
      '模块划分',
      '代码地图',
      'project structure',
      'architecture overview',
    ],
    search_hints: [
      '项目怎么组织的',
      '核心模块在哪',
      '入口文件是哪个',
      'adapters目录做什么',
      'cli如何运行',
      '如何查看项目全貌',
    ],
    source_refs: [
      'package.json',
      'xiaotao/SKILL.md',
    ],
    code_refs: [
      'package.json',
      'xiaotao/SKILL.md',
      'bin/xiaotao.js',
    ],
    code_fingerprints: {
      'package.json': pkgHash,
      'xiaotao/SKILL.md': skillHash,
      'bin/xiaotao.js': binHash,
    },
    status: 'active',
  };

  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/entries/project.overview.md',
    entryFile(overviewEntry),
  );

  // Write an entry without code references (pure meeting/evidence note)
  await writeProjectFile(
    projectRoot,
    '.xiaotao/evidence/meeting.md',
    '# Meeting Note\nUser requested brief conclusion first.\n',
  );

  const noteEntry = {
    entry_id: 'lt-user-preference',
    title: '用户偏好先说结论',
    memory_kind: 'principle',
    content: '用户希望沟通时始终用简洁大白话中文先说结论，不汇报常规代码查找或内部派工过程。',
    tags: ['communication', 'persona'],
    aliases: ['先说结论', '大白话沟通'],
    source_refs: ['.xiaotao/evidence/meeting.md'],
    status: 'active',
  };

  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/entries/lt-user-preference.md',
    entryFile(noteEntry),
  );

  // Build the catalog index
  await runCatalog(projectRoot, ['build']);

  return {
    projectRoot,
    pkgHash,
    skillHash,
    binHash,
    originalBinContent: binContent,
  };
}

test('knowledge retrieval benchmark suite passes top-5 recall and negative assertions', async (t) => {
  const { projectRoot } = await createBenchmarkProject(t);
  const benchmarkPath = path.join(
    repositoryRoot,
    'xiaotao',
    'references',
    'scenarios',
    'knowledge-benchmark.json',
  );
  const scenarios = JSON.parse(await readFile(benchmarkPath, 'utf8'));

  for (const scenario of scenarios) {
    const rawResult = await runCatalog(projectRoot, ['search', scenario.query]);
    const result = JSON.parse(rawResult.stdout);
    const candidates = result.candidates || [];

    if (scenario.expect_empty) {
      assert.equal(
        candidates.length,
        0,
        `Expected 0 candidates for negative query "${scenario.query}", got ${candidates.length}`,
      );
    } else {
      assert.ok(
        candidates.length >= scenario.min_expected_matches,
        `Expected at least ${scenario.min_expected_matches} candidates for "${scenario.query}", got ${candidates.length}`,
      );

      const topIds = candidates.slice(0, 5).map((c) => c.memory_id);
      for (const expectedId of scenario.expected_top_ids) {
        assert.ok(
          topIds.includes(expectedId),
          `Expected ${expectedId} in top 5 results for query "${scenario.query}", actual top 5: ${JSON.stringify(topIds)}`,
        );
      }

      // Assert candidates have relevance_reason and valid scores
      assert.ok(candidates[0].score > 0);
      assert.ok(candidates[0].relevance_reason.length > 0);
    }
  }
});

test('code freshness check returns fresh when code matches, review_needed when code changes, and unknown when no code refs', async (t) => {
  const { projectRoot, originalBinContent } = await createBenchmarkProject(t);
  const overviewFile = path.join(projectRoot, '.xiaotao/memory/long-term/entries/project.overview.md');
  const originalOverviewText = await readFile(overviewFile, 'utf8');

  // 1. Initial state: all code matches fingerprints -> fresh
  const initialShow = JSON.parse((await runCatalog(projectRoot, ['show', 'project.overview'])).stdout);
  assert.equal(initialShow.code_freshness.status, 'fresh');
  assert.ok(initialShow.code_freshness.reason.includes('一致') || initialShow.code_freshness.reason.includes('match'));
  assert.equal(initialShow.code_freshness.files.length, 3);
  for (const f of initialShow.code_freshness.files) {
    assert.equal(f.status, 'fresh');
  }

  // Also check that search returns candidate with code_freshness
  const searchResult = JSON.parse((await runCatalog(projectRoot, ['search', '架构概览'])).stdout);
  const overviewCandidate = searchResult.candidates.find((c) => c.memory_id === 'project.overview');
  assert.ok(overviewCandidate);
  assert.equal(overviewCandidate.code_freshness.status, 'fresh');

  // 2. Modify one of the code files -> review_needed with reason
  const modifiedBin = originalBinContent + '\n// modified for benchmark test\n';
  await writeProjectFile(projectRoot, 'bin/xiaotao.js', modifiedBin);

  const modifiedShow = JSON.parse((await runCatalog(projectRoot, ['show', 'project.overview'])).stdout);
  assert.equal(modifiedShow.code_freshness.status, 'review_needed');
  assert.ok(modifiedShow.code_freshness.reason.includes('bin/xiaotao.js'));
  assert.ok(modifiedShow.code_freshness.reason.includes('变动') || modifiedShow.code_freshness.reason.includes('changed'));

  const changedFile = modifiedShow.code_freshness.files.find((f) => f.path === 'bin/xiaotao.js');
  assert.ok(changedFile);
  assert.equal(changedFile.status, 'changed');

  // Guard: Authority entry file MUST NOT be quietly tampered with or modified
  const currentOverviewText = await readFile(overviewFile, 'utf8');
  assert.equal(
    currentOverviewText,
    originalOverviewText,
    'Guard violation: show command must never silently rewrite or modify the authoritative memory entry on disk',
  );

  // 3. Restore the code file -> returns to fresh
  await writeProjectFile(projectRoot, 'bin/xiaotao.js', originalBinContent);
  const restoredShow = JSON.parse((await runCatalog(projectRoot, ['show', 'project.overview'])).stdout);
  assert.equal(restoredShow.code_freshness.status, 'fresh');

  // 4. Missing code file -> review_needed with missing reason
  await rm(path.join(projectRoot, 'bin', 'xiaotao.js'));
  const missingShow = JSON.parse((await runCatalog(projectRoot, ['show', 'project.overview'])).stdout);
  assert.equal(missingShow.code_freshness.status, 'review_needed');
  assert.ok(missingShow.code_freshness.reason.includes('不存在') || missingShow.code_freshness.reason.includes('missing'));

  // 5. Entry without code references -> status is unknown, never falsely claim success
  const noteShow = JSON.parse((await runCatalog(projectRoot, ['show', 'lt-user-preference'])).stdout);
  assert.equal(noteShow.code_freshness.status, 'unknown');
  assert.ok(noteShow.code_freshness.reason.includes('无关联代码') || noteShow.code_freshness.reason.includes('no associated code'));
});
