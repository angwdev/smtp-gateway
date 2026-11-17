import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { SecretReveal } from './SecretReveal';

describe('SecretReveal', () => {
  it('shows the secret as copyable text and calls onDone', async () => {
    const onDone = vi.fn();
    render(<SecretReveal title="Password for nextcloud" secret="s3cret-value" doneLabel="Done" onDone={onDone} />);
    expect(screen.getByText('s3cret-value')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Copy password/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(onDone).toHaveBeenCalledOnce();
  });

  it('renders extra children between the secret and the done button', () => {
    render(
      <SecretReveal title="Token for grafana" secret="sgw_abc" doneLabel="Done" onDone={vi.fn()}>
        <p>Extra facts</p>
      </SecretReveal>,
    );
    expect(screen.getByText('Extra facts')).toBeInTheDocument();
  });
});
