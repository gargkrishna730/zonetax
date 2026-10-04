import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Segmented } from './components/Segmented'
import { TopOffendersTable } from './components/TopOffendersTable'
import { CostHistoryChart } from './components/CostHistoryChart'
import type { HistoryResponse, MapEntry } from './types'

describe('Segmented (radiogroup)', () => {
  const opts = [
    { value: 'a', label: 'A' },
    { value: 'b', label: 'B' },
    { value: 'c', label: 'C' },
  ]
  it('exposes one tab stop and moves selection with arrow keys', () => {
    const onChange = vi.fn()
    render(<Segmented label="Pick" options={opts} value="a" onChange={onChange} />)
    const radios = screen.getAllByRole('radio')
    expect(radios.map((r) => r.getAttribute('tabindex'))).toEqual(['0', '-1', '-1'])
    expect(radios[0]).toHaveAttribute('aria-checked', 'true')
    fireEvent.keyDown(radios[0], { key: 'ArrowRight' })
    expect(onChange).toHaveBeenLastCalledWith('b')
    fireEvent.keyDown(radios[0], { key: 'ArrowLeft' })
    expect(onChange).toHaveBeenLastCalledWith('c')
    fireEvent.keyDown(radios[0], { key: 'End' })
    expect(onChange).toHaveBeenLastCalledWith('c')
  })
})

const entry = (w: string, cost: number, gb: number): MapEntry => ({
  src_zone: 'a', dst_zone: 'b', src_namespace: 'ns', src_workload: w, dst_namespace: 'ns', dst_workload: 'db', cost_usd: cost, gb,
})

describe('TopOffendersTable', () => {
  it('rows are keyboard-activatable and sorting updates aria-sort', () => {
    const onSelect = vi.fn()
    render(
      <TopOffendersTable entries={[entry('cheap-big', 0.1, 50), entry('pricey-small', 1, 5)]} totalCrossAZCostUSD={1.1} range="24h" onSelectRoute={onSelect} collapsed={false} onToggleCollapsed={() => {}} />,
    )
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows[0]).toHaveTextContent('pricey-small')
    fireEvent.keyDown(rows[0], { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ src_workload: 'pricey-small' }))

    fireEvent.click(screen.getByRole('button', { name: /Traffic/ }))
    expect(screen.getAllByRole('row')[1]).toHaveTextContent('cheap-big')
    expect(screen.getByRole('columnheader', { name: /Traffic/ })).toHaveAttribute('aria-sort', 'descending')
    expect(screen.getByRole('columnheader', { name: /Cost/ })).toHaveAttribute('aria-sort', 'none')
  })
})

describe('CostHistoryChart', () => {
  const history: HistoryResponse = {
    range_requested: '24h', range_start_utc: '2026-10-01T00:00:00Z', range_end_utc: '2026-10-01T03:00:00Z', server_time_utc: '2026-10-01T03:00:00Z',
    history_start_utc: '2026-10-01T00:30:00Z', scrape_interval_seconds: 30, bucket_size_seconds: 3600,
    buckets: [
      { start_utc: '2026-10-01T00:00:00Z', end_utc: '2026-10-01T01:00:00Z', cross_az_cost_usd: 0, cross_az_gb: 0, same_az_gb: 0, complete: false, has_data: false },
      { start_utc: '2026-10-01T01:00:00Z', end_utc: '2026-10-01T02:00:00Z', cross_az_cost_usd: 0.08, cross_az_gb: 4, same_az_gb: 1, complete: true, has_data: true },
      { start_utc: '2026-10-01T02:00:00Z', end_utc: '2026-10-01T03:00:00Z', cross_az_cost_usd: 0.03, cross_az_gb: 1.5, same_az_gb: 0.2, complete: false, has_data: true },
    ],
  }
  it('labels every bar with value and completeness, never as $0 for missing data', () => {
    render(<CostHistoryChart range="24h" onRangeChange={() => {}} history={history} loading={false} error={null} />)
    const bars = screen.getAllByRole('button', { pressed: false }).concat(screen.getAllByRole('button', { pressed: true })).filter((b) => b.classList.contains('bar-btn'))
    const labels = bars.map((b) => b.getAttribute('aria-label') ?? '')
    expect(labels.some((l) => l.includes('no data'))).toBe(true)
    expect(labels.some((l) => l.includes('$0.08') && l.includes('complete'))).toBe(true)
    expect(labels.some((l) => l.includes('$0.03') && l.includes('partial'))).toBe(true)
    expect(labels.some((l) => l.includes('$0.00'))).toBe(false)
  })
  it('arrow keys move the selected bar and update the detail row', () => {
    render(<CostHistoryChart range="24h" onRangeChange={() => {}} history={history} loading={false} error={null} />)
    const pressed = () => document.querySelector('.bar-btn[aria-pressed="true"]')!
    expect(pressed().getAttribute('aria-label')).toContain('$0.03') // defaults to latest
    fireEvent.keyDown(pressed(), { key: 'ArrowLeft' })
    expect(pressed().getAttribute('aria-label')).toContain('$0.08')
    expect(document.querySelector('.chart-detail')).toHaveTextContent('$0.08')
  })
})
