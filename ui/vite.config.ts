import { defineConfig, type ProxyOptions } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import type { ClientRequest, IncomingMessage } from 'node:http';

function localProxy(ws = false): ProxyOptions {
  const target = process.env.STEF_DAEMON ?? 'http://127.0.0.1:4820';
  return {
    target, ws, changeOrigin: true,
    configure(proxy) {
      const rewrite = (outgoing: ClientRequest, incoming: IncomingMessage) => {
        const origin = incoming.headers.origin;
        if (!origin || origin !== 'http://' + incoming.headers.host) return;
        const parsed = new URL(origin);
        if (parsed.port !== '5173' || !['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname)) return;
        // Rewrite only our own development browser origin. Foreign origins
        // remain intact for the daemon to reject, including WebSocket upgrades.
        outgoing.setHeader('Origin', new URL(target).origin);
      };
      proxy.on('proxyReq', rewrite);
      proxy.on('proxyReqWs', rewrite);
    },
  };
}

export default defineConfig({
  base: './',
  plugins: [svelte(), tailwindcss()],
  server: {
    proxy: {
      '/api': localProxy(),
      '/healthz': localProxy(),
      '/ws': localProxy(true),
    },
  },
});
