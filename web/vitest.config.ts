import path from 'node:path'
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

export default defineConfig((configEnv) => {
  const resolvedViteConfig = typeof viteConfig === 'function' ? viteConfig(configEnv) : viteConfig

  return mergeConfig(resolvedViteConfig, {
    test: {
      environment: 'jsdom',
      globals: true,
      include: ['src/**/*.{test,spec}.{ts,tsx}'],
      setupFiles: ['./vitest.setup.ts'],
      css: false,
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html'],
        include: ['src/**/*.{ts,tsx}'],
        exclude: [
          'src/**/*.test.{ts,tsx}',
          'src/main.tsx',
          'src/routes/**',
          'src/components/ui/**',
          'src/api/types.ts',
        ],
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
  })
})
