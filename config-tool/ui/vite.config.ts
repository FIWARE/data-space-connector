import react from '@vitejs/plugin-react'
import { configDefaults, defineConfig } from 'vitest/config'
import { CONFIG_URL_ENV, DEV_PORT, LOOPBACK_HOST, devProxy, parseConfigUrl } from './dev-proxy.ts'

export default defineConfig(({ command, mode, isPreview }) => {
  const devServer = command === 'serve' && !isPreview && mode !== 'test'
  return {
    plugins: [react()],
    // emptyOutDir deletes the tracked dist/.gitkeep that go:embed needs on a fresh clone;
    // public/.gitkeep is what copies it back, so keep both files.
    build: { outDir: 'dist', emptyOutDir: true },
    server: devServer
      ? {
          host: LOOPBACK_HOST,
          port: DEV_PORT,
          strictPort: true,
          proxy: devProxy(parseConfigUrl(process.env[CONFIG_URL_ENV]), DEV_PORT),
        }
      : undefined,
    test: {
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts'],
      // e2e/ is the Playwright suite against the built binary (npm run e2e).
      exclude: [...configDefaults.exclude, 'e2e/**'],
    },
  }
})
