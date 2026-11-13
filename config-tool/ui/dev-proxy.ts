/** Development-only proxy from the Vite dev server to a running dsc-config; never in a build. */
import type { IncomingHttpHeaders, IncomingMessage, ServerResponse } from 'node:http'
import type { ProxyOptions } from 'vite'

/** Environment variable holding the URL printed by `dsc-config --no-browser`. */
export const CONFIG_URL_ENV = 'DSC_CONFIG_URL'
/** Fixed dev-server port: the guard's own-origin list depends on it, so it must not drift. */
export const DEV_PORT = 5173
/** The only host the dev server and dsc-config listen on. */
export const LOOPBACK_HOST = '127.0.0.1'
/** Path prefix forwarded to dsc-config. */
export const API_PREFIX = '/api'

const LOCALHOST = 'localhost'
const TOKEN_PARAM = 'token'
const HTTP_FORBIDDEN = 403
const SAFE_METHODS: ReadonlySet<string> = new Set(['GET', 'HEAD'])
const SAME_ORIGIN = 'same-origin'
const UPGRADE_FORBIDDEN = `HTTP/1.1 ${HTTP_FORBIDDEN} Forbidden\r\nConnection: close\r\n\r\n`

const MISSING_URL_HELP = `${CONFIG_URL_ENV} is not set. Start \`dsc-config --no-browser\` and run
  ${CONFIG_URL_ENV}='<printed URL>' npm run dev`

/** Where dsc-config listens and the run token the proxy adds to every request it lets through. */
export interface ConfigTarget {
  port: number
  token: string
}

/** Parses the printed dsc-config URL; throws with instructions when it is missing or malformed. */
export function parseConfigUrl(raw: string | undefined): ConfigTarget {
  if (!raw) {
    throw new Error(MISSING_URL_HELP)
  }
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    throw new Error(`${CONFIG_URL_ENV} is not a URL: ${raw}`)
  }
  const token = url.searchParams.get(TOKEN_PARAM)
  const port = Number(url.port)
  if (url.protocol !== 'http:' || url.hostname !== LOOPBACK_HOST || !port || !token) {
    throw new Error(`${CONFIG_URL_ENV} must be the URL dsc-config printed, http://${LOOPBACK_HOST}:<port>/?${TOKEN_PARAM}=<token>`)
  }
  return { port, token }
}

/** The dev server's own origins, the only ones the proxy forwards for. */
export function devOrigins(devPort: number): string[] {
  return [`http://${LOOPBACK_HOST}:${devPort}`, `http://${LOCALHOST}:${devPort}`]
}

function single(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value.join(',') : value
}

/**
 * Applies the server's Origin rules before the proxy adds the token, so no other page can use it.
 * Returns why the request gets a 403, or undefined when it may pass.
 */
export function devProxyGuard(method: string, headers: IncomingHttpHeaders, ownOrigins: readonly string[]): string | undefined {
  const origin = single(headers.origin)
  const unsafe = !SAFE_METHODS.has(method.toUpperCase()) || headers.upgrade !== undefined
  if (origin !== undefined && !ownOrigins.includes(origin)) {
    return `foreign Origin ${origin}`
  }
  if (origin === undefined && unsafe) {
    return 'Origin missing on an unsafe request'
  }
  const site = single(headers['sec-fetch-site'])
  if (site !== undefined && site !== SAME_ORIGIN) {
    return `Sec-Fetch-Site ${site}`
  }
  return undefined
}

/** Builds the `/api` proxy entry; the guard runs in `bypass`, which Vite calls for HTTP and WebSocket alike. */
export function devProxy(target: ConfigTarget, devPort: number): Record<string, ProxyOptions> {
  const backend = `http://${LOOPBACK_HOST}:${target.port}`
  const ownOrigins = devOrigins(devPort)
  return {
    [API_PREFIX]: {
      target: backend,
      changeOrigin: true,
      ws: true,
      // Headers are set on req before the proxy copies them, so they reach the WebSocket too.
      bypass(req: IncomingMessage, res: ServerResponse | undefined) {
        const reason = devProxyGuard(req.method ?? '', req.headers, ownOrigins)
        if (reason === undefined) {
          req.headers.authorization = `Bearer ${target.token}`
          if (req.headers.origin !== undefined) {
            req.headers.origin = backend
          }
          return undefined
        }
        console.warn(`dev proxy: ${HTTP_FORBIDDEN} ${req.method} ${req.url}: ${reason}`)
        if (res) {
          res.writeHead(HTTP_FORBIDDEN).end()
        } else {
          req.socket.end(UPGRADE_FORBIDDEN)
        }
        // A string with the response ended tells Vite the request is handled; it does not proxy.
        return req.url ?? API_PREFIX
      },
    },
  }
}
