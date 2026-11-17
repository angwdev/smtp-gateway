import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { HourBars } from './HourBars';

function hours(overrides: Partial<Record<number, { sent: number; failed: number; rejected: number }>> = {}) {
  return Array.from({ length: 24 }, (_, i) => ({
    hour: `2026-11-16T${String(i).padStart(2, '0')}:00:00Z`,
    sent: overrides[i]?.sent ?? 0,
    failed: overrides[i]?.failed ?? 0,
    rejected: overrides[i]?.rejected ?? 0,
  }));
}

describe('HourBars', () => {
  it('renders 24 slots in the hidden table and an aria-label with the total', () => {
    const series = hours({ 3: { sent: 5, failed: 0, rejected: 0 }, 10: { sent: 2, failed: 1, rejected: 0 } });
    render(<HourBars series={series} />);
    const rows = screen.getAllByRole('row');
    // 24 data rows + 1 header row
    expect(rows).toHaveLength(25);
    expect(screen.getByRole('img')).toHaveAccessibleName('Messages per hour over the last 24 hours: 8 in total, busiest hour 5.');
  });

  it('shows the empty sentence when every hour is zero', () => {
    render(<HourBars series={hours()} />);
    expect(screen.getByText('No messages in the last 24 hours.')).toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
  });
});
