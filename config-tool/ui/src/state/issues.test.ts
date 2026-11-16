import { describe, expect, it } from 'vitest'
import { STAGES, type StageDef } from '../stages'
import { NO_ISSUES, StageStatus, stageStatus, type Issues } from './issues'
import { initialState } from './reducer'
import { StageId } from './stageIds'

const TARGET = STAGES[0] as StageDef
const ON_ROLE = { ...initialState, entered: true, stage: StageId.Role }

describe('stageStatus', () => {
  it.each<[string, Issues | undefined, boolean, StageStatus]>([
    ['visited without a selector', undefined, true, StageStatus.Visited],
    ['not visited without a selector', undefined, false, StageStatus.None],
    ['visited, selector without issues', NO_ISSUES, true, StageStatus.Complete],
    ['not visited, selector without issues', NO_ISSUES, false, StageStatus.None],
    ['visited with open questions', { warnings: 0, errors: 0, open: 1 }, true, StageStatus.Open],
    ['visited with warnings', { warnings: 1, errors: 0, open: 1 }, true, StageStatus.Warning],
    ['visited with errors', { warnings: 1, errors: 1, open: 1 }, true, StageStatus.Error],
  ])('%s', (_name, issues, visited, expected) => {
    const stage: StageDef = { ...TARGET, selector: issues ? () => issues : undefined }
    const state = { ...ON_ROLE, visited: visited ? [TARGET.id] : [] }
    expect(stageStatus(state, stage)).toBe(expected)
  })

  it('marks the current stage current', () => {
    expect(stageStatus({ ...ON_ROLE, stage: TARGET.id }, { ...TARGET, selector: () => NO_ISSUES })).toBe(StageStatus.Current)
  })
})
