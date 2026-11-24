/** The data space answers' ids and the session record of the TIR reachability check (ADR 0022, Decision 5). */
import type { Reachability } from '../api/dataspace'
import type { AppState } from './reducer'

/** Answer ids of the dataspace foundations, as `internal/dataspace` names them. */
export const TIR_ADDRESS_ID = 'dataspace.tirAddress'
export const CREDENTIAL_TYPES_ID = 'dataspace.credentialTypes'
export const AUTO_REGISTRATION_ID = 'dataspace.autoRegistration'

/** `skipped` while an address is set and no check ran for it. */
export type TirOutcome = Reachability | 'skipped'

/** The last TIR check of this session, tied to the address it ran for; never a profile field. */
export interface TirCheck {
  address: string
  outcome: TirOutcome
  /** The HTTP status a reachable TIR answered with, or why an unreachable one failed. */
  detail?: string
}

/** The check of the current TIR address: the recorded one when it ran for this address, else skipped; undefined
 *  while no address is given. */
export function tirCheckOf(state: Pick<AppState, 'answers' | 'tirCheck'>): TirCheck | undefined {
  const address = state.answers.values[TIR_ADDRESS_ID]
  if (typeof address !== 'string') return undefined
  return state.tirCheck?.address === address ? state.tirCheck : { address, outcome: 'skipped' }
}
