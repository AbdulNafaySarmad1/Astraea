import { EmptyChart } from './ui';
import type { MetricSeries } from '@/lib/api';

export function MetricChart({ series }: { series?: MetricSeries }) {
  const points = series?.points || [];
  if (points.length < 2) return <EmptyChart />;

  const width = 600, height = 188, padX = 39, padY = 20;
  const max = Math.max(...points.map(point => point.value), 1) * 1.15;
  const coords = points.map((point, index) =>
    `${padX + (index / (points.length - 1)) * (width - padX - 13)},${height - padY - (point.value / max) * (height - padY * 2)}`
  ).join(' ');
  const formatValue = (value: number) => new Intl.NumberFormat('en-GB', { notation: 'compact', maximumFractionDigits: 1 }).format(value);
  const formatTime = (value: string) => `${new Date(value).toLocaleString('en-GB', { timeZone: 'UTC', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })} UTC`;
  const title = series?.metric.replaceAll('_', ' ') || 'Metric';

  return <div className="metric-chart">
    <svg role="img" aria-label={`${title} in ${series?.unit} over ${series?.window}; ${points.length} samples`} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <g className="grid-lines"><line x1={padX} y1="20" x2={width - 13} y2="20"/><line x1={padX} y1="94" x2={width - 13} y2="94"/><line x1={padX} y1="168" x2={width - 13} y2="168"/></g>
      <g className="axis-labels"><text x="4" y="23">{formatValue(max)}</text><text x="4" y="97">{formatValue(max / 2)}</text><text x="19" y="171">0</text></g>
      <polyline points={coords} fill="none" stroke="#227b76" strokeWidth="2.5" strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke"/>
    </svg>
    <div className="chart-caption"><span>{formatTime(points[0].at)}</span><strong>{title.toUpperCase()} · {series?.unit.toUpperCase()}</strong><span>{formatTime(points[points.length - 1].at)}</span></div>
  </div>;
}
