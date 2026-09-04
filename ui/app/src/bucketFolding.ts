import type { HistoryBucket } from './types'

/** Folds a sequence of hourly HistoryBuckets into daily buckets (grouped by the LOCAL calendar
 * date of each hourly bucket's start, matching how a user reading "cost by day" expects days to
 * be split — not UTC-day boundaries, which would look wrong to anyone outside UTC). A daily
 * bucket is Complete only if every hourly bucket that folded into it was itself complete, and
 * HasData if at least one contributing hourly bucket had data — so a day that's still
 * in-progress (today) is correctly shown as partial, not silently treated as a finished day. */
export function foldHourlyToDaily(buckets: HistoryBucket[]): HistoryBucket[] {
  const byDay = new Map<string, HistoryBucket>()
  const order: string[] = []

  for (const b of buckets) {
    const start = new Date(b.start_utc)
    const dayKey = localDayKey(start)
    let day = byDay.get(dayKey)
    if (!day) {
      day = {
        start_utc: localDayStartISO(start),
        end_utc: localDayEndISO(start),
        cross_az_cost_usd: 0,
        cross_az_gb: 0,
        same_az_gb: 0,
        complete: true,
        has_data: false,
      }
      byDay.set(dayKey, day)
      order.push(dayKey)
    }
    day.cross_az_cost_usd += b.cross_az_cost_usd
    day.cross_az_gb += b.cross_az_gb
    day.same_az_gb += b.same_az_gb
    day.has_data = day.has_data || b.has_data
    day.complete = day.complete && b.complete
  }

  return order.map((k) => byDay.get(k)!).sort((a, b) => new Date(a.start_utc).getTime() - new Date(b.start_utc).getTime())
}

function localDayKey(d: Date): string {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

function localDayStartISO(d: Date): string {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 0, 0, 0, 0).toISOString()
}

function localDayEndISO(d: Date): string {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1, 0, 0, 0, 0).toISOString()
}
