import {
  browserName,
  formatCount,
  formatDateTime,
  formatRelative,
  formatSize,
  formatTime,
  messageTitle,
  recipients,
  toRFC3339End,
  toRFC3339Start,
} from './format';

const NOW = '2026-11-17T12:00:00Z';

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(NOW));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('formatDateTime', () => {
  it.each([
    ['2026-11-17T03:12:09Z', '17 Nov 2026, 03:12'],
  ])('%s -> %s', (input, expected) => {
    expect(formatDateTime(input)).toBe(expected);
  });
});

describe('formatTime', () => {
  it.each([
    ['2026-11-17T03:12:09Z', '03:12'],
    ['2026-11-16T22:05:00Z', '16 Nov, 22:05'],
    ['2025-12-31T22:05:00Z', '31 Dec 2025'],
  ])('%s -> %s', (input, expected) => {
    expect(formatTime(input)).toBe(expected);
  });
});

describe('formatRelative', () => {
  it.each([
    ['2026-11-17T11:59:30Z', 'just now'],
    ['2026-11-17T11:55:00Z', '5 minutes ago'],
    ['2026-11-17T11:00:00Z', '1 hour ago'],
    ['2026-11-14T12:00:00Z', '3 days ago'],
    [null, 'never'],
  ])('%s -> %s', (input, expected) => {
    expect(formatRelative(input)).toBe(expected);
  });
});

describe('formatSize', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1536, '1.5 KB'],
    [43867, '43 KB'],
    [5242880, '5.0 MB'],
    [1023, '1023 B'],
    [1024, '1.0 KB'],
    [1047552, '1023 KB'],
    // 1,048,575 rounds to 1024 KB before the MB check; must carry into MB.
    [1048575, '1.0 MB'],
    [1048576, '1.0 MB'],
  ])('%d -> %s', (input, expected) => {
    expect(formatSize(input)).toBe(expected);
  });
});

describe('formatCount', () => {
  it.each([
    [1, 'message', '1 message'],
    [1234, 'message', '1,234 messages'],
  ])('%d %s -> %s', (n, noun, expected) => {
    expect(formatCount(n, noun)).toBe(expected);
  });
});

describe('recipients', () => {
  it.each([
    [['a@b.c'], 'a@b.c'],
    [['a@b.c', 'd@e.f', 'g@h.i'], 'a@b.c and 2 more'],
    [[], 'no recipients'],
  ])('%j -> %s', (input, expected) => {
    expect(recipients(input)).toBe(expected);
  });
});

describe('messageTitle', () => {
  it('titles a sent message by status and time', () => {
    expect(messageTitle('sent', '2026-11-17T03:12:09Z')).toBe('Sent 17 Nov 2026, 03:12');
  });
});

describe('browserName', () => {
  it.each([
    ['Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0', 'Firefox on Linux'],
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1', 'Safari on iPhone'],
    ['Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36', 'Chrome on Android'],
    ['curl/8.5.0', 'curl'],
    ['', 'Unknown client'],
  ])('%s -> %s', (input, expected) => {
    expect(browserName(input)).toBe(expected);
  });
});

describe('toRFC3339Start / toRFC3339End', () => {
  it('bounds a calendar day in UTC', () => {
    expect(toRFC3339Start('2026-11-17')).toBe('2026-11-17T00:00:00.000Z');
    expect(toRFC3339End('2026-11-17')).toBe('2026-11-17T23:59:59.999Z');
  });
});
