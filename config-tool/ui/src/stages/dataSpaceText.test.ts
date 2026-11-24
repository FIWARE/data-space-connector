import { describe, expect, it } from 'vitest'
import { rowError } from './dataSpaceText'

describe('rowError', () => {
  it.each([
    ["[0].type must start with a letter, followed by letters, digits, '.', '_' and '-'", { row: 0, part: 'type', text: "Type 1 must start with a letter, followed by letters, digits, '.', '_' and '-'" }],
    ['[1].type repeats an earlier type', { row: 1, part: 'type', text: 'Type 2 repeats an earlier type' }],
    ['[2].description must be a string', { row: 2, part: 'description', text: 'Description 3 must be a string' }],
    ['[0] has a key other than type and description', { row: 0, part: undefined, text: 'Credential type 1 has a key other than type and description' }],
    ['must be a non-empty list of credential types', { text: 'must be a non-empty list of credential types' }],
  ])('locates %j', (error, want) => {
    expect(rowError(error)).toEqual(want)
  })
})
