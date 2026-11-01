import {defineConfig} from 'vite';
import {env} from 'node:process';

// Optional preview against an already-running local service. This deliberately
// rewrites Host and Origin to satisfy the service's local-only origin guard.
const apiTarget = env.XIAOTAO_DEV_API?.trim();
const parsedTarget = apiTarget ? new URL(apiTarget) : undefined;
if (parsedTarget && !['127.0.0.1', 'localhost', '::1'].includes(parsedTarget.hostname)) {
  throw new Error('XIAOTAO_DEV_API must point to a loopback service');
}
const proxy = parsedTarget ? {
  '/api': {
    target: parsedTarget.origin,
    changeOrigin: true,
    configure(proxy) {
      proxy.on('proxyReq', request => {
        request.setHeader('Host', parsedTarget.host);
        request.setHeader('Origin', parsedTarget.origin);
      });
    },
  },
} : undefined;

export default defineConfig({
  server: {proxy},
  build: {outDir: '../internal/app/web', emptyOutDir: true},
});
