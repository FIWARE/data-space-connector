/** Starts dsc-config and a page on another site that links to it; returns the teardown. */
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createInterface } from 'node:readline'
import { BINARY_ENV, CROSS_SITE_URL_ENV, DEFAULT_BINARY, KUBECONFIG_FIXTURE, PROFILE_DIR_ENV, TOKEN_URL_ENV } from './env.ts'

const START_TIMEOUT_MS = 15_000
const LOOPBACK = '127.0.0.1'
// localhost is another site than 127.0.0.1; another port on 127.0.0.1 would be same-site.
const CROSS_SITE_HOST = 'localhost'
const TOKEN_URL = /^http:\/\/127\.0\.0\.1:\d+\/\?token=\S+$/
// Mirrors internal/recent.ConfigHomeEnv; keeps the developer's real recent list untouched.
const CONFIG_HOME_ENV = 'DSC_CONFIG_HOME'

/** Waits for the token URL on the first lines of stdout. */
async function startBinary(configHome: string) {
  const binary = process.env[BINARY_ENV] || DEFAULT_BINARY
  const child = spawn(binary, ['--no-browser'], {
    env: { ...process.env, KUBECONFIG: KUBECONFIG_FIXTURE, [CONFIG_HOME_ENV]: configHome },
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
  const configHome = await mkdtemp(join(tmpdir(), 'dsc-config-home-'))
  const profileDir = await mkdtemp(join(tmpdir(), 'dsc-config-profiles-'))
  const { child, url } = await startBinary(configHome)
  const crossSite = await startCrossSitePage(url)
  process.env[TOKEN_URL_ENV] = url
  process.env[CROSS_SITE_URL_ENV] = crossSite.url
  process.env[PROFILE_DIR_ENV] = profileDir
  return async () => {
    crossSite.server.close()
    if (child.exitCode === null && child.signalCode === null) {
      child.kill()
      await once(child, 'exit')
    }
    await Promise.all([configHome, profileDir].map((dir) => rm(dir, { recursive: true, force: true })))
  }
}
