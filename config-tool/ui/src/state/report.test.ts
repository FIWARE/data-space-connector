import { describe, expect, it } from 'vitest'
import { REPORT } from '../test/api'
import { carriedOverCount, formatAnswerValue, groupByTopLevel, topLevelKey, understoodCount, underivedAnswers } from './report'

describe('report', () => {
  it.each([
    ['keycloak', 'keycloak'],
    ['keycloak.ingress.host', 'keycloak'],
    ['extraManifests[0].kind', 'extraManifests'],
    ['"a.b".c', '"a.b"'],
    ['"say \\"hi\\"".x', '"say \\"hi\\""'],
    ['[2].name', '[2]'],
  ])('takes the top-level key of %s', (path, key) => {
    expect(topLevelKey(path)).toBe(key)
  })

  it('groups entries by top-level key in order of appearance', () => {
    const entry = (path: string) => ({ path, line: 1, value: '' })
    const groups = groupByTopLevel([entry('b.x'), entry('a.y'), entry('b.z')])
    expect(groups.map(([key, entries]) => [key, entries.map((e) => e.path)])).toEqual([
      ['b', ['b.x', 'b.z']],
      ['a', ['a.y']],
    ])
  })

  it('counts owned plus recognized as understood and unknown plus skipped as carried over', () => {
    expect([understoodCount(REPORT), carriedOverCount(REPORT)]).toEqual([2, 3])
  })

  it.each([
    [true, 'on'],
    [false, 'off'],
    ['example.org', 'example.org'],
    [3, '3'],
  ])('formats the answer %s as %s', (value, text) => {
    expect(formatAnswerValue(value)).toBe(text)
  })

  it('lists each underived marker answer once', () => {
    const [marker] = REPORT.markers
    if (!marker) throw new Error('REPORT has no marker')
    expect(underivedAnswers({ ...REPORT, markers: [marker, marker] })).toEqual(['did.value'])
  })
})
