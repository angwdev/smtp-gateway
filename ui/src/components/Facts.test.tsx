import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Facts } from './Facts';

describe('Facts', () => {
  it('exposes each pair as a term and a definition', () => {
    render(<Facts items={[{ label: 'Server', value: 'mail.example.com' }]} />);
    expect(screen.getByText('Server').tagName).toBe('DT');
    expect(screen.getByText('mail.example.com').tagName).toBe('DD');
  });
});
