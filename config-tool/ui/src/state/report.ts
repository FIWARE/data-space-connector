/** Pure views over a load report: counts, grouping and display strings. */
import type { DerivedAnswer, LoadEntry, LoadReport } from '../api/load'

const QUOTE = '"'
const ESCAPE = '\\'
// Delimiters that end an unquoted path segment, as roundtrip.Path writes them.
const SEGMENT_END = /[.[]/

/** Leaves the tool reads: owned plus recognized. */
export function understoodCount(report: LoadReport): number {
  return report.counts.owned + report.counts.recognized
}

/** What is kept verbatim: unknown leaves plus skipped pairs. */
export function carriedOverCount(report: LoadReport): number {
  return report.counts.unknown + report.counts.skipped
}

/** The first segment of a roundtrip path, quoted as the path writes it. */
export function topLevelKey(path: string): string {
  if (path.startsWith(QUOTE)) {
    for (let i = 1; i < path.length; i++) {
      if (path[i] === ESCAPE) i++
      else if (path[i] === QUOTE) return path.slice(0, i + 1)
    }
    return path
  }
  if (path.startsWith('[')) return path.slice(0, path.indexOf(']') + 1) || path
  const end = path.search(SEGMENT_END)
  return end < 0 ? path : path.slice(0, end)
}

/** Entries grouped by top-level key, groups in order of first appearance. */
export function groupByTopLevel(entries: readonly LoadEntry[]): [string, LoadEntry[]][] {
  const groups = new Map<string, LoadEntry[]>()
  for (const entry of entries) {
    const key = topLevelKey(entry.path)
    const group = groups.get(key)
    if (group) group.push(entry)
    else groups.set(key, [entry])
  }
  return [...groups]
}

/** A toggle reads on/off; any other value as written. */
export function formatAnswerValue(value: DerivedAnswer['value']): string {
  if (typeof value === 'boolean') return value ? 'on' : 'off'
  return String(value)
}

/** Answer ids that markers name but no owned leaf derived, each once. */
export function underivedAnswers(report: LoadReport): string[] {
  return [...new Set(report.markers.flatMap((marker) => marker.underived))]
}
