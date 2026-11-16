/** Fakes the dsc-config API for component tests by stubbing the global fetch. */
import { vi } from 'vitest'
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

/** A kubeconfig answer, or a status code the route fails with. */
export type ContextsAnswer = Kubeconfig | number

/** Answers GET /api/model with `model` and GET /api/cluster/contexts with `contexts(path)`. */
export function mockApi(contexts: (path: string) => ContextsAnswer = () => ({ paths: [], contexts: [], current: '' }), model: ModelInfo = MODEL) {
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://localhost')
    const json = (body: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(body), { status }))
    if (url.pathname === '/api/model') return json(model)
    if (url.pathname === '/api/cluster/contexts') {
      const answer = contexts(url.searchParams.get('kubeconfig') ?? '')
      return typeof answer === 'number' ? json({ error: 'kubeconfig not found' }, answer) : json(answer)
    }
    return json({ error: 'not found' }, 404)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
