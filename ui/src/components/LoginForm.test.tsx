import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import { ApiError } from '../api/client';
import { LoginForm } from './LoginForm';

const initial = { username: 'nextcloud', description: 'File sync', allowed_senders: [], rate_per_hour: 0, enabled: true };

function renderForm(props: Partial<Parameters<typeof LoginForm>[0]> = {}) {
  const onSubmit = vi.fn();
  render(
    <MemoryRouter>
      <LoginForm mode="edit" initial={initial} busy={false} cancelTo="/logins" onSubmit={onSubmit} {...props} />
    </MemoryRouter>,
  );
  return { onSubmit };
}

describe('LoginForm', () => {
  it('disables submit until something changes, in edit mode', async () => {
    renderForm();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
    expect(screen.getByText('No changes to save.')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Description'), '!');
    expect(screen.getByRole('button', { name: 'Save changes' })).not.toBeDisabled();
  });

  it('sends only the changed field as a patch', async () => {
    const { onSubmit } = renderForm();
    await userEvent.clear(screen.getByLabelText('Description'));
    await userEvent.type(screen.getByLabelText('Description'), 'Updated');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(onSubmit).toHaveBeenCalledWith({ description: 'Updated' });
  });

  it('focuses the field named by a server validation error and shows its message', () => {
    const error = new ApiError(400, 'VALIDATION_ERROR', 'Must be a number.', 'rate_per_hour');
    renderForm({ error });
    expect(screen.getByLabelText('Messages per hour')).toHaveFocus();
    expect(screen.getByText('Must be a number.')).toBeInTheDocument();
  });

  it('parses allowed senders by newline and comma, trimming and dropping empties', async () => {
    const { onSubmit } = renderForm();
    const field = screen.getByLabelText('Allowed senders');
    await userEvent.clear(field);
    await userEvent.type(field, '@example.com, alerts@foo.com{Enter}  {Enter}@bar.com');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(onSubmit).toHaveBeenCalledWith({ allowed_senders: ['@example.com', 'alerts@foo.com', '@bar.com'] });
  });
});
