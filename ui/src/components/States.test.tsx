import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { Empty, Failure, Loading } from './States';

describe('States', () => {
  it('Loading has role status', () => {
    render(<Loading label="Loading the logins…" />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading the logins…');
  });

  it('Empty shows a title, text and an optional action', () => {
    render(<Empty title="No logins" action={<button>New login</button>}>Create one to get started.</Empty>);
    expect(screen.getByText('No logins')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New login' })).toBeInTheDocument();
  });

  it('Failure calls onRetry', async () => {
    const onRetry = vi.fn();
    render(<Failure error={new Error('Database is locked.')} onRetry={onRetry} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Database is locked.');
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});
