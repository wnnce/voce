import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import basicSsl from '@vitejs/plugin-basic-ssl'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const BACKEND = process.env.VOCE_BACKEND ?? 'http://agora-lab:7001'

// https://vite.dev/config/
export default defineConfig({
  // HTTPS is not cosmetic here: getUserMedia and enumerateDevices only exist in
  // a secure context. Over plain http on a LAN hostname `navigator.mediaDevices`
  // is undefined, so the microphone can never open.
  plugins: [tailwindcss(), react(), basicSsl()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    // The dev server is reached over the LAN and over Tailscale host names.
    // Vite rejects any Host header it does not recognise, which reads as
    // "Blocked request. This host is not allowed" in the browser.
    allowedHosts: ['agora-lab', 'localhost', '127.0.0.1', '10.100.1.191', '.ts.net', '.local'],
    // Proxying keeps the page same-origin with the API: an https page talking
    // to http://agora-lab:7001 directly would be blocked as mixed content.
    proxy: {
      '/workflows': { target: BACKEND, changeOrigin: true },
      '/plugins': { target: BACKEND, changeOrigin: true },
      '/sessions': { target: BACKEND, changeOrigin: true },
      '/health': { target: BACKEND, changeOrigin: true },
      '/metrics': { target: BACKEND, changeOrigin: true },
      '/realtime': { target: BACKEND, changeOrigin: true, ws: true },
    },
  },
})
