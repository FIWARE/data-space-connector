/** Fakes the dsc-config API for component tests by stubbing the global fetch. */
import { vi } from 'vitest'
import type { LoadReport } from '../api/load'
import type { Profile, ProfileResponse, RecentProfiles } from '../api/profile'
import type { FeatureChoice, FeaturesResult, FeatureToggle } from '../api/features'
import type { Offer, PresetResult, RoleCatalogue } from '../api/roles'
import type { Kubeconfig, ModelInfo } from '../api/target'
import type { Trial, TrialWritten } from '../api/trial'

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
  root: '',
  rootFrom: 'detected',
  rootCandidates: [''],
  generated: true,
  sha256: 'a'.repeat(64),
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

/** A load that answers only when `resolve` is called. */
export function lateLoad() {
  let resolve: (report: LoadReport) => void = () => {}
  const pending = new Promise<LoadReport>((done) => (resolve = done))
  return { load: () => pending, resolve }
}

/** Where the fake recent list puts a new profile. */
export const DEFAULT_DIR = '/home/user/dsc'

/** The body PUT /api/profile receives. */
export interface SaveBody {
  path: string
  overwrite: boolean
  profile: Profile
  sourceSha256?: string
}

/** An answer of a profile route, or a status code it fails with. */
export type ProfileAnswer<T> = T | number

/** The profile routes' fakes; by default the list is empty, save echoes and open finds nothing. */
export interface ProfileFakes {
  recent?: () => ProfileAnswer<RecentProfiles> | Promise<ProfileAnswer<RecentProfiles>>
  save?: (body: SaveBody) => ProfileAnswer<ProfileResponse>
  open?: (path: string) => ProfileAnswer<ProfileResponse> | Promise<ProfileAnswer<ProfileResponse>>
}

/** The server's message for a refused profile request. */
export function refusedProfile(path: string, status: number): string {
  return `${path}: refused with ${status}`
}

const PROFILE_DEFAULTS: Required<ProfileFakes> = {
  recent: () => ({ entries: [], defaultDir: DEFAULT_DIR, notes: [] }),
  save: (body) => ({ path: body.path, profile: body.profile, notes: [] }),
  open: () => HTTP_NOT_FOUND,
}

const TRUST_ANCHOR = 'trust-anchor'
const DSC_CHART = 'data-space-connector'
const TRUST_ANCHOR_VERSION = '1.3.6'
const DSC_VERSION = '10.16.0'

/** A trial matching config-tool/trial/trial.yaml: three releases in install order, one warning with a link. */
export const TRIAL: Trial = {
  profile: 'local-trial',
  environment: 'development',
  releases: [TRUST_ANCHOR, 'provider', 'consumer'].map((role) => {
    const trustAnchor = role === TRUST_ANCHOR
    return {
      role,
      releaseName: role,
      namespace: role,
      chart: trustAnchor ? TRUST_ANCHOR : DSC_CHART,
      chartVersion: trustAnchor ? TRUST_ANCHOR_VERSION : DSC_VERSION,
      source: `k3s/${role}.yaml`,
      file: `${role}.yaml`,
      values: `# ${role}\n`,
    }
  }),
  warnings: [
    { id: 'self-signed', releases: ['provider', 'consumer'], message: 'Certificates are self-signed.' },
    { id: 'memory', releases: [TRUST_ANCHOR, 'provider', 'consumer'], message: 'Sized for a laptop.', link: 'https://example.org/sizing' },
  ],
}

/** An answer of a trial route, or a status code it fails with. */
export type TrialAnswer<T> = T | number

/** The trial routes' fakes; by default GET answers TRIAL and write puts the three files into `dir`. */
export interface TrialFakes {
  fetch?: () => TrialAnswer<Trial> | Promise<TrialAnswer<Trial>>
  write?: (dir: string) => TrialAnswer<TrialWritten>
}

/** The server's message for a refused trial request. */
export function refusedTrial(status: number): string {
  return `trial refused with ${status}`
}

const TRIAL_DEFAULTS: Required<TrialFakes> = {
  fetch: () => TRIAL,
  write: (dir) => ({ dir, files: TRIAL.releases.map((release) => `${dir}/${release.file}`) }),
}

