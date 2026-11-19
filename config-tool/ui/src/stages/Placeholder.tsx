import type { ReactNode } from 'react'
import type { StageId } from '../state/stageIds'
import { DerivedAnswers } from './DerivedAnswers'

/** Stands in for a stage that a later epic implements, so jumping there never fails; lists a loaded file's answers for `stageId`. */
export function Placeholder({ label, comingIn, stageId, children }: { label: string; comingIn?: string; stageId?: StageId; children?: ReactNode }) {
  return (
    <section>
      <h1>{label} — not available yet</h1>
      {comingIn && <p className="lede">This stage comes with {comingIn}.</p>}
      {children}
      {stageId && <DerivedAnswers stage={stageId} />}
    </section>
  )
}
