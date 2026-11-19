import { describe, expect, it } from 'vitest'
import { emptyAnswers, Environment, TargetType, type Target } from './answers'
import { FIELD_IDS, fieldStage, openQuestions, targetIssues, type FieldId } from './openQuestions'
import { initialState, type AppState } from './reducer'
import { StageId } from './stageIds'

const VALID: Target = {
  ...emptyAnswers().target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  chartVersion: '10.16.0',
  environment: Environment.Development,
}

function stateOf(change: Partial<Target>, open: readonly FieldId[] = []): AppState {
  return { ...initialState, answers: { target: { ...VALID, ...change } }, open }
}

describe('openQuestions', () => {
  it.each<[string, Partial<Target>, FieldId[], [FieldId, boolean][]]>([
    ['nothing open', {}, [], []],
    ['unanswered only', { namespace: '' }, [], [['target.namespace', false]]],
    ['marked only', {}, ['target.namespace'], [['target.namespace', true]]],
    ['unanswered and marked, counted once', { namespace: '' }, ['target.namespace'], [['target.namespace', true]]],
    ['a marked cluster field on a file target', {}, ['target.context'], []],
    ['a marked cluster field on a cluster target', { type: TargetType.Cluster, context: 'dev' }, ['target.context'], [['target.context', true]]],
    [
      'several, in screen order',
      { environment: '', releaseName: '' },
      ['target.type', 'target.releaseName'],
      [
        ['target.type', true],
        ['target.releaseName', true],
        ['target.environment', false],
      ],
    ],
  ])('%s', (_name, change, open, want) => {
    expect(openQuestions(stateOf(change, open)).map((q) => [q.field, q.marked])).toEqual(want)
  })

  it('names the stage and label of each question', () => {
    expect(openQuestions(stateOf({ chartVersion: '' }))).toEqual([{ field: 'target.chartVersion', stage: StageId.Target, label: 'Chart version', marked: false }])
  })

  it.each(FIELD_IDS)('asks %s on the Target stage', (id) => {
    expect(fieldStage(id)).toBe(StageId.Target)
  })
})

describe('targetIssues', () => {
  it.each<[string, Partial<Target>, FieldId[], number, number]>([
    ['complete', {}, [], 0, 0],
    ['empty', emptyAnswers().target, [], 0, 5],
    ['invalid', { namespace: 'Bad_NS', releaseName: '1abc' }, [], 2, 0],
    ['marked empty, open and no error', { namespace: '' }, ['target.namespace'], 0, 1],
    ['marked with a format error, both', { namespace: 'Prod_NS' }, ['target.namespace'], 1, 1],
  ])('counts %s inputs', (_name, change, open, errors, openCount) => {
    expect(targetIssues(stateOf(change, open))).toEqual({ warnings: 0, errors, open: openCount })
  })
})