/** Offers of one fake role for its recommended, its available and the remaining catalogue features. */
function offers(recommended: string[], available: string[]): Record<string, Offer> {
  return Object.fromEntries(
    ROLE_FEATURES.map((id) => [id, recommended.includes(id) ? 'recommended' : available.includes(id) ? 'available' : 'not-offered']),
  )
}

const ROLE_FEATURES = ['identity.issuer', 'data.scorpio', 'obs.tracing', 'ta.registry']
const DSC_FEATURES = ROLE_FEATURES.filter((id) => id !== 'ta.registry')

/** A role catalogue shaped like the real 10.16.0 one, cut down to four features. */
export const ROLES: RoleCatalogue = {
  roles: [
    { id: 'consumer', title: 'Consumer', summary: 'Consumes data.', chart: DSC_CHART, docs: 'roles#consumer',
      floor: { memory: '0.8Gi', cpu: '50m', measuredWith: '10.13.2' }, features: offers(['identity.issuer'], ['obs.tracing']) },
    { id: 'provider', title: 'Provider', summary: 'Offers data.', chart: DSC_CHART, docs: 'roles#provider',
      floor: { memory: '2.3Gi', cpu: '200m', measuredWith: '10.13.2' }, features: offers(['identity.issuer', 'data.scorpio'], ['obs.tracing']) },
    { id: 'consumer-provider', title: 'Consumer + Provider', summary: 'Both.', chart: DSC_CHART, docs: 'roles#consumer-provider',
      floor: { memory: '2.3Gi', cpu: '200m', measuredWith: '10.13.2' }, features: offers(['identity.issuer', 'data.scorpio'], ['obs.tracing']) },
    { id: 'operator', title: 'Operator', summary: 'Runs the trust anchor.', chart: TRUST_ANCHOR, docs: 'roles#operator',
      floor: { memory: '0.3Gi', cpu: '20m', measuredWith: '1.3.0' }, features: offers(['ta.registry'], []) },
    { id: 'custom', title: 'Custom', summary: 'Pick every feature.', chart: DSC_CHART, docs: 'roles#custom', floor: null,
      features: offers([], DSC_FEATURES) },
  ],
  features: [
    { id: 'identity.issuer', title: 'Credential issuer', summary: 'Keycloak issues VCs.', group: 'identity',
      consequence: 'Participants cannot get credentials from you.' },
    { id: 'data.scorpio', title: 'Context broker', summary: 'Scorpio NGSI-LD.', group: 'data', consequence: '' },
    { id: 'obs.tracing', title: 'Tracing', summary: 'OTEL traces.', group: 'observability', consequence: '' },
    { id: 'ta.registry', title: 'Trusted issuers registry', summary: 'The trust anchor.', group: 'operator', consequence: '' },
  ],
}

/** The body POST /api/preset receives. */
export interface PresetBody {
  profile: Profile
  role: string
  sourceSha256?: string
}

/** The body POST /api/features receives. */
export interface FeaturesBody {
  profile: Profile
  sourceSha256?: string
  toggle: FeatureToggle | null
}

/** An answer of a role route, or a status code it fails with. */
export type RoleAnswer<T> = T | number

/** The role routes' fakes; by default GET answers ROLES, preset applies the role without features or switch and
 *  features answers `featureChoices` after recording the toggle as source user. */
export interface RoleFakes {
  roles?: (chart: string, version: string) => RoleAnswer<RoleCatalogue> | Promise<RoleAnswer<RoleCatalogue>>
  preset?: (body: PresetBody) => RoleAnswer<PresetResult>
  features?: (body: FeaturesBody) => RoleAnswer<FeaturesResult> | Promise<RoleAnswer<FeaturesResult>>
}

