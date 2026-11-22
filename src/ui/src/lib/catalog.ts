// The browsing metadata every catalog item carries, and the filter the catalog
// views apply to it. The values match pkg/scenario/meta.go.

import type { CatalogMeta } from '../types'

/** A catalog filter. An empty string means "any". */
export interface CatalogFilter {
  track: string
  level: string
}

export const NO_FILTER: CatalogFilter = { track: '', level: '' }

export const TRACKS = ['kubernetes', 'cicd', 'iac'] as const
export const LEVELS = ['beginner', 'intermediate', 'advanced'] as const

const DEFAULT_TRACK = 'kubernetes'
const DEFAULT_LEVEL = 'intermediate'

const TRACK_LABELS: Record<string, string> = {
  kubernetes: 'Kubernetes',
  cicd: 'CI/CD',
  iac: 'IaC',
}

const LEVEL_LABELS: Record<string, string> = {
  beginner: 'Beginner',
  intermediate: 'Intermediate',
  advanced: 'Advanced',
}

/** The item's track; an item that states none is a Kubernetes item. */
export function trackOf(item: CatalogMeta): string {
  return item.track || DEFAULT_TRACK
}

/** The item's level; an item that states none is intermediate. */
export function levelOf(item: CatalogMeta): string {
  return item.level || DEFAULT_LEVEL
}

export function trackLabel(track: string): string {
  return TRACK_LABELS[track] ?? track
}

export function levelLabel(level: string): string {
  return LEVEL_LABELS[level] ?? level
}

export function matchesMeta(item: CatalogMeta, filter: CatalogFilter): boolean {
  return (!filter.track || trackOf(item) === filter.track) &&
    (!filter.level || levelOf(item) === filter.level)
}

export function metaFilterActive(filter: CatalogFilter): boolean {
  return filter.track !== '' || filter.level !== ''
}

/** "~20 min", or '' when the item states no time. */
export function minutesLabel(minutes?: number): string {
  return minutes && minutes > 0 ? `~${minutes} min` : ''
}

/** The empty-state sentence. Search text reads "No scenarios match “x” on the
 *  CI/CD track."; pickers alone read "No scenarios on the CI/CD track at
 *  Advanced level yet." */
export function emptyMessage(noun: string, search: string, filter: CatalogFilter): string {
  const where: string[] = []
  if (filter.track) where.push(`on the ${trackLabel(filter.track)} track`)
  if (filter.level) where.push(`at ${levelLabel(filter.level)} level`)
  if (search) return `No ${noun} match “${search}”${where.length ? ' ' + where.join(' ') : ''}.`
  return `No ${noun} ${where.join(' ')} yet.`
}
