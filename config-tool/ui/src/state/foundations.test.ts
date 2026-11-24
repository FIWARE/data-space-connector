import { describe, expect, it } from 'vitest'
import type { Foundation, FoundationOption, FoundationType } from '../api/foundations'
import { FOUNDATIONS } from '../test/api'
import {
  answerErrors,
  checkAnswer,
  MAX_HOSTNAME_LENGTH,
  MSG_DID,
  MSG_DOMAIN,
  MSG_HOSTNAME,
  MSG_PLAIN_LINE,
  msgChoice,
  validAnswers,
} from './foundations'

const METHODS: FoundationOption[] = [
  { value: 'did:web', label: 'Web', note: '' },
  { value: 'did:key', label: 'Key', note: 'Meant for local development.' },
]

describe('checkAnswer', () => {
  // Mirrors TestFoundationTypeCheck in Go, so both sides refuse the same answers.
  it.each<[FoundationType, string, string | undefined]>([
    ['string', 'Acme Corp', undefined],
    ['string', 'Müller & Söhne', undefined],
    ['string', '', MSG_PLAIN_LINE],
    ['string', 'Acme ', MSG_PLAIN_LINE],
    ['string', ' Acme', MSG_PLAIN_LINE],
    ['string', 'Müller "Sons"', MSG_PLAIN_LINE],
    ['string', '<b>', MSG_PLAIN_LINE],
    ['string', 'a\\b', MSG_PLAIN_LINE],
    ['string', 'two\nlines', MSG_PLAIN_LINE],
    ['string', 'tab\there', MSG_PLAIN_LINE],
    ['hostname', 'localhost', undefined],
    ['hostname', 'keycloak.example.org', undefined],
    ['hostname', 'Example.org', MSG_HOSTNAME],
    ['hostname', '-a.org', MSG_HOSTNAME],
    ['hostname', 'a..org', MSG_HOSTNAME],
    ['hostname', `${'a'.repeat(MAX_HOSTNAME_LENGTH - 4)}.org`, undefined],
    ['hostname', `${'a'.repeat(MAX_HOSTNAME_LENGTH)}.org`, MSG_HOSTNAME],
    ['domain', 'example.org', undefined],
    ['domain', '127.0.0.1.nip.io', undefined],
    ['domain', 'localhost', MSG_DOMAIN],
    ['domain', 'Example.org', MSG_HOSTNAME],
    ['domain', 'https://example.org', MSG_HOSTNAME],
    ['did', 'did:web:example.org', undefined],
    ['did', 'did:web:example.org%3A8443:user:alice', undefined],
    ['did', 'did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK', undefined],
    ['did', 'did:web:', MSG_DID],
    ['did', 'did:web', MSG_DID],
    ['did', 'DID:web:example.org', MSG_DID],
    ['did', 'did:Web:example.org', MSG_DID],
    ['did', 'did:web:example.org/path', MSG_DID],
    ['did', 'did:web:a"b', MSG_DID],
    ['did', 'did:web:a b', MSG_DID],
  ])('%s %j → %s', (type, value, message) => {
    expect(checkAnswer(type, value)).toBe(message)
  })

  // Mirrors TestCompiledFoundationCheck in Go.
  it.each<[string, string | undefined]>([
    ['did:web', undefined],
    ['did:key', undefined],
    ['did:ebsi', 'must be one of did:web, did:key'],
  ])('choice %j → %s', (value, message) => {
    expect(checkAnswer('choice', value, METHODS)).toBe(message)
  })

  it('refuses every choice when no options are known', () => {
    expect(checkAnswer('choice', 'did:web')).toBe(msgChoice([]))
  })
})

describe('answer validity', () => {
  const values = { 'org.displayName': 'Acme ', 'domain.base': 'example.org', 'host.other': 'Not Checked' }

  it('reports the format errors of declared answers only', () => {
    expect(answerErrors(values, FOUNDATIONS.foundations)).toEqual({ 'org.displayName': MSG_PLAIN_LINE })
  })

  it('checks a choice against its options', () => {
    const method: Foundation = {
      stage: 'foundations',
      id: 'did.method',
      type: 'choice',
      default: 'did:web',
      options: METHODS,
      label: 'DID method',
      help: '',
      doc: '',
      targets: [],
      count: 0,
      components: [],
    }
    expect(answerErrors({ 'did.method': 'did:ebsi' }, [method])).toEqual({ 'did.method': msgChoice(METHODS) })
    expect(answerErrors({ 'did.method': 'did:key' }, [method])).toEqual({})
  })

  it.each([
    ['without the foundations', undefined, values],
    ['with the foundations', FOUNDATIONS.foundations, { 'domain.base': 'example.org', 'host.other': 'Not Checked' }],
  ])('keeps an invalid answer out of what is sent %s', (_, foundations, sent) => {
    expect(validAnswers(values, foundations)).toEqual(sent)
  })
})
