/** Lists of load-report entries: the per-class tables and the overrides carried over verbatim. */
import type { LoadEntry, LoadReport } from '../api/load'
import { groupByTopLevel } from '../state/report'

/** One table row per entry: file line, path, value preview and what the tool made of it. */
export function EntryTable({ entries }: { entries: readonly LoadEntry[] }) {
  return (
    <table className="entries">
      <thead>
        <tr>
          <th>Line</th>
          <th>Path</th>
          <th>Value</th>
          <th>Note</th>
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <tr key={`${entry.line}:${entry.path}`}>
            <td>{entry.line}</td>
            <td>
              <code>{entry.path}</code>
            </td>
            <td>
              <code>{entry.value}</code>
            </td>
            <td>{[entry.answer && `answers ${entry.answer}`, entry.note].filter(Boolean).join(' · ')}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

/** Unknown leaves grouped by top-level key, then the skipped pairs; every group starts collapsed. */
export function Overrides({ report }: { report: LoadReport }) {
  if (report.unknown.length === 0 && report.skipped.length === 0) {
    return <p className="hint">Nothing: the tool reads every key of {report.source}.</p>
  }
  return (
    <div className="groups">
      {groupByTopLevel(report.unknown).map(([key, entries]) => (
        <details key={key}>
          <summary>
            <code>{key}</code> ({entries.length})
          </summary>
          <EntryTable entries={entries} />
        </details>
      ))}
      {report.skipped.length > 0 && (
        <details>
          <summary>Keys Helm ignores or cannot address ({report.skipped.length})</summary>
          <EntryTable entries={report.skipped} />
        </details>
      )}
    </div>
  )
}
