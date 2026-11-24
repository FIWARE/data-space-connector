/** Text the Foundations stage derives from bundle data, apart from the component for fast refresh. */
import type { Foundation } from '../api/foundations'

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

/** The fan-out button text, e.g. "used by 3 values in 2 components". */
export function usedByText(f: Pick<Foundation, 'count' | 'components'>): string {
  return `used by ${plural(f.count, 'value')} in ${plural(f.components.length, 'component')}`
}
