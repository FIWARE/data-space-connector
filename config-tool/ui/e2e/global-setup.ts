/** Starts dsc-config and a page on another site that links to it; returns the teardown. */
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { createInterface } from 'node:readline'
import { BINARY_ENV, CROSS_SITE_URL_ENV, DEFAULT_BINARY, KUBECONFIG_FIXTURE, TOKEN_URL_ENV } from './env.ts'

const START_TIMEOUT_MS = 15_000
const LOOPBACK = '127.0.0.1'
// localhost is another site than 127.0.0.1; another port on 127.0.0.1 would be same-site.
const CROSS_SITE_HOST = 'localhost'
const TOKEN_URL = /^http:\/\/127\.0\.0\.1:\d+\/\?token=\S+$/

/** Waits for the token URL on the first lines of stdout. */
async function startBinary() {
  const binary = process.env[BINARY_ENV] || DEFAULT_BINARY
  const child = spawn(binary, ['--no-browser'], {
    env: { ...process.env, KUBECONFIG: KUBECONFIG_FIXTURE },
    stdio: ['ignore', 'pipe', 'inherit'],
  })
  let spawnError: Error | undefined
  child.once('error', (err) => (spawnError = err))
  const lines = createInterface({ input: child.stdout })
  const timer = setTimeout(() => child.kill(), START_TIMEOUT_MS)
  try {
    for await (const line of lines) {
      if (TOKEN_URL.test(line.trim())) return { child, url: line.trim() }
    }
  } finally {
    clearTimeout(timer)
  }
  throw new Error(`${binary} exited before printing its URL${spawnError ? `: ${spawnError.message}` : ''}`)
}

function escapeAttribute(value: string): string {
  return value.replaceAll('&', '&amp;').replaceAll('"', '&quot;')
}

async function startCrossSitePage(tokenUrl: string) {
  const page = `<!doctype html><title>elsewhere</title><a href="${escapeAttribute(tokenUrl)}">open dsc-config</a>`
  const server = createServer((_, res) => {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    res.end(page)
  })
  server.listen(0, LOOPBACK)
  await once(server, 'listening')
  const { port } = server.address() as AddressInfo
  return { server, url: `http://${CROSS_SITE_HOST}:${port}/` }
}

export default async function globalSetup() {
  const { child, url } = await startBinary()
  const crossSite = await startCrossSitePage(url)
  process.env[TOKEN_URL_ENV] = url
  process.env[CROSS_SITE_URL_ENV] = crossSite.url
  return async () => {
    crossSite.server.close()
    if (child.exitCode === null && child.signalCode === null) {
      child.kill()
      await once(child, 'exit')
    }
  }
}
