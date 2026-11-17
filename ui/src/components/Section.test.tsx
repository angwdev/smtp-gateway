import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Section } from './Section';

describe('Section', () => {
  it('labels the section with its heading', () => {
    render(<Section title="Upstream"><p>Server details</p></Section>);
    const section = screen.getByRole('region', { name: 'Upstream' });
    expect(section).toContainElement(screen.getByText('Server details'));
  });
});
