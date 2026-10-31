#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { access, mkdir, readFile, rename, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { pathToFileURL, fileURLToPath } from 'node:url'

const adapterRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

function usage() {
  return `Usage: npm run dsh:install:local -- [options]

Options:
  --profile <name>       DSH profile to install into (default: web)
  --dsh-home <path>      DSH home directory (default: DSH_HOME or ~/.dsh)
  --skip-dependencies    Skip npm install in the adapter package
  --no-verify            Skip package import and dsh --dump-config checks
  --help                 Show this help
`
}

export function parseArguments(argv, environment = process.env) {
  const options = {
    profile: 'web',
    dshHome: environment.DSH_HOME || path.join(os.homedir(), '.dsh'),
    installDependencies: true,
    verify: true,
    help: false,
  }
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index]
    if (argument === '--help') {
      options.help = true
      continue
    }
    if (argument === '--skip-dependencies') {
      options.installDependencies = false
      continue
    }
    if (argument === '--no-verify') {
      options.verify = false
      continue
    }
    if (argument === '--profile' || argument === '--dsh-home') {
      const value = argv[index + 1]
      if (!value || value.startsWith('--')) throw new Error(`${argument} requires a value`)
      index += 1
      if (argument === '--profile') options.profile = value
      else options.dshHome = value
      continue
    }
    throw new Error(`Unknown option: ${argument}`)
  }
  if (!/^[A-Za-z0-9._-]+$/.test(options.profile)) {
    throw new Error('Profile name may contain only letters, numbers, dot, underscore, and hyphen')
  }
  options.dshHome = path.resolve(options.dshHome)
  return options
}

export function removeLegacyProfileInsert(raw, yaml) {
  if (!raw.trim()) return { changed: false, content: raw, removedCoreDirs: [] }
  const document = yaml.parseDocument(raw)
  if (document.errors.length > 0) {
    throw new Error(`Cannot parse profile cordis.patch.yml: ${document.errors[0].message}`)
  }
  if (!yaml.isSeq(document.contents)) {
    throw new Error('Profile cordis.patch.yml must contain a top-level YAML sequence')
  }

  let changed = false
  const removedCoreDirs = []
  for (let operationIndex = document.contents.items.length - 1; operationIndex >= 0; operationIndex -= 1) {
    const operation = document.contents.items[operationIndex]
    if (!yaml.isMap(operation)) continue
    const insertions = operation.get('insert', true)
    if (!yaml.isSeq(insertions)) continue
    for (let insertionIndex = insertions.items.length - 1; insertionIndex >= 0; insertionIndex -= 1) {
      const candidate = insertions.items[insertionIndex]
      if (!yaml.isMap(candidate)) continue
      if (candidate.get('id') === 'xiaotao-adapter' || candidate.get('name') === '@xiaotao-ai/dsh-adapter') {
        const config = candidate.get('config', true)
        if (yaml.isMap(config) && typeof config.get('coreDir') === 'string') {
          removedCoreDirs.push(config.get('coreDir'))
        }
        insertions.items.splice(insertionIndex, 1)
        changed = true
      }
    }
    if (insertions.items.length === 0) {
      operation.delete('insert')
      if (operation.items.length === 0) document.contents.items.splice(operationIndex, 1)
    }
  }
  return { changed, content: changed ? document.toString() : raw, removedCoreDirs }
}

function run(command, args, options = {}) {
  const capture = options.capture === true
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env,
    encoding: 'utf8',
    shell: process.platform === 'win32',
    stdio: capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
  })
  if (result.error) throw result.error
  if (result.status !== 0) {
    const detail = capture ? `\n${result.stderr || result.stdout || ''}` : ''
    throw new Error(`${command} ${args.join(' ')} exited with code ${result.status}${detail}`)
  }
  return result
}

async function readOptional(filePath) {
  try {
    return await readFile(filePath, 'utf8')
  } catch (error) {
    if (error.code === 'ENOENT') return ''
    throw error
  }
}

