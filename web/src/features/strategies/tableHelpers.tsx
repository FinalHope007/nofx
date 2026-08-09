export function formatMoney(value: number): string {
  if (!value) return '$0'
  const abs = Math.abs(value)
  if (abs >= 1_000_000_000) return `$${(value / 1_000_000_000).toFixed(2)}B`
  if (abs >= 1_000_000) return `$${(value / 1_000_000).toFixed(2)}M`
  if (abs >= 1_000) return `$${(value / 1_000).toFixed(2)}K`
  return `$${value.toFixed(2)}`
}

export function EquitySparkline({
  points,
}: {
  points: { timestamp: string; total_equity: number }[]
}) {
  if (points.length < 2) return <span className="text-nofx-text-muted">—</span>
  const values = points.map((p) => p.total_equity)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const w = 80
  const h = 24
  const coords = values
    .map((v, i) => {
      const x = (i / (values.length - 1)) * w
      const y = h - ((v - min) / span) * h
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      className="inline-block"
    >
      <polyline
        fill="none"
        stroke="#D4AF37"
        strokeWidth={1.5}
        points={coords}
      />
    </svg>
  )
}
