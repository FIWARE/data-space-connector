/** The render Review's sections read: hand-set answers and findings of the current answers. */
import { useEffect, useRef, useState } from 'react'
import { toProfile } from '../api/profile'
import { fetchRender, type RenderResult } from '../api/render'
import { useStore } from '../state/context'
import { answerErrors } from '../state/foundations'

/** The render of the current answers; none while there is no answer or one is invalid. */
export function useReviewRender(): { result?: RenderResult; error?: string } {
  const { state } = useStore()
  const values = state.answers.values
  const sha = state.loaded?.sha256
  const latest = useRef(state)
  const [result, setResult] = useState<{ result?: RenderResult; error?: string }>({})
  const skip = Object.keys(values).length === 0 || Object.keys(answerErrors(values, state.foundations)).length > 0

  useEffect(() => {
    latest.current = state
  }, [state])

  useEffect(() => {
    if (skip) return
    const controller = new AbortController()
    fetchRender(toProfile(latest.current), sha, controller.signal).then(
      (r) => setResult({ result: r }),
      (err: Error) => controller.signal.aborted || setResult({ error: err.message }),
    )
    return () => controller.abort()
  }, [values, sha, skip])
  return skip ? {} : result
}
