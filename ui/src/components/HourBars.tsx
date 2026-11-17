import type { HourCount } from '../api/types';

export interface HourBarsProps { series: HourCount[] }

const SLOT_WIDTH = 10;
const BAR_WIDTH = 8;
const CHART_HEIGHT = 60;
const BASELINE_Y = 60;

export function HourBars({ series }: HourBarsProps) {
  const total = series.reduce((sum, h) => sum + h.sent + h.failed + h.rejected, 0);
  const max = Math.max(1, ...series.map((h) => h.sent + h.failed + h.rejected));

  if (total === 0) {
    return <p className="py-4 text-ink2">No messages in the last 24 hours.</p>;
  }

  const busiest = Math.max(...series.map((h) => h.sent + h.failed + h.rejected));
  const label = `Messages per hour over the last 24 hours: ${total} in total, busiest hour ${busiest}.`;

  return (
    <div>
      <svg width="100%" viewBox="0 0 240 64" preserveAspectRatio="none" role="img" aria-label={label}>
        <line x1="0" y1={BASELINE_Y} x2="240" y2={BASELINE_Y} stroke="var(--sg-hairline)" />
        {series.map((hour, index) => {
          const x = index * SLOT_WIDTH + 1;
          const total3 = hour.sent + hour.failed + hour.rejected;
          const scale = (count: number) => (count === 0 ? 0 : Math.max(2, (count / max) * CHART_HEIGHT));
          const sentH = total3 > 0 ? scale(hour.sent) : 0;
          const rejectedH = total3 > 0 ? scale(hour.rejected) : 0;
          const failedH = total3 > 0 ? scale(hour.failed) : 0;
          let y = BASELINE_Y;
          const bars = [] as { h: number; fill: string }[];
          bars.push({ h: sentH, fill: 'var(--sg-sent)' });
          bars.push({ h: rejectedH, fill: 'var(--sg-rejected)' });
          bars.push({ h: failedH, fill: 'var(--sg-failed)' });
          const rects = bars.map((bar, i) => {
            y -= bar.h;
            return <rect key={i} x={x} y={y} width={BAR_WIDTH} height={bar.h} fill={bar.fill} />;
          });
          return <g key={hour.hour}>{rects}</g>;
        })}
      </svg>
      <div className="flex justify-between text-xs text-muted">
        <span>24 hours ago</span>
        <span>now</span>
      </div>
      <table className="sr-only">
        <caption>Messages per hour</caption>
        <thead>
          <tr><th scope="col">Hour</th><th scope="col">Sent</th><th scope="col">Rejected</th><th scope="col">Failed</th></tr>
        </thead>
        <tbody>
          {series.map((hour) => (
            <tr key={hour.hour}>
              <th scope="row">{hour.hour}</th>
              <td>{hour.sent}</td>
              <td>{hour.rejected}</td>
              <td>{hour.failed}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
