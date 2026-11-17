import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { CopyButton } from './CopyButton';

describe('CopyButton', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('writes the value and shows Copied for two seconds', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<CopyButton value="s3cret" label="Copy password" />);
    await userEvent.click(screen.getByRole('button', { name: 'Copy password' }), { delay: null });
    expect(writeText).toHaveBeenCalledWith('s3cret');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument());
    vi.advanceTimersByTime(2000);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy password' })).toBeInTheDocument());
  });

  it('shows manual-copy text when the clipboard API rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'));
    Object.assign(navigator, { clipboard: { writeText } });
    render(<CopyButton value="s3cret" label="Copy password" />);
    await userEvent.click(screen.getByRole('button', { name: 'Copy password' }), { delay: null });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Select and copy manually' })).toBeInTheDocument());
  });
});
