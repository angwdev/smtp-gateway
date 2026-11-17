import { describe, expect, it } from 'vitest';

import { isActive, NAV } from './nav';

const find = (to: string) => {
  const item = NAV.find((entry) => entry.to === to);
  if (!item) throw new Error(`no nav item for ${to}`);
  return item;
};

describe('isActive', () => {
  it('matches the overview tab only on the root path', () => {
    expect(isActive(find('/'), '/')).toBe(true);
    expect(isActive(find('/'), '/logins')).toBe(false);
    expect(isActive(find('/'), '/messages')).toBe(false);
  });

  it('matches other items on their own path and their subtree', () => {
    expect(isActive(find('/messages'), '/messages')).toBe(true);
    expect(isActive(find('/messages'), '/messages/9')).toBe(true);
    expect(isActive(find('/messages'), '/messages?status=failed')).toBe(false);
    expect(isActive(find('/logins'), '/logins/new')).toBe(true);
    expect(isActive(find('/logins'), '/audit')).toBe(false);
  });

  it('gives the More tab ownership of /tokens', () => {
    expect(isActive(find('/more'), '/tokens')).toBe(true);
    expect(isActive(find('/more'), '/tokens/'.slice(0, -1) + '/x')).toBe(true);
    expect(isActive(find('/more'), '/more')).toBe(true);
    expect(isActive(find('/more'), '/')).toBe(false);
  });

  it('does not let the rail-only tokens item match /more', () => {
    expect(isActive(find('/tokens'), '/more')).toBe(false);
    expect(isActive(find('/tokens'), '/tokens')).toBe(true);
  });
});
