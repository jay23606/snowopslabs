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
  iac: 'Infrastructure as code',
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

/** What the active filters say, as the end of "No scenarios match ...": the
 *  search text and the track and level, joined with "and". */
export function describeFilters(search: string, filter: CatalogFilter): string {
  const parts: string[] = []
  if (search) parts.push(`“${search}”`)
  if (filter.track) parts.push(`track ${filter.track}`)
  if (filter.level) parts.push(`level ${filter.level}`)
  return parts.join(' and ')
}
