import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The build output is written straight into the Go package that embeds it, so
// `go build` always ships the dashboard that was built from this source.
export default defineConfig({
  base: '/dashboard/',
  plugins: [react()],
  build: {
    outDir: '../internal/proxy/inbound/dashboard/dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: false,
  },
})
