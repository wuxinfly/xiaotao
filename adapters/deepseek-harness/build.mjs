#!/usr/bin/env node

import { cp, mkdir, rm } from 'node:fs/promises'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import esbuild from 'esbuild'

const adapterRoot = path.dirname(fileURLToPath(import.meta.url))
const repositoryRoot = path.resolve(adapterRoot, '..', '..')
const outputRoot = path.join(adapterRoot, 'lib')
const coreSource = path.join(repositoryRoot, 'xiaotao')
const typescriptCli = path.join(adapterRoot, 'node_modules', 'typescript', 'bin', 'tsc')

await rm(outputRoot, { recursive: true, force: true })
await mkdir(outputRoot, { recursive: true })

await esbuild.build({
  entryPoints: [path.join(adapterRoot, 'src', 'index.ts')],
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node20',
  outfile: path.join(outputRoot, 'index.js'),
  sourcemap: true,
  external: ['ajv', 'yaml'],
})

const declarationBuild = spawnSync(
  process.execPath,
  [typescriptCli, '--project', path.join(adapterRoot, 'tsconfig.build.json')],
  { cwd: adapterRoot, encoding: 'utf8' },
)
if (declarationBuild.status !== 0) {
  process.stderr.write(declarationBuild.stdout ?? '')
  process.stderr.write(declarationBuild.stderr ?? '')
  process.exit(declarationBuild.status ?? 1)
}

await cp(coreSource, path.join(outputRoot, 'core'), {
  recursive: true,
  filter: (source) => path.basename(source) !== '__pycache__' && !source.endsWith('.pyc'),
})
process.stdout.write('Built lib/index.js, declarations, and packaged XiaoTao Core.\n')
