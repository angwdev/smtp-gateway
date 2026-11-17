import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { PageHeader } from './PageHeader';

describe('PageHeader', () => {
  it('renders one h1 with the title and sets the document title', () => {
    render(<PageHeader title="Logins" description="One per app." />);
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Logins');
    expect(document.title).toBe('Logins · SMTP Gateway');
  });

  it('renders the back link and the action', () => {
    render(<PageHeader title="Login" back={{ to: '/logins', label: 'Logins' }} action={<button>New</button>} />);
    expect(screen.getByRole('link', { name: /Logins/ })).toHaveAttribute('href', '/logins');
    expect(screen.getByRole('button', { name: 'New' })).toBeInTheDocument();
  });
});
