// Bundle the canonical Skill without requiring the source checkout at runtime.
import {readdir, readFile, writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {join, relative} from 'node:path';
import {gzipSync} from 'node:zlib';
const source = fileURLToPath(new URL('../../xt-skills/xiaotao/', import.meta.url));
const output = fileURLToPath(new URL('../internal/agent/core.json.gz', import.meta.url));
const files = {};
async function walk(dir) {
  for (const entry of (await readdir(dir, {withFileTypes:true})).sort((a,b)=>a.name.localeCompare(b.name, 'en'))) {
    if (entry.name === '__pycache__' || entry.name.endsWith('.pyc')) continue;
    const path = join(dir, entry.name);
    if (entry.isDirectory()) await walk(path);
    else if (entry.isFile()) files[relative(source,path).replaceAll('\\','/')] = await readFile(path,'utf8');
  }
}
await walk(source);
await writeFile(output, gzipSync(JSON.stringify(files), {level:9}));
