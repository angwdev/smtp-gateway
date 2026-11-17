import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from './ConfirmDialog';

describe('ConfirmDialog', () => {
  it('fires onConfirm once even on a double click', async () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog open title="Delete nextcloud?" confirmLabel="Delete login" onConfirm={onConfirm} onClose={vi.fn()}>
        This cannot be undone.
      </ConfirmDialog>,
    );
    const button = screen.getByRole('button', { name: 'Delete login' });
    await userEvent.click(button);
    await userEvent.click(button);
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it('disables the confirm and cancel buttons while busy', () => {
    render(
      <ConfirmDialog open busy title="Delete nextcloud?" confirmLabel="Delete login" onConfirm={vi.fn()} onClose={vi.fn()}>
        This cannot be undone.
      </ConfirmDialog>,
    );
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  });

  it('shows the server error', () => {
    render(
      <ConfirmDialog open title="Revoke grafana?" confirmLabel="Revoke token" error="Could not revoke." onConfirm={vi.fn()} onClose={vi.fn()}>
        Anything using this token will be refused at once.
      </ConfirmDialog>,
    );
    expect(screen.getByText('Could not revoke.')).toBeInTheDocument();
  });

  it('focuses Cancel for a destructive dialog', async () => {
    render(
      <ConfirmDialog open destructive title="Delete nextcloud?" confirmLabel="Delete login" onConfirm={vi.fn()} onClose={vi.fn()}>
        This cannot be undone.
      </ConfirmDialog>,
    );
    await waitFor(() => expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus());
  });

  it('calls onClose on Escape', async () => {
    const onClose = vi.fn();
    render(
      <ConfirmDialog open title="Delete nextcloud?" confirmLabel="Delete login" onConfirm={vi.fn()} onClose={onClose}>
        This cannot be undone.
      </ConfirmDialog>,
    );
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
