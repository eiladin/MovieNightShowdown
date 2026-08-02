/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [react()],
  // Component tests need a DOM, and the pure unit tests run fine under jsdom
  // too, so one environment covers the whole suite. Test files sit beside the
  // code they cover; they stay out of the production bundle because Rollup
  // only follows the module graph reachable from index.html.
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
    restoreMocks: true,
  },
})
