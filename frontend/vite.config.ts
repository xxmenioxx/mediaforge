import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const backendTarget = process.env.VITE_DEV_PROXY_TARGET ?? 'http://localhost:8080';
const backendProxy = {
  target: backendTarget,
  changeOrigin: true,
  // These are same-application requests routed through the development
  // server. Removing Origin avoids treating LAN access as cross-origin.
  headers: { Origin: '' },
};

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': backendProxy,
      '/health': backendProxy,
      '/openapi.json': backendProxy,
      '/swagger': backendProxy,
    },
  },
});
