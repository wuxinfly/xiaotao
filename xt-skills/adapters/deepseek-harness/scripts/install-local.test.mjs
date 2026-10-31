import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import * as yaml from 'yaml'
import { parseArguments, removeLegacyProfileInsert } from './install-local.mjs'

test('local installer defaults to the web profile', () => {
  const options = parseArguments([], { DSH_HOME: '/tmp/dsh-home' })
  assert.equal(options.profile, 'web')
  assert.equal(options.dshHome, path.resolve('/tmp/dsh-home'))
  assert.equal(options.installDependencies, true)
  assert.equal(options.verify, true)
})

test('local installer parses explicit safe options', () => {
  const options = parseArguments([
    '--profile', 'headless-dev',
    '--dsh-home', '/tmp/custom-dsh',
    '--skip-dependencies',
    '--no-verify',
  ], {})
  assert.equal(options.profile, 'headless-dev')
  assert.equal(options.dshHome, path.resolve('/tmp/custom-dsh'))
  assert.equal(options.installDependencies, false)
  assert.equal(options.verify, false)
})

test('local installer rejects profile path traversal', () => {
  assert.throws(
    () => parseArguments(['--profile', '../web'], {}),
    /Profile name may contain only/,
  )
})

test('legacy manual insert is removed without changing unrelated plugins', () => {
  const original = `# existing profile patch
- insert:
    - id: devtools
      name: mcp-electron-devtools
    - id: xiaotao-adapter
      name: '@xiaotao-ai/dsh-adapter'
      inject: [skills]
`
  const result = removeLegacyProfileInsert(original, yaml)
  const value = yaml.parse(result.content)
  const insertions = value.flatMap((operation) => operation.insert ?? [])
  assert.equal(result.changed, true)
  assert.equal(insertions.some((item) => item.id === 'xiaotao-adapter'), false)
  assert.equal(insertions.some((item) => item.id === 'devtools'), true)
  assert.match(result.content, /# existing profile patch/)
})

test('legacy coreDir is reported while the manual insert is removed', () => {
  const original = `- insert:
    - id: xiaotao-adapter
      name: '@xiaotao-ai/dsh-adapter'
      config:
        coreDir: D:/code/xiaotao/xiaotao
`
  const result = removeLegacyProfileInsert(original, yaml)
  assert.deepEqual(result.removedCoreDirs, ['D:/code/xiaotao/xiaotao'])
  assert.deepEqual(yaml.parse(result.content), [])
})

test('profile patch is untouched when it has no legacy XiaoTao insert', () => {
  const original = '# keep exact formatting\n- insert: [{ id: devtools, name: tools }]\n'
  const result = removeLegacyProfileInsert(original, yaml)
  assert.equal(result.changed, false)
  assert.equal(result.content, original)
})