async function install(options) {
  const profileRoot = path.join(options.dshHome, 'profiles', options.profile)
  const packageCache = path.join(options.dshHome, 'local-packages', options.profile)
  await mkdir(packageCache, { recursive: true })

  if (options.installDependencies) {
    process.stdout.write('Installing adapter build dependencies...\n')
    run('npm', ['install', '--no-audit', '--no-fund'], { cwd: adapterRoot })
  }

  process.stdout.write('Building and packing the local adapter...\n')
  const packed = run(
    'npm',
    ['pack', '--silent', '--pack-destination', packageCache],
    { cwd: adapterRoot, capture: true },
  )
  const archiveName = packed.stdout.split(/\r?\n/).map((line) => line.trim())
    .findLast((line) => line.endsWith('.tgz'))
  if (!archiveName) throw new Error(`npm pack did not return an archive name:\n${packed.stdout}`)
  const packedPath = path.join(packageCache, archiveName)
  const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '')
  const archivePath = path.join(packageCache, `xiaotao-ai-dsh-adapter-local-${stamp}.tgz`)
  await rename(packedPath, archivePath)

  process.stdout.write(`Installing package into DSH profile "${options.profile}"...\n`)
  const dshEnvironment = { ...process.env, DSH_HOME: options.dshHome }
  // DSH profiles can be workspace roots. Permit this intentional add without
  // using -w, which could redirect installation to an ancestor workspace.
  run('dsh', ['plugin', '--profile', options.profile, 'add', '--ignore-workspace-root-check', archivePath], {
    env: dshEnvironment,
  })

  await mkdir(profileRoot, { recursive: true })
  const patchPath = path.join(profileRoot, 'cordis.patch.yml')
  const yaml = await import('yaml')
  const legacyPatch = removeLegacyProfileInsert(await readOptional(patchPath), yaml)
  if (legacyPatch.changed) {
    await writeFile(patchPath, legacyPatch.content, 'utf8')
    process.stdout.write('Removed the legacy manual XiaoTao insert; DSH now activates the package bundle.\n')
    if (legacyPatch.removedCoreDirs.length > 0) {
      process.stdout.write(`Packaged Core replaced legacy coreDir: ${legacyPatch.removedCoreDirs.join(', ')}\n`)
    }
  }

  if (options.verify) {
    process.stdout.write('Verifying installed package and DSH configuration...\n')
    const requireFromProfile = createRequire(path.join(profileRoot, 'package.json'))
    const entryPath = requireFromProfile.resolve('@xiaotao-ai/dsh-adapter')
    await import(pathToFileURL(entryPath).href)
    await access(path.join(path.dirname(entryPath), 'core', 'SKILL.md'))
    const profileManifest = JSON.parse(await readFile(path.join(profileRoot, 'package.json'), 'utf8'))
    if (!profileManifest.dsh?.profile?.bundles?.includes('@xiaotao-ai/dsh-adapter')) {
      throw new Error('DSH did not activate @xiaotao-ai/dsh-adapter as a profile bundle')
    }
    const dumped = run(
      'dsh',
      ['--profile', options.profile, '--dump-config'],
      { capture: true, cwd: profileRoot, env: dshEnvironment },
    )
    const config = `${dumped.stdout}\n${dumped.stderr}`
    if (!config.includes('xiaotao-adapter') && !config.includes('@xiaotao-ai/dsh-adapter')) {
      throw new Error('DSH config dump does not contain the XiaoTao adapter')
    }
  }

  process.stdout.write(`\nXiaoTao installed locally into DSH profile "${options.profile}".\n`)
  process.stdout.write(`Package: ${archivePath}\n`)
  process.stdout.write('Activation: dsh.profile.bundles → @xiaotao-ai/dsh-adapter\n')
}

async function main() {
  let options
  try {
    options = parseArguments(process.argv.slice(2))
  } catch (error) {
    process.stderr.write(`${error.message}\n\n${usage()}`)
    process.exitCode = 2
    return
  }
  if (options.help) {
    process.stdout.write(usage())
    return
  }
  try {
    await install(options)
  } catch (error) {
    process.stderr.write(`Local DSH install failed: ${error.message}\n`)
    process.exitCode = 1
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  await main()
}
