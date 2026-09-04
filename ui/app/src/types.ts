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
  // server_time_utc is this response's own generation time (RFC3339 UTC) — an exact "as of"
  // timestamp independent of the client clock, always present.
  server_time_utc: string
  // collector_started_at_utc is when this collector process completed its first successful
  // collection cycle — the moment "tracked" totals started accumulating. Empty if no cycle has
  // completed yet. This is a SESSION/PROCESS metric, not a calendar day — restarts reset it.
  collector_started_at_utc?: string
  // scrape_interval_seconds is the collector's actual collection cadence — the real bound on
  // data freshness (distinct from how often the UI itself polls).
  scrape_interval_seconds: number
}

// One hourly bucket from GET /api/v1/history.
export interface HistoryBucket {
  start_utc: string
  end_utc: string
  cross_az_cost_usd: number
  cross_az_gb: number
  same_az_gb: number
  // complete=false: this bucket's window is only partially observed (current in-progress hour,
  // or a bucket that started before history began) — its numbers are real but partial, not a
  // full period's total.
  complete: boolean
  // has_data=false: no observed data at all for this bucket (entirely before history began).
  has_data: boolean
}

export type HistoryRange = '1h' | '6h' | '24h' | '7d'

export interface HistoryResponse {
  range_requested: HistoryRange
  range_start_utc: string
  range_end_utc: string
  server_time_utc: string
  // history_start_utc: when this collector's retained history actually begins. May be later
  // than range_start_utc if the collector hasn't been running for the full requested range.
  history_start_utc?: string
  scrape_interval_seconds: number
  bucket_size_seconds: number
  buckets: HistoryBucket[]
}
