import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Mono } from './Mono';

describe('Mono', () => {
  it('renders its children', () => {
    render(<Mono>nextcloud</Mono>);
    expect(screen.getByText('nextcloud')).toBeInTheDocument();
  });
});
