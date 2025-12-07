import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const backendUrl = process.env.VITE_BACKEND_URL || 'http://localhost:8080';
  
  return {
    plugins: [vue()],
    server: {
      port: 5173,
      proxy: {
        '/namecoin': {
          target: backendUrl,
          changeOrigin: true
        },
        '/blockchain': {
          target: backendUrl,
          changeOrigin: true
        }
      }
    }
  };
})
