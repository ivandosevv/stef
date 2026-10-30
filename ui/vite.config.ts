import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  base: './',
  plugins: [svelte(), tailwindcss()],
  server: {
    proxy: {
      '/api': { target: process.env.STEF_DAEMON ?? 'http://127.0.0.1:4820' },
      '/healthz': { target: process.env.STEF_DAEMON ?? 'http://127.0.0.1:4820' },
      '/ws': { target: process.env.STEF_DAEMON ?? 'http://127.0.0.1:4820', ws: true },
    },
  },
});
