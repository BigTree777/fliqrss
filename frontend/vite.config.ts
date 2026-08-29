import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, '.', '')
  const port = Number(environment.FRONTEND_PORT ?? 5173)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('FRONTEND_PORT must be an integer between 1 and 65535')
  }

  const allowedHosts = environment.FRONTEND_ALLOWED_HOSTS
    ?.split(',')
    .map((host) => host.trim())
    .filter(Boolean)

  return {
    plugins: [react()],
    server: {
      host: environment.FRONTEND_HOST ?? '127.0.0.1',
      port,
      strictPort: true,
      allowedHosts,
      proxy: {
        '/api': environment.BACKEND_PROXY_URL ?? 'http://127.0.0.1:8080',
      },
    },
  }
})
