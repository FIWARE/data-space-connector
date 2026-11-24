import { describe, expect, it } from 'vitest'
import type { CredentialType, Foundation, FoundationOption, FoundationType } from '../api/foundations'
import type { AnswerValue } from './answers'
import { DID_FOUNDATIONS, DID_METHOD, FOUNDATIONS } from '../test/api'
import {
  answerErrors,
  checkAnswer,
  dropMismatched,
  MAX_CREDENTIAL_TYPES,
  MAX_HOSTNAME_LENGTH,
  MAX_URL_LENGTH,
  MSG_BOOL,
  MSG_CREDENTIAL_TYPES,
  MSG_CREDENTIAL_TYPES_LENGTH,
  MSG_DID,
  MSG_DOMAIN,
  MSG_HOSTNAME,
  MSG_PLAIN_LINE,
  MSG_STRING,
  MSG_URL,
  MSG_URL_LENGTH,
  msgChoice,
  msgPrefix,
  resolvedAnswer,
  textAnswer,
  validAnswers,
} from './foundations'

/** `n` distinct credential types. */
function manyTypes(n: number): CredentialType[] {
  return Array.from({ length: n }, (_, i) => ({ type: `Credential${i}` }))
}

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
    ['did', 'did:web:example.org:', MSG_DID],
    ['did', 'did:web::', MSG_DID],
    ['did', 'did:web:a%', MSG_DID],
    ['did', 'did:web:a%zz', MSG_DID],
  ])('%s %j → %s', (type, value, message) => {
    expect(checkAnswer(type, value)).toBe(message)
  })

  // Mirrors the url, bool and credentialTypes rows of TestFoundationTypeCheck in Go.
  it.each<[FoundationType, AnswerValue, string | undefined]>([
    ['url', 'https://tir.127.0.0.1.nip.io/', undefined],
    ['url', 'http://tir.example.org:8080/v4/issuers?page=1', undefined],
    ['url', 'https://[::1]:8443', undefined],
    ['url', `https://tir.example.org/${'a'.repeat(MAX_URL_LENGTH)}`, MSG_URL_LENGTH],
    ['url', `https://tir.example.org/${'ü'.repeat(1100)}`, MSG_URL_LENGTH],
    ['url', 'https://tür.example.org/', undefined],
    ['url', "https://a!$&'()*+,;=b.example.org/", undefined],
    ['url', 'https://a%C3%BCb.example.org/', undefined],
    ['url', 'https://a{b}.example.org/', MSG_URL],
    ['url', 'https://a|b.example.org/', MSG_URL],
    ['url', 'https://a^b.example.org/', MSG_URL],
    ['url', 'https://a`b.example.org/', MSG_URL],
    ['url', 'https://a%41b.example.org/', MSG_URL],
    ['url', 'https://tir.example.org:x/', MSG_URL],
    ['url', 'tir.example.org', MSG_URL],
    ['url', '/v4/issuers', MSG_URL],
    ['url', 'ftp://tir.example.org/', MSG_URL],
    ['url', 'https://', MSG_URL],
    ['url', 'https:tir.example.org', MSG_URL],
    ['url', 'https://user:pass@tir.example.org/', MSG_URL],
    ['url', 'https://@tir.example.org/', MSG_URL],
    ['url', 'https://tir.example.org/#issuers', MSG_URL],
    ['url', 'https://tir.example.org/a b', MSG_URL],
    ['url', 'https://tir.example.org/<x>', MSG_URL],
    ['url', ' https://tir.example.org/', MSG_URL],
    ['url', true, MSG_STRING],
    ['string', ['Acme'] as unknown as AnswerValue, MSG_STRING],
    ['bool', true, undefined],
    ['bool', false, undefined],
    ['bool', 'true', MSG_BOOL],
    ['credentialTypes', [{ type: 'UserCredential' }], undefined],
    [
      'credentialTypes',
      [{ type: 'UserCredential', description: 'Employees of a participant' }, { type: 'LegalParticipant' }, { type: 'eu.europa.ec.eudi.pid_1' }],
      undefined,
    ],
    ['credentialTypes', [], MSG_CREDENTIAL_TYPES],
    ['credentialTypes', 'UserCredential', MSG_CREDENTIAL_TYPES],
    ['credentialTypes', ['UserCredential'] as unknown as AnswerValue, '[0] must be a mapping with type and an optional description'],
    ['credentialTypes', [{ description: 'no type' }] as unknown as AnswerValue, "[0].type must start with a letter, followed by letters, digits, '.', '_' and '-'"],
    ['credentialTypes', [{ type: '1Credential' }], "[0].type must start with a letter, followed by letters, digits, '.', '_' and '-'"],
    ['credentialTypes', [{ type: 'User Credential' }], "[0].type must start with a letter, followed by letters, digits, '.', '_' and '-'"],
    ['credentialTypes', [{ type: 'A', format: 'jwt_vc' }] as unknown as AnswerValue, '[0] has a key other than type and description'],
    ['credentialTypes', [{ type: 'A', description: ' padded' }], `[0].description ${MSG_PLAIN_LINE}`],
    ['credentialTypes', [{ type: 'A', description: 1 }] as unknown as AnswerValue, `[0].description ${MSG_STRING}`],
    ['credentialTypes', [{ type: 'A' }, { type: 'A' }], '[1].type repeats an earlier type'],
    ['credentialTypes', manyTypes(MAX_CREDENTIAL_TYPES), undefined],
    ['credentialTypes', manyTypes(MAX_CREDENTIAL_TYPES + 1), MSG_CREDENTIAL_TYPES_LENGTH],
  ])('%s %j → %s', (type, value, message) => {
    expect(checkAnswer(type, value)).toBe(message)
  })

  it.each<[AnswerValue | undefined, string | undefined]>([
    ['Acme', 'Acme'],
    [true, undefined],
    [[{ type: 'A' }], undefined],
    [undefined, undefined],
  ])('textAnswer(%j) → %j', (value, text) => {
    expect(textAnswer(value)).toBe(text)
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
    const method: Foundation = { ...DID_METHOD, options: METHODS }
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

describe('resolution', () => {
  const all = DID_FOUNDATIONS.foundations

  it.each([
    ['a choice falls back to its constant default', 'did.method', {}, 'did:web'],
    ['a did:web DID derives from the domain', 'did.value', { 'domain.base': 'example.org' }, 'did:web:example.org'],
    ['a derived DID waits for the domain', 'did.value', {}, undefined],
    ['a did:key DID has no default', 'did.value', { 'domain.base': 'example.org', 'did.method': 'did:key' }, undefined],
    ['a given answer wins over the default', 'did.value', { 'domain.base': 'example.org', 'did.value': 'did:web:other.org' }, 'did:web:other.org'],
  ])('%s', (_, id, values, want) => {
    expect(resolvedAnswer(id, values, all)).toEqual(want)
  })

  it.each([
    ['refuses a DID of another method', { 'did.method': 'did:key', 'did.value': 'did:web:example.org' }, { 'did.value': msgPrefix('did:key:') }],
    ['accepts a DID of the chosen method', { 'did.method': 'did:key', 'did.value': 'did:key:z6Mk' }, {}],
    ['checks the prefix against the default method', { 'did.value': 'did:key:z6Mk' }, { 'did.value': msgPrefix('did:web:') }],
  ])('%s', (_, values, want) => {
    expect(answerErrors(values, all)).toEqual(want)
  })

  it.each([
    ['drops a DID the new method no longer matches', { 'did.method': 'did:key', 'did.value': 'did:web:example.org' }, { 'did.method': 'did:key' }],
    ['keeps a DID that still matches', { 'did.method': 'did:key', 'did.value': 'did:key:z6Mk' }, { 'did.method': 'did:key', 'did.value': 'did:key:z6Mk' }],
  ])('%s', (_, values, want) => {
    expect(dropMismatched(values, 'did.method', all)).toEqual(want)
  })

  it('keeps a mismatched DID when the change is the DID itself', () => {
    const values = { 'did.value': 'did:key:z6Mk' }
    expect(dropMismatched(values, 'did.value', all)).toBe(values)
  })
})
