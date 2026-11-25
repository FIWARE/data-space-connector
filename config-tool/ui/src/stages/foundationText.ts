/** Text the Foundations stage derives from bundle data, apart from the component for fast refresh. */
import type { Foundation } from '../api/foundations'
import type { Finding } from '../api/render'

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

/** The fan-out button text, e.g. "used by 3 values in 2 components". */
export function usedByText(f: Pick<Foundation, 'count' | 'components'>): string {
  return `used by ${plural(f.count, 'value')} in ${plural(f.components.length, 'component')}`
}

/** The DOM id of a finding's note under field `id`; `related` keeps it unique when one rule names a field per pair. */
export function findingNoteId(id: string, f: Pick<Finding, 'rule' | 'related'>): string {
  return f.related ? `${id}-${f.rule}-${f.related}` : `${id}-${f.rule}`
}

/** The ids of a field's error, findings and help, for `aria-describedby`. */
export function describedBy(id: string, error: string | undefined, own: readonly Finding[]): string {
  return [error && `${id}-error`, ...own.map((f) => findingNoteId(id, f)), `${id}-help`].filter(Boolean).join(' ')
}
