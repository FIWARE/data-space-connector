/** The two routes the Target stage reads: the chart-version model and the kubeconfig contexts. */
import { api } from './client'

/** One chart version the tool has a model for. */
export interface ModelVersion {
  version: string
  latest: boolean
}

/** One chart and its versions, sorted newest first; `latest` is per chart. */
export interface ModelChart {
  name: string
  versions: ModelVersion[]
}

/** Body of GET /api/model. */
export interface ModelInfo {
  charts: ModelChart[]
}

/** Body of GET /api/cluster/contexts: context names only, never credentials or server URLs. */
export interface Kubeconfig {
  paths: string[]
  contexts: string[]
  current: string
}

/** Query parameter of the explicit kubeconfig path. */
export const QUERY_KUBECONFIG = 'kubeconfig'

/** Loads the charts and versions offered on the Target stage. */
export function fetchModel(signal?: AbortSignal): Promise<ModelInfo> {
  return api<ModelInfo>('/model', { signal })
}

/** Lists kube contexts; an empty `path` uses the default loading rules. A missing explicit path answers 404. */
export function fetchContexts(path: string, signal?: AbortSignal): Promise<Kubeconfig> {
  const query = path === '' ? '' : `?${new URLSearchParams({ [QUERY_KUBECONFIG]: path }).toString()}`
  return api<Kubeconfig>(`/cluster/contexts${query}`, { signal })
}
