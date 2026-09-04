// Shape of a single row returned by GET /api/v1/costs (see internal/api for the Go source of
// truth). Every field the UI reads is listed here; anything else in the JSON is ignored.
export interface CostEntry {
  src_zone: string
  dst_zone: string
  src_namespace: string
  src_workload: string
  // dst_namespace/dst_workload identify the destination pod's owning workload — the actual
  // "which workload is this traffic going TO" the zone-only view couldn't answer.
  dst_namespace: string
  dst_workload: string
  gb: number
  cost_usd: number
}

export interface CostsTotals {
  cross_az_cost_usd: number
  cross_az_gb: number
  same_az_gb: number
  price_per_gb_usd: number
  price_per_gb_direction_usd?: number
}

export interface CostsResponse {
  cloud?: string
  region?: string
  updated_at?: string
  stale_error?: string
  totals: CostsTotals
  entries: CostEntry[]
}
