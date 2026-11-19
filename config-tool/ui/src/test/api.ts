/** Fakes the dsc-config API for component tests by stubbing the global fetch. */
import { vi } from 'vitest'
import type { LoadReport } from '../api/load'
import type { Kubeconfig, ModelInfo } from '../api/target'

/** The model the real 10.16.0 bundle produces. */
export const MODEL: ModelInfo = {
  charts: [
    {
      name: 'data-space-connector',
      versions: [
        { version: '10.16.0', latest: true },
        { version: '10.15.0', latest: false },
      ],
    },
  ],
}

/** The status the contexts route answers for a kubeconfig it cannot read. */
export const HTTP_NOT_FOUND = 404

/** A kubeconfig answer, or a status code the route fails with. */
export type ContextsAnswer = Kubeconfig | number

/** The KUBECONFIG file refusedKubeconfig names when the field is empty. */
export const DEFAULT_RULES_FILE = '/dev/fd/63'

/** The server's message for a kubeconfig it refused; the default rules name the file they read. */
export function refusedKubeconfig(path: string): string {
  return `cannot read kubeconfig: ${path || DEFAULT_RULES_FILE} is not a regular file`
}

/** A load report that fills every list, has both answer origins and one underived marker id. */
export const REPORT: LoadReport = {
  source: 'provider.yaml',
  chart: 'data-space-connector',
  chartVersion: '10.16.0',
  chartVersionFrom: 'header',
  generated: true,
  counts: { owned: 1, recognized: 1, unknown: 2, skipped: 1, total: 5 },
  owned: [{ path: 'did.enabled', line: 5, value: 'true', answer: 'identity.did.enabled' }],
  recognized: [{ path: 'scorpio.enabled', line: 9, value: 'false', answer: 'data.scorpio.enabled' }],
  unknown: [
    { path: 'keycloak.ingress.host', line: 12, value: 'kc.example.org' },
    { path: 'marketplace.replicas', line: 20, value: '2', note: 'differs from market.replicas = 1' },
  ],
  skipped: [{ path: 'mongodb.enabled', line: 30, value: 'false', note: 'shadowed by line 31' }],
  answers: [
    { id: 'identity.did.enabled', stage: 'features', value: true, paths: ['did.enabled'], from: 'owned' },
    { id: 'domain.base', stage: 'foundations', value: 'example.org', paths: ['keycloak.ingress.host'], from: 'recognized' },
  ],
  markers: [{ path: 'did', line: 4, answerIds: ['identity.did.enabled', 'did.value'], underived: ['did.value'] }],
}

/** A load report, or a status code the route fails with. */
export type LoadAnswer = LoadReport | number

/** The server's message for an upload it refused. */
export function refusedUpload(name: string, status: number): string {
  return `cannot load ${name}: refused with ${status}`
}

/** Answers GET /api/model with `model`, GET /api/cluster/contexts with `contexts(path)` and POST /api/load with `load(name)`. */
export function mockApi(
  contexts: (path: string) => ContextsAnswer = () => ({ paths: [], contexts: [], current: '' }),
  model: ModelInfo = MODEL,
  load: (name: string) => LoadAnswer | Promise<LoadAnswer> = () => REPORT,
) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost')
    const json = (body: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(body), { status }))
    if (url.pathname === '/api/model') return json(model)
    if (url.pathname === '/api/cluster/contexts') {
      const answer = contexts(url.searchParams.get('kubeconfig') ?? '')
      return typeof answer === 'number' ? json({ error: refusedKubeconfig(url.searchParams.get('kubeconfig') ?? '') }, answer) : json(answer)
    }
    if (url.pathname === '/api/load' && init?.method === 'POST') {
      const name = url.searchParams.get('name') ?? ''
      const answer = await load(name)
      return typeof answer === 'number' ? json({ error: refusedUpload(name, answer) }, answer) : json(answer)
    }
    return json({ error: 'not found' }, HTTP_NOT_FOUND)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
