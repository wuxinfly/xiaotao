#!/usr/bin/env node

import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { access, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const adapterRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const packageJson = JSON.parse(await readFile(path.join(adapterRoot, 'package.json'), 'utf8'))
const requiredFiles = [
  'lib/index.js',
  'lib/index.d.ts',
  'lib/core/SKILL.md',
  'lib/core/references/storage.md',
  'cordis.patch.yml',
]
for (const relativePath of requiredFiles) await access(path.join(adapterRoot, relativePath))

assert.equal(packageJson.main, './lib/index.js')
assert.equal(packageJson.types, './lib/index.d.ts')
assert.equal(packageJson.exports['.'].default, './lib/index.js')
assert.equal(packageJson.dsh.bundle.patch, './cordis.patch.yml')

const module = await import(pathToFileURL(path.join(adapterRoot, 'lib', 'index.js')).href)
assert.equal(typeof module.apply, 'function')
assert.deepEqual(module.inject, ['skills'])

let registeredSkill
const skills = {
  register(skill) {
    registeredSkill = skill
    return () => {}
  },
}
  const injectedDeps = []
  await module.apply({
    skills,
    get(name) {
      return name === 'skills' ? skills : undefined
    },
    effect() {},
    // This plain-Skill fixture never publishes optional filesystem/tools/agents services.
    inject(dependencies) { injectedDeps.push(dependencies) },
    logger: { info() {}, warn() {} },
  })
  assert.deepEqual(injectedDeps, [['fs'], ['agents']])
assert.equal(registeredSkill.name, 'xiaotao')
assert.equal(registeredSkill.resourceBase.kind, 'directory')
assert.equal(registeredSkill.resourceBase.path, path.join(adapterRoot, 'lib', 'core'))

const pack = spawnSync('npm', ['pack', '--dry-run', '--ignore-scripts', '--json'], {
  cwd: adapterRoot,
  encoding: 'utf8',
  shell: process.platform === 'win32',
})
if (pack.status !== 0) throw new Error(pack.stderr || pack.stdout)
const manifest = JSON.parse(pack.stdout)[0]
const packedFiles = new Set(manifest.files.map((file) => file.path))
for (const relativePath of requiredFiles) assert.equal(packedFiles.has(relativePath), true)
assert.equal([...packedFiles].some((file) => file.includes('__pycache__') || file.endsWith('.pyc')), false)
process.stdout.write('Adapter package runtime and bundled Core are ready.\n')
