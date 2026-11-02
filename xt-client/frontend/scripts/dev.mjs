import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const args = process.argv.slice(2);
const index = args.indexOf('--api-port');
if (index !== -1) {
  const port = args[index + 1];
  if (!port || !/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535 || args.lastIndexOf('--api-port') !== index) {
    console.error('--api-port requires one port number between 1 and 65535');
    process.exit(1);
  }
  process.env.XIAOTAO_DEV_API = `http://127.0.0.1:${Number(port)}`;
  args.splice(index, 2);
}

const vite = fileURLToPath(new URL('../node_modules/vite/bin/vite.js', import.meta.url));
const child = spawn(process.execPath, [vite, ...args], {stdio: 'inherit', env: process.env});
child.on('error', error => {
  console.error(error.message);
  process.exitCode = 1;
});
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code ?? 1;
});
