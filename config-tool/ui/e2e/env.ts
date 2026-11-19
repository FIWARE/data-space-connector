/** Names of the variables the global setup hands to the tests, and the fixture they read. */
import { join } from 'node:path'

/** The token URL `dsc-config --no-browser` printed. */
export const TOKEN_URL_ENV = 'DSC_E2E_TOKEN_URL'
/** A page on another site whose only link is the token URL. */
export const CROSS_SITE_URL_ENV = 'DSC_E2E_CROSS_SITE_URL'
/** Overrides the binary under test; the default is the one `make build` writes. */
export const BINARY_ENV = 'DSC_CONFIG_BIN'

/** Suffix `make build` appends on Windows, mirroring the Makefile's `$(EXE)`. */
const EXE_SUFFIX = process.platform === 'win32' ? '.exe' : ''
export const DEFAULT_BINARY = join(import.meta.dirname, '..', '..', 'bin', `dsc-config${EXE_SUFFIX}`)
export const KUBECONFIG_FIXTURE = join(import.meta.dirname, 'fixtures', 'kubeconfig.yaml')
/** A real hand-written values.yaml from the repository, opened through S0. */
export const PROVIDER_VALUES = join(import.meta.dirname, '..', '..', '..', 'k3s', 'provider.yaml')
/** The fixture's current-context, and the other context it holds. */
export const FIXTURE_CURRENT_CONTEXT = 'prod-eu-central'
export const FIXTURE_OTHER_CONTEXT = 'staging'

/** Reads a variable the global setup must have set. */
export function required(name: string): string {
  const value = process.env[name]
  if (!value) throw new Error(`${name} is not set; run the suite through playwright.config.ts`)
  return value
}
