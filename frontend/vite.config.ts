import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { viteSingleFile } from 'vite-plugin-singlefile';
import type { IncomingMessage, ServerResponse } from 'node:http';

const target = process.env.VITE_BACKEND_URL || 'http://localhost:8080';

const proxyConfig = {
  target,
  changeOrigin: true,
  configure: (proxy: {
    on: (
      event: string,
      handler: (err: NodeJS.ErrnoException, req: IncomingMessage, res: ServerResponse) => void
    ) => void;
  }) => {
    proxy.on('error', (err: NodeJS.ErrnoException, req: IncomingMessage, res: ServerResponse) => {
      // Gracefully handle backend offline / connection refused without dumping noisy stack traces
      if (res && 'writeHead' in res && !res.headersSent) {
        try {
          res.writeHead(503, {
            'Content-Type': 'application/json',
            'Retry-After': '5'
          });
          res.end(
            JSON.stringify({
              error: `Kerob engine unreachable at ${target}. Start the engine via 'docker compose up -d' or 'go run ./cmd/server'.`,
              code: err.code || 'ECONNREFUSED',
              path: req.url
            })
          );
        } catch {
          // ignore closed socket
        }
      }
    });
  }
};

export default defineConfig({
  plugins: [svelte(), viteSingleFile()],
  resolve: {
    conditions: ['browser']
  },
  server: {
    port: 5173,
    proxy: {
      '/api': proxyConfig,
      '/health': proxyConfig,
      '/ready': proxyConfig,
      '/metrics': proxyConfig,
      '/webhooks': proxyConfig
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      thresholds: {
        lines: 65,
        statements: 65,
        functions: 65,
        branches: 50
      }
    }
  }
});
