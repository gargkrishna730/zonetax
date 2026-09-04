import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { KpiPanel } from './KpiPanel'
import type { CostsResponse } from '../types'

function makeCosts(overrides: Partial<CostsResponse> = {}): CostsResponse {
  return {
    cloud: 'aws',
    region: 'us-east-1',
    updated_at: new Date().toISOString(),
    server_time_utc: new Date().toISOString(),
    collector_started_at_utc: new Date(Date.now() - 3600_000).toISOString(),
    scrape_interval_seconds: 15,
    entries: [],
    totals: {
      cross_az_cost_usd: 1.23,
      cross_az_gb: 45.6,
      same_az_gb: 12.3,
      price_per_gb_usd: 0.02,
      price_per_gb_direction_usd: 0.01,
    },
    ...overrides,
  }
}

describe('KpiPanel', () => {
  it('labels the cross-AZ spend KPI as a session metric, not a calendar-day metric', () => {
    render(<KpiPanel costs={makeCosts()} fetchError={null} />)
    // The old ambiguous "since agents last restarted" copy must be gone / replaced with an
    // explicit, non-misleading session framing.
    expect(screen.getAllByText(/this session/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/not a calendar day/i)).toBeInTheDocument()
  })

  it('shows an exact collection interval derived from real API data', () => {
    render(<KpiPanel costs={makeCosts()} fetchError={null} />)
    expect(screen.getByText(/every 15s/i)).toBeInTheDocument()
  })

  it('shows a stale-data warning when the API reports a collection error', () => {
    render(<KpiPanel costs={makeCosts({ stale_error: 'agent scrape timeout' })} fetchError={null} />)
    expect(screen.getByText(/agent scrape timeout/i)).toBeInTheDocument()
  })

  it('shows a fetch-failure warning distinct from a stale-data warning', () => {
    render(<KpiPanel costs={null} fetchError="network error" />)
    expect(screen.getByText(/network error/i)).toBeInTheDocument()
  })

  it('does not fabricate a collector-started timestamp when none exists yet', () => {
    render(<KpiPanel costs={makeCosts({ collector_started_at_utc: undefined })} fetchError={null} />)
    expect(screen.getByText(/no completed cycle yet/i)).toBeInTheDocument()
  })

  it('renders zeroed KPI values (not crash) when costs is entirely null', () => {
    render(<KpiPanel costs={null} fetchError={null} />)
    expect(screen.getByText('$0.00')).toBeInTheDocument()
  })
})
