import { defineConfig } from 'vitest/config'
import { alias } from './vite.config'

export default defineConfig({
  resolve: { alias },
  // TJARenderer is another project's source, kept as it came; its tests run there.
  test: { include: ['src/**/*.test.ts'], exclude: ['src/TJARenderer/**', 'node_modules/**'] },
})
