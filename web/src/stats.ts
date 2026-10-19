import type { Totals } from './api'
import { formatCompact, formatCost, formatCount, formatLatency, formatPercent } from './format'

export type Stat = {
  label: string
  value: string
  sub: string
  tone?: 'success' | 'warning'
}

/** The five traffic stats shared by Overview and Projects, plus spend when the
 *  provider actually reported prices. */
export function trafficStats(totals: Totals): Stat[] {
  const stats: Stat[] = [
    { label: 'Requests', value: formatCount(totals.requests), sub: `${formatCount(totals.errors)} upstream errors` },
    {
      label: 'Tokens',
      value: formatCompact(totals.inputTokens + totals.outputTokens),
      sub: `${formatCompact(totals.inputTokens)} in · ${formatCompact(totals.outputTokens)} out`,
    },
    {
      label: 'Cache hit rate',
      value: formatPercent(totals.cacheHitRate),
      sub: `${formatCompact(totals.cacheReadTokens)} tokens reused`,
    },
    {
      label: 'Avg latency',
      value: formatLatency(totals.avgLatencyMs),
      sub: `p95 ${formatLatency(totals.p95LatencyMs)}`,
    },
    {
      label: 'Time to first token',
      value: formatLatency(totals.avgTtftMs),
      sub: `p95 ${formatLatency(totals.p95TtftMs)}`,
    },
  ]
  return withSpend(stats, totals)
}

export function withSpend(stats: Stat[], totals: Totals): Stat[] {
  if (totals.pricedRequests === 0) return stats
  return [
    ...stats,
    {
      label: 'Spend',
      value: formatCost(totals.costUSD),
      sub: `${formatCount(totals.pricedRequests)} of ${formatCount(totals.requests)} priced`,
    },
  ]
}
