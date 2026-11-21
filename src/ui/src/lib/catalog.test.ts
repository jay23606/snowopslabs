// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest'
import {
  LEVELS, NO_FILTER, TRACKS, describeFilters, levelOf, matchesMeta, metaFilterActive,
  minutesLabel, trackLabel, trackOf,
} from './catalog'

describe('defaults', () => {
  it('treats a missing track as kubernetes and a missing level as intermediate', () => {
    expect(trackOf({})).toBe('kubernetes')
    expect(levelOf({})).toBe('intermediate')
    expect(trackOf({ track: 'cicd' })).toBe('cicd')
    expect(levelOf({ level: 'advanced' })).toBe('advanced')
  })
})

describe('matchesMeta', () => {
  it.each([
    ['no filter keeps everything', {}, NO_FILTER, true],
    ['track hit', { track: 'cicd' }, { track: 'cicd', level: '' }, true],
    ['track miss', { track: 'iac' }, { track: 'cicd', level: '' }, false],
    ['level miss', { level: 'beginner' }, { track: '', level: 'advanced' }, false],
    ['both must hold', { track: 'cicd', level: 'beginner' }, { track: 'cicd', level: 'advanced' }, false],
    ['a missing track is kubernetes', {}, { track: 'kubernetes', level: '' }, true],
    ['a missing level is intermediate', {}, { track: '', level: 'intermediate' }, true],
    ['a missing track is not cicd', {}, { track: 'cicd', level: '' }, false],
  ])('%s', (_name, item, filter, want) => {
    expect(matchesMeta(item, filter)).toBe(want)
  })
})

describe('metaFilterActive', () => {
  it('is true when either picker is set', () => {
    expect(metaFilterActive(NO_FILTER)).toBe(false)
    expect(metaFilterActive({ track: 'iac', level: '' })).toBe(true)
    expect(metaFilterActive({ track: '', level: 'beginner' })).toBe(true)
  })
})

describe('minutesLabel', () => {
  it('prints the time, or nothing when none is stated', () => {
    expect(minutesLabel(20)).toBe('~20 min')
    expect(minutesLabel(0)).toBe('')
    expect(minutesLabel(undefined)).toBe('')
  })
})

describe('trackLabel', () => {
  it('names the known tracks and passes an unknown one through', () => {
    expect(trackLabel('cicd')).toBe('CI/CD')
    expect(trackLabel('aws')).toBe('aws')
  })
})

describe('describeFilters', () => {
  it('joins the search text, track and level', () => {
    expect(describeFilters('', { track: '', level: '' })).toBe('')
    expect(describeFilters('mesh', NO_FILTER)).toBe('“mesh”')
    expect(describeFilters('', { track: 'cicd', level: 'beginner' })).toBe('track cicd and level beginner')
    expect(describeFilters('x', { track: 'iac', level: '' })).toBe('“x” and track iac')
  })
})

describe('the lists', () => {
  it('match the values the server validates', () => {
    expect([...TRACKS]).toEqual(['kubernetes', 'cicd', 'iac'])
    expect([...LEVELS]).toEqual(['beginner', 'intermediate', 'advanced'])
  })
})
