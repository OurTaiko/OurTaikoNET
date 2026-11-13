import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

const from = (path: string) => fileURLToPath(new URL(path, import.meta.url))
// @sitekit: what both song sources share. @fanmade and @official: each source's own parts.
export const alias = {
  '@sitekit': from('./src/kit'),
  '@fanmade': from('./src/sources/fanmade'),
  '@official': from('./src/sources/official'),
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')

  return {
    plugins: [react(), tailwindcss()],
    resolve: { alias },
    server: {
      watch: { usePolling: env.VITE_USE_POLLING === 'true' },
      proxy: {
        // OurTaikoNet, the one backend.
        '/api': {
          target: env.VITE_API_TARGET || 'http://127.0.0.1:8090',
          changeOrigin: true,
        },
      },
    },
  }
})
