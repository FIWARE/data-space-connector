import { describe, expect, it } from 'vitest'
import { emptyAnswers, Environment, TargetType, type Target } from './answers'
import { initialState } from './reducer'
import { MAX_NAMESPACE_LENGTH, MAX_RELEASE_NAME_LENGTH, MSG_REQUIRED, TargetField, targetIssues, validateTarget } from './target'

const VALID: Target = {
  ...emptyAnswers().target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  chartVersion: '10.16.0',
  environment: Environment.Development,
}

describe('validateTarget', () => {
  it.each<[string, Partial<Target>, [TargetField, string][]]>([
    ['a valid file target', {}, []],
    ['a cluster target with a context', { type: TargetType.Cluster, context: 'dev' }, []],
    ['a cluster target without a context', { type: TargetType.Cluster }, [[TargetField.Context, MSG_REQUIRED]]],
    ['a file target ignores the kept context', { context: 'dev' }, []],
    ['an unanswered type', { type: '' }, [[TargetField.Type, MSG_REQUIRED]]],
    ['an unanswered environment', { environment: '' }, [[TargetField.Environment, MSG_REQUIRED]]],
    ['an unknown environment', { environment: 'staging' as Environment }, [[TargetField.Environment, 'must be "development" or "production"']]],
    ['an empty chart version', { chartVersion: '' }, [[TargetField.ChartVersion, MSG_REQUIRED]]],
    ['a namespace at the limit', { namespace: 'a'.repeat(MAX_NAMESPACE_LENGTH) }, []],
    ['a namespace over the limit', { namespace: 'a'.repeat(MAX_NAMESPACE_LENGTH + 1) }, [[TargetField.Namespace, `must be at most ${MAX_NAMESPACE_LENGTH} characters`]]],
    ['a namespace with a leading digit', { namespace: '1provider' }, []],
    ['a namespace with upper case', { namespace: 'Provider' }, [[TargetField.Namespace, expect.stringContaining('lowercase')]]],
    ['a release name at the limit', { releaseName: 'a'.repeat(MAX_RELEASE_NAME_LENGTH) }, []],
    ['a release name over the limit', { releaseName: 'a'.repeat(MAX_RELEASE_NAME_LENGTH + 1) }, [[TargetField.ReleaseName, `must be at most ${MAX_RELEASE_NAME_LENGTH} characters`]]],
    ['a release name with a leading digit', { releaseName: '1provider' }, [[TargetField.ReleaseName, expect.stringContaining('start with a letter')]]],
  ])('%s', (_name, change, want) => {
    expect(validateTarget({ ...VALID, ...change }).map((e) => [e.field, e.message])).toEqual(want)
  })
})

describe('targetIssues', () => {
  it.each<[string, Partial<Target>, number, number]>([
    ['complete', {}, 0, 0],
    ['empty', emptyAnswers().target, 0, 5],
    ['invalid', { namespace: 'Bad_NS', releaseName: '1abc' }, 2, 0],
  ])('counts %s inputs', (_name, change, errors, open) => {
    const state = { ...initialState, answers: { target: { ...VALID, ...change } } }
    expect(targetIssues(state)).toEqual({ warnings: 0, errors, open })
  })
})
