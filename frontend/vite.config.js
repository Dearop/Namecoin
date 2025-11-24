import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/namecoin': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/blockchain': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