/** The choices the server derives from a profile with a ROLES role: every feature of the role's chart, in bundle order. */
export function featureChoices(profile: Profile): FeatureChoice[] {
  const role = ROLES.roles.find((r) => r.id === profile.role)
  if (!role) return []
  const inChart = (id: string) => (id === 'ta.registry') === (role.chart === TRUST_ANCHOR)
  return ROLES.features
    .filter((f) => inChart(f.id))
    .map((f) => {
      const offer = role.features[f.id] ?? 'not-offered'
      const preset = offer === 'recommended'
      const state = profile.features?.[f.id] ?? { enabled: false, source: 'preset' as const }
      return { id: f.id, offer, preset, ...state, deliberate: state.enabled !== preset && state.source !== 'preset' }
    })
}

/** POST /api/features as the server answers it: the toggle recorded as source user, then the choices. */
export function recordToggle(body: FeaturesBody): FeaturesResult {
  const { profile, toggle } = body
  const features = toggle ? { ...profile.features, [toggle.feature]: { enabled: toggle.enabled, source: 'user' as const } } : profile.features
  const next = { ...profile, features }
  return { profile: next, choices: featureChoices(next) }
}

/** The server's message for a refused role request. */
export function refusedRole(status: number): string {
  return `role request refused with ${status}`
}

const ROLE_DEFAULTS: Required<RoleFakes> = {
  roles: () => ROLES,
  preset: (body) => ({ profile: { ...body.profile, role: body.role, features: {} }, chartSwitch: null }),
  features: recordToggle,
}

/** Answers GET /api/model with `model`, GET /api/cluster/contexts with `contexts(path)` and POST /api/load with
 *  `load(name, root)`; `root` is null when the upload lets the server detect it. The profile routes use `profiles`,
 *  the trial routes `trial`, the role routes `roles`. */
export function mockApi(
  contexts: (path: string) => ContextsAnswer = () => ({ paths: [], contexts: [], current: '' }),
  model: ModelInfo = MODEL,
  load: (name: string, root: string | null) => LoadAnswer | Promise<LoadAnswer> = () => REPORT,
  profiles: ProfileFakes = {},
  trial: TrialFakes = {},
  roles: RoleFakes = {},
) {
  const fakes = { ...PROFILE_DEFAULTS, ...profiles }
  const trialFakes = { ...TRIAL_DEFAULTS, ...trial }
  const roleFakes = { ...ROLE_DEFAULTS, ...roles }
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
      const answer = await load(name, url.searchParams.get('root'))
      return typeof answer === 'number' ? json({ error: refusedUpload(name, answer) }, answer) : json(answer)
    }
    const answer = <T,>(path: string, value: ProfileAnswer<T>) => (typeof value === 'number' ? json({ error: refusedProfile(path, value) }, value) : json(value))
    if (url.pathname === '/api/profile/recent') return answer('recent', await fakes.recent())
    if (url.pathname === '/api/profile' && init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as SaveBody
      return answer(body.path, fakes.save(body))
    }
    if (url.pathname === '/api/profile/open' && init?.method === 'POST') {
      const { path } = JSON.parse(String(init.body)) as { path: string }
      return answer(path, await fakes.open(path))
    }
    const trialAnswer = <T,>(value: TrialAnswer<T>) => (typeof value === 'number' ? json({ error: refusedTrial(value) }, value) : json(value))
    if (url.pathname === '/api/trial') return trialAnswer(await trialFakes.fetch())
    if (url.pathname === '/api/trial/write' && init?.method === 'POST') {
      const { dir } = JSON.parse(String(init.body)) as { dir: string }
      return trialAnswer(trialFakes.write(dir))
    }
    const roleAnswer = <T,>(value: RoleAnswer<T>) => (typeof value === 'number' ? json({ error: refusedRole(value) }, value) : json(value))
    if (url.pathname === '/api/roles') {
      return roleAnswer(await roleFakes.roles(url.searchParams.get('chart') ?? '', url.searchParams.get('version') ?? ''))
    }
    if (url.pathname === '/api/preset' && init?.method === 'POST') {
      return roleAnswer(roleFakes.preset(JSON.parse(String(init.body)) as PresetBody))
    }
    if (url.pathname === '/api/features' && init?.method === 'POST') {
      return roleAnswer(await roleFakes.features(JSON.parse(String(init.body)) as FeaturesBody))
    }
    return json({ error: 'not found' }, HTTP_NOT_FOUND)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
