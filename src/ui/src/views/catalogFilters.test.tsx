// SPDX-License-Identifier: Apache-2.0
//
// The track and level filters on the four catalog views: each row shows its
// track, level and time, the pickers narrow the list, and a combination that
// matches nothing says so and offers a way back.
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { page, serveContract, type ContractServer } from '../test/contract'
import { Challenges } from './Challenges'
import { Incidents } from './Incidents'
import { Learn } from './Learn'
import { Scenarios } from './Scenarios'
import type { CatalogMeta } from '../types'

afterEach(() => {
  vi.unstubAllGlobals()
})

// Three items per view: a beginner CI/CD one with a time, an advanced IaC one,
// and one that states neither track nor level, which is a Kubernetes item at
// the intermediate level.
const cicd: CatalogMeta = { track: 'cicd', level: 'beginner', minutes: 20 }
const iac: CatalogMeta = { track: 'iac', level: 'advanced', minutes: 90 }
const plain: CatalogMeta = {}

interface ViewCase {
  name: string
  noun: string // the word in "No <noun> match ..."
  title: RegExp // the card title with its count
  serve: (server: ContractServer) => void
  render: () => ReactElement
  names: [string, string, string] // the item names shown, cicd / iac / plain
}

const scenario = (name: string, meta: CatalogMeta) =>
  ({ name, displayName: name, description: `about ${name}`, category: 'delivery', active: false, ...meta })

const fault = (name: string, meta: CatalogMeta) =>
  ({ name, displayName: name, description: `about ${name}`, verified: true, category: 'workload', severity: 'low', ...meta })

const challenge = (name: string, meta: CatalogMeta) =>
  ({ name, displayName: name, description: `about ${name}`, category: 'workload', parTime: '10m', ...meta })

const path = (name: string, meta: CatalogMeta) =>
  ({ name, displayName: name, description: `about ${name}`, tags: [], estimatedMinutes: 0, moduleCount: 2, completedCount: 0, ...meta })

function wrap(el: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{el}</QueryClientProvider>
}

