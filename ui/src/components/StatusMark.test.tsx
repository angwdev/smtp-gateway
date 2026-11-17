import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { StatusMark } from './StatusMark';

describe('StatusMark', () => {
  it('shows the word for every status', () => {
    render(
      <ul>
        <li><StatusMark status="sent" /></li>
        <li><StatusMark status="failed" /></li>
        <li><StatusMark status="rejected" /></li>
      </ul>,
    );
    expect(screen.getByText('Sent')).toBeInTheDocument();
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('Rejected')).toBeInTheDocument();
  });

  it('does not rely on a title attribute alone: the word is visible text', () => {
    render(<StatusMark status="sent" />);
    const word = screen.getByText('Sent');
    expect(word.className).not.toContain('sr-only');
  });

  it('can hide the word visually while keeping it for screen readers', () => {
    render(<StatusMark status="sent" wordHidden />);
    const word = screen.getByText('Sent');
    expect(word.className).toContain('sr-only');
  });
});
