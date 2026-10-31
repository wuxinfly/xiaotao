#!/usr/bin/env node

import { mkdir, rename, rm } from 'node:fs/promises'
import { access } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const adapterRoot = path.resolve(scriptDir, '..')
const outputDir = path.join(adapterRoot, 'adapter')
const outputArchive = path.join(outputDir, 'xiaotao-dsh-adapter.tgz')

function run(command, args, { capture = false } = {}) {
  const result = spawnSync(command, args, {
    cwd: adapterRoot,
    encoding: 'utf8',
    shell: process.platform === 'win32',
    stdio: capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
  })
  if (result.status !== 0) {
    if (capture) {
      process.stderr.write(result.stdout ?? '')
      process.stderr.write(result.stderr ?? '')
    }
    process.exit(result.status ?? 1)
  }
  return result
}

await rm(outputDir, { recursive: true, force: true })
await mkdir(outputDir, { recursive: true })

const esbuildModule = path.join(adapterRoot, 'node_modules', 'esbuild')
const hasDependencies = await access(esbuildModule).then(() => true, () => false)
if (!hasDependencies) {
  process.stdout.write('Installing adapter build dependencies...\n')
  run('npm', ['install', '--no-dry-run', '--no-audit', '--no-fund'])
}

run('npm', ['run', 'build', '--silent'])

const packed = run('npm', ['pack', '--silent', '--no-dry-run', '--ignore-scripts', '--pack-destination', outputDir], { capture: true })
const archiveName = packed.stdout.split(/\r?\n/).map((line) => line.trim())
  .findLast((line) => line.endsWith('.tgz'))
if (!archiveName) throw new Error(`npm pack did not return an archive name:\n${packed.stdout}`)

await rename(path.join(outputDir, archiveName), outputArchive)
process.stdout.write(`Built ${outputArchive}\n`)
