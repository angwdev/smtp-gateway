import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Notice } from './Notice';

describe('Notice', () => {
  it('renders the message as status for success, info and warning', () => {
    render(<Notice kind="success">Saved.</Notice>);
    expect(screen.getByRole('status')).toHaveTextContent('Saved.');
  });

  it('renders the message as alert for error, so it is announced at once', () => {
    render(<Notice kind="error">Could not save.</Notice>);
    expect(screen.getByRole('alert')).toHaveTextContent('Could not save.');
  });

  it('never uses a result colour class regardless of kind', () => {
    for (const kind of ['success', 'info', 'warning', 'error'] as const) {
      const { container, unmount } = render(<Notice kind={kind}>Text</Notice>);
      const html = container.innerHTML;
      expect(html).not.toMatch(/text-sent|text-rejected|text-failed/);
      unmount();
    }
  });
});
