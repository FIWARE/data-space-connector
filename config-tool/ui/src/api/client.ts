/** Same-origin client for the dsc-config API; the session cookie travels by itself. */

/** Path prefix of every API route. */
export const API_BASE = '/api'
/** WebSocket path for progress events. */
export const WS_PATH = `${API_BASE}/ws`

const HTTP_UNAUTHORIZED = 401
/** The status of a missing resource, such as an explicit kubeconfig path that does not exist. */
export const HTTP_NOT_FOUND = 404
const HTTP_NOT_IMPLEMENTED = 501
const CONTENT_TYPE_JSON = 'application/json'

/** Error body every API failure carries. */
interface ErrorBody {
  error?: string
  ticket?: string
}

/** Any non-2xx answer; `status` is the HTTP status. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/** 401: the session cookie is missing or stale; reopen the URL dsc-config printed. */
export class UnauthorizedError extends ApiError {
  constructor(message: string) {
    super(HTTP_UNAUTHORIZED, message)
    this.name = 'UnauthorizedError'
  }
}

/** 501: the route exists but a later ticket implements it. */
export class NotImplementedError extends ApiError {
  constructor(
    message: string,
    readonly ticket?: string,
  ) {
    super(HTTP_NOT_IMPLEMENTED, message)
    this.name = 'NotImplementedError'
  }
}

async function errorFrom(response: Response): Promise<ApiError> {
  let body: ErrorBody = {}
  try {
    body = (await response.json()) as ErrorBody
  } catch {
    // Not every failure has a JSON body; fall back to the status text.
  }
  const message = body.error ?? response.statusText
  switch (response.status) {
    case HTTP_UNAUTHORIZED:
      return new UnauthorizedError(message)
    case HTTP_NOT_IMPLEMENTED:
      return new NotImplementedError(message, body.ticket)
    default:
      return new ApiError(response.status, message)
  }
}

async function request(path: string, init: RequestInit): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', CONTENT_TYPE_JSON)
  }
  const response = await fetch(`${API_BASE}${path}`, { ...init, headers, credentials: 'same-origin' })
  if (!response.ok) {
    throw await errorFrom(response)
  }
  return response
}

/** Calls `path` under /api and decodes the JSON body; throws an ApiError subclass on failure.
 *  An empty or non-JSON body (204 included) is an ApiError too; use apiNoContent for routes that answer without one. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await request(path, init)
  const text = await response.text()
  if (text === '') {
    throw new ApiError(response.status, `${API_BASE}${path}: empty answer where a JSON body was expected`)
  }
  try {
    return JSON.parse(text) as T
  } catch {
    throw new ApiError(response.status, `${API_BASE}${path}: answer is not JSON`)
  }
}

/** Calls `path` under /api for its effect only (PUT, DELETE); any 2xx body is ignored. */
export async function apiNoContent(path: string, init: RequestInit = {}): Promise<void> {
  await request(path, init)
}

/** One message on the progress WebSocket. */
export interface ProgressEvent {
  type: string
  version?: string
  payload?: unknown
}

/** Opens the progress WebSocket on the page's own origin; the caller closes the returned socket. */
export function connectProgress(onEvent: (event: ProgressEvent) => void, location: Location = window.location): WebSocket {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const socket = new WebSocket(`${scheme}//${location.host}${WS_PATH}`)
  socket.addEventListener('message', (message: MessageEvent<string>) => {
    const event = parseProgressEvent(message.data)
    if (event === undefined) {
      console.warn('dropped a malformed progress frame')
      return
    }
    onEvent(event)
  })
  return socket
}

function parseProgressEvent(data: string): ProgressEvent | undefined {
  let value: unknown
  try {
    value = JSON.parse(data)
  } catch {
    return undefined
  }
  const isEvent = typeof value === 'object' && value !== null && typeof (value as ProgressEvent).type === 'string'
  return isEvent ? (value as ProgressEvent) : undefined
}