const cases: ViewCase[] = [
  {
    name: 'Scenarios', noun: 'scenarios', title: /^Scenarios \(/,
    names: ['ci-cache', 'iac-state', 'kube-basics'],
    serve: s => s.override('GET', '/scenarios', 200, page([scenario('ci-cache', cicd), scenario('iac-state', iac), scenario('kube-basics', plain)])),
    render: () => wrap(<Scenarios notify={vi.fn()} requestConfirm={vi.fn()} />),
  },
  {
    name: 'Incidents', noun: 'incidents', title: /^Incident library \(/,
    names: ['stale-cache', 'locked-state', 'bad-config'],
    serve: s => s.override('GET', '/incidents', 200, { faults: [fault('stale-cache', cicd), fault('locked-state', iac), fault('bad-config', plain)], active: null }),
    render: () => wrap(<Incidents notify={vi.fn()} requestConfirm={vi.fn()} />),
  },
  {
    name: 'Challenges', noun: 'challenges', title: /^Challenges \(/,
    names: ['cache-bust', 'unlock-state', 'fix-config'],
    serve: s => s.override('GET', '/challenges', 200, page([challenge('cache-bust', cicd), challenge('unlock-state', iac), challenge('fix-config', plain)])),
    render: () => wrap(<Challenges notify={vi.fn()} requestConfirm={vi.fn()} />),
  },
  {
    name: 'Learn', noun: 'learning paths', title: /^Learning Paths \(/,
    names: ['ci-basics', 'iac-basics', 'kube-basics'],
    serve: s => s.override('GET', '/learn/paths', 200, page([path('ci-basics', cicd), path('iac-basics', iac), path('kube-basics', plain)])),
    render: () => wrap(<Learn notify={vi.fn()} requestConfirm={vi.fn()} />),
  },
]

describe.each(cases)('$name catalog filters', tc => {
  async function open() {
    const server = serveContract()
    tc.serve(server)
    render(tc.render())
    const [first] = tc.names
    await screen.findByText(first)
    return userEvent.setup()
  }
  const title = () => screen.getByText(tc.title).textContent

  it('shows each item with its track, level and time', async () => {
    await open()
    const row = screen.getByText(tc.names[0]).closest('.scenario-row') as HTMLElement
    expect(within(row).getByText('CI/CD')).toBeInTheDocument()
    expect(within(row).getByText('Beginner')).toBeInTheDocument()
    expect(within(row).getByText('~20 min')).toBeInTheDocument()

    // An item that states nothing shows the defaults and no time.
    const plainRow = screen.getByText(tc.names[2]).closest('.scenario-row') as HTMLElement
    expect(within(plainRow).getByText('Kubernetes')).toBeInTheDocument()
    expect(within(plainRow).getByText('Intermediate')).toBeInTheDocument()
    expect(within(plainRow).queryByText(/min$/)).toBeNull()
  })

  it('narrows the list by track, then by level', async () => {
    const user = await open()
    expect(title()).toMatch(/\(3\)$/)

    await user.selectOptions(screen.getByLabelText('Filter by track'), 'cicd')
    expect(screen.getByText(tc.names[0])).toBeInTheDocument()
    expect(screen.queryByText(tc.names[1])).toBeNull()
    expect(screen.queryByText(tc.names[2])).toBeNull()
    expect(title()).toMatch(/\(1 of 3\)$/)

    await user.selectOptions(screen.getByLabelText('Filter by track'), '')
    await user.selectOptions(screen.getByLabelText('Filter by level'), 'intermediate')
    expect(screen.getByText(tc.names[2])).toBeInTheDocument()
    expect(screen.queryByText(tc.names[0])).toBeNull()
  })

  it('says when the filters match nothing and clears them on request', async () => {
    const user = await open()
    await user.selectOptions(screen.getByLabelText('Filter by track'), 'cicd')
    await user.selectOptions(screen.getByLabelText('Filter by level'), 'advanced')

    expect(screen.getByText(`No ${tc.noun} on the CI/CD track at Advanced level yet.`)).toBeInTheDocument()
    expect(screen.queryByText(tc.names[0])).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Clear filters' }))
    for (const n of tc.names) expect(screen.getByText(n)).toBeInTheDocument()
    expect(screen.getByLabelText('Filter by track')).toHaveValue('')
    expect(screen.getByLabelText('Filter by level')).toHaveValue('')
  })
})

describe('text search with the pickers', () => {
  it('names both in the empty state and clears both', async () => {
    const server = serveContract()
    server.override('GET', '/scenarios', 200, page([
      scenario('ci-cache', cicd), scenario('iac-state', iac), scenario('kube-basics', plain), scenario('mesh', plain),
    ]))
    render(wrap(<Scenarios notify={vi.fn()} requestConfirm={vi.fn()} />))
    await screen.findByText('ci-cache')
    const user = userEvent.setup()

    await user.selectOptions(screen.getByLabelText('Filter by track'), 'cicd')
    await user.type(screen.getByLabelText('Filter scenarios'), 'mesh')
    expect(screen.getByText('No scenarios match “mesh” on the CI/CD track.')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(screen.getByLabelText('Filter scenarios')).toHaveValue('')
    expect(screen.getByText('mesh')).toBeInTheDocument()
  })

  it('still names only the text when no picker is set', async () => {
    const server = serveContract()
    server.override('GET', '/incidents', 200, { faults: [fault('a', plain), fault('b', plain)], active: null })
    render(wrap(<Incidents notify={vi.fn()} requestConfirm={vi.fn()} />))
    await screen.findByText('a')
    await userEvent.setup().type(screen.getByLabelText('Filter incidents'), 'zzz')
    expect(screen.getByText('No incidents match “zzz”.')).toBeInTheDocument()
  })
})

describe('path time', () => {
  it('shows estimatedMinutes when the server sends no minutes', async () => {
    const server = serveContract()
    server.override('GET', '/learn/paths', 200, page([path('old', { estimatedMinutes: 45 } as CatalogMeta)]))
    render(wrap(<Learn notify={vi.fn()} requestConfirm={vi.fn()} />))
    expect(await screen.findByText('~45 min')).toBeInTheDocument()
  })
})
