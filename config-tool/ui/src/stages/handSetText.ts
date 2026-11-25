/** UI copy of S7 Review's "Set by hand" section, apart from the component so fast refresh keeps working. */

/** Heading of the section. */
export const HAND_SET_HEADING = 'Set by hand'
/** What a hand-set answer means for the user. */
export const HAND_SET_HINT = 'These answers keep their value when the answers they would be derived from change, such as the base domain.'

/** A given answer kept while the answers it would derive from change, on Foundations and Review. */
export const PINNED_LABEL = 'pinned, not updated'

/** The note after a hand-set value whose derived value differs. */
export function wouldDerive(derived: string): string {
  return `would be ${derived} if derived`
}
