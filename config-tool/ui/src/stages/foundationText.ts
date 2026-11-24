/** Text the Foundations stage derives from bundle data, apart from the component for fast refresh. */
import type { Foundation } from '../api/foundations'

/** A derived answer's default refers to other answers as `{{ .id }}`, the only form the shipped bundles use. */
const ANSWER_REFERENCE = /\{\{\s*\.([\w.]+)\s*\}\}/g

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

/** The fan-out button text, e.g. "used by 3 values in 2 components". */
export function usedByText(f: Pick<Foundation, 'count' | 'components'>): string {
  return `used by ${plural(f.count, 'value')} in ${plural(f.components.length, 'component')}`
}

/** A derived answer: the user's answer when given, else its default with the references filled in; undefined while
 *  a referenced answer is missing. */
export function derivedValue(f: Foundation, values: Record<string, string>): string | undefined {
  if (values[f.id] !== undefined) return values[f.id]
  let missing = false
  const value = f.default.replace(ANSWER_REFERENCE, (_, id: string) => {
    missing ||= values[id] === undefined
    return values[id] ?? ''
  })
  return missing ? undefined : value
}

