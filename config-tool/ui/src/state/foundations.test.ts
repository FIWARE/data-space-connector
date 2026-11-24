import { describe, expect, it } from 'vitest'
import type { FoundationType } from '../api/foundations'
import { FOUNDATIONS } from '../test/api'
import { answerErrors, checkAnswer, MAX_HOSTNAME_LENGTH, MSG_DOMAIN, MSG_HOSTNAME, MSG_PLAIN_LINE, validAnswers } from './foundations'

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
  ])('%s %j → %s', (type, value, message) => {
    expect(checkAnswer(type, value)).toBe(message)
  })
})

describe('answer validity', () => {
  const values = { 'org.displayName': 'Acme ', 'domain.base': 'example.org', 'host.other': 'Not Checked' }

  it('reports the format errors of declared answers only', () => {
    expect(answerErrors(values, FOUNDATIONS.foundations)).toEqual({ 'org.displayName': MSG_PLAIN_LINE })
  })

  it.each([
    ['without the foundations', undefined, values],
    ['with the foundations', FOUNDATIONS.foundations, { 'domain.base': 'example.org', 'host.other': 'Not Checked' }],
  ])('keeps an invalid answer out of what is sent %s', (_, foundations, sent) => {
    expect(validAnswers(values, foundations)).toEqual(sent)
  })
})
