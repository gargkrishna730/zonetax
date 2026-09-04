import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// ZoneTax's Vite app is built and its output embedded directly into the Go collector binary
// (see ui/ui.go's go:embed) so the whole dashboard ships as part of the single collector image
// with no separate frontend deployment. base: './' makes all asset URLs relative so the app
// works when served from the collector's root ("/") without needing an absolute public path
// baked in at build time. outDir points one level up (ui/dist) so it sits next to ui.go, since
// Go's go:embed cannot reach outside the directory containing the source file.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    // In dev, proxy API calls to the real collector (reached via `kubectl port-forward` on
    // 18099) so the app can be visually verified against real live cluster data instead of
    // mocked fixtures — the collector serves the built SPA itself in production, so this proxy
    // only matters for `npm run dev`.
    proxy: {
      '/api': 'http://localhost:18099',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // Without this, Vitest can inherit a "production" NODE_ENV from the invoking shell/CI,
    // which makes React resolve its production build (no react-dom/test-utils act()) and every
    // component test fails with "React.act is not a function" — force "test" explicitly so
    // React always loads its development build under Vitest regardless of the outer shell env.
    env: { NODE_ENV: 'test' },
  },
})
