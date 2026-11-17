import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { LoadMore } from './LoadMore';

describe('LoadMore', () => {
  it('calls onClick and shows the running count', async () => {
    const onClick = vi.fn();
    render(<LoadMore hasMore loading={false} onClick={onClick} shown={50} noun="message" />);
    await userEvent.click(screen.getByRole('button', { name: 'Show older' }));
    expect(onClick).toHaveBeenCalledOnce();
    expect(screen.getByText('Showing 50 messages')).toBeInTheDocument();
  });

  it('is hidden when there is nothing shown and nothing more', () => {
    render(<LoadMore hasMore={false} loading={false} onClick={vi.fn()} shown={0} noun="message" />);
    expect(screen.queryByRole('button', { name: 'Show older' })).not.toBeInTheDocument();
    expect(screen.queryByText(/everything/)).not.toBeInTheDocument();
  });

  it('shows the retention note once everything has loaded', () => {
    render(
      <LoadMore hasMore={false} loading={false} onClick={vi.fn()} shown={12} noun="event">
        Events older than 30 days are removed.
      </LoadMore>,
    );
    expect(screen.getByText(/That is everything\./)).toBeInTheDocument();
    expect(screen.getByText(/Events older than 30 days/)).toBeInTheDocument();
  });
});
