import { Badge } from './Badge'
import {
  LEVELS, TRACKS, describeFilters, levelOf, minutesLabel, trackLabel, trackOf,
  type CatalogFilter,
} from '../lib/catalog'
import type { CatalogMeta as Meta } from '../types'

/** The track, level and time of a catalog item, as the badges on its row. */
export function MetaBadges({ item }: { item: Meta }) {
  const minutes = minutesLabel(item.minutes)
  return (
    <>
      <Badge variant="runtime">{trackLabel(trackOf(item))}</Badge>
      <Badge variant="category">{levelOf(item)}</Badge>
      {minutes && <span className="hint-text">{minutes}</span>}
    </>
  )
}

interface NoMatchesProps {
  noun: string
  search: string
  filter: CatalogFilter
  onClear: () => void
}

/** The empty state when the search text or the track and level pickers leave
 *  nothing to show, with a way back to the whole list. */
export function NoMatches({ noun, search, filter, onClear }: NoMatchesProps) {
  return (
    <>
      <div>No {noun} match {describeFilters(search, filter)}.</div>
      <div className="empty-hint">
        <button className="btn btn-sm" onClick={onClear}>Clear filters</button>
      </div>
    </>
  )
}

interface CatalogFiltersProps {
  value: CatalogFilter
  onChange: (next: CatalogFilter) => void
}

/** The track and level pickers above a catalog list. */
export function CatalogFilters({ value, onChange }: CatalogFiltersProps) {
  return (
    <>
      <select
        className="select"
        aria-label="Filter by track"
        value={value.track}
        onChange={e => onChange({ ...value, track: e.target.value })}
      >
        <option value="">All tracks</option>
        {TRACKS.map(t => <option key={t} value={t}>{trackLabel(t)}</option>)}
      </select>
      <select
        className="select"
        aria-label="Filter by level"
        value={value.level}
        onChange={e => onChange({ ...value, level: e.target.value })}
      >
        <option value="">All levels</option>
        {LEVELS.map(l => <option key={l} value={l}>{l}</option>)}
      </select>
    </>
  )
}
