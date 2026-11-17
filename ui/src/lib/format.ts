import dayjs from 'dayjs';

import type { MessageStatus } from '../api/types';

export const STATUS_WORD: Record<MessageStatus, string> = { sent: 'Sent', failed: 'Failed', rejected: 'Rejected' };

export function formatDateTime(iso: string): string {
  return dayjs(iso).format('D MMM YYYY, HH:mm');
}

// Compact time for log rows: today shows the time, this year the day, older the date.
export function formatTime(iso: string): string {
  const then = dayjs(iso);
  const now = dayjs();
  if (then.isSame(now, 'day')) return then.format('HH:mm');
  if (then.isSame(now, 'year')) return then.format('D MMM, HH:mm');
  return then.format('D MMM YYYY');
}

export function formatRelative(iso: string | null): string {
  if (!iso) return 'never';
  const seconds = Math.max(0, dayjs().diff(dayjs(iso), 'second'));
  if (seconds < 60) return 'just now';
  const steps: [number, string][] = [[60, 'minute'], [60, 'hour'], [24, 'day']];
  let value = seconds;
  let unit = 'second';
  for (const [size, name] of steps) {
    if (value < size) break;
    value = Math.floor(value / size);
    unit = name;
  }
  return `${value} ${unit}${value === 1 ? '' : 's'} ago`;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) {
    const kb = bytes / 1024;
    const rounded = kb < 10 ? kb.toFixed(1) : String(Math.round(kb));
    // Rounding can carry a value up to "1024" (e.g. 1,048,575 bytes), which
    // belongs in the MB tier, not as "1024 KB". Fall through consistently
    // with the >= 1024*1024 branch below.
    if (Number(rounded) < 1024) return `${rounded} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatCount(n: number, noun: string): string {
  return `${n.toLocaleString('en-US')} ${noun}${n === 1 ? '' : 's'}`;
}

export function recipients(list: string[]): string {
  const [first, ...rest] = list;
  if (first === undefined) return 'no recipients';
  return rest.length === 0 ? first : `${first} and ${rest.length} more`;
}

// A message has no name of its own, so it is titled by what happened and when.
export function messageTitle(status: MessageStatus, createdAt: string): string {
  return `${STATUS_WORD[status]} ${formatDateTime(createdAt)}`;
}

export function browserName(userAgent: string): string {
  if (!userAgent) return 'Unknown client';
  const tool = /^(curl|Wget|python-requests|Go-http-client|node|PostmanRuntime)/i.exec(userAgent);
  if (tool) return tool[1] as string;
  const browser = /Firefox\//.test(userAgent) ? 'Firefox'
    : /Edg\//.test(userAgent) ? 'Edge'
    : /Chrome\//.test(userAgent) ? 'Chrome'
    : /Safari\//.test(userAgent) ? 'Safari'
    : 'Browser';
  const system = /iPhone/.test(userAgent) ? 'iPhone'
    : /iPad/.test(userAgent) ? 'iPad'
    : /Android/.test(userAgent) ? 'Android'
    : /Mac OS X/.test(userAgent) ? 'macOS'
    : /Windows/.test(userAgent) ? 'Windows'
    : /Linux/.test(userAgent) ? 'Linux'
    : '';
  return system ? `${browser} on ${system}` : browser;
}

// Date pickers give a calendar day; the API wants instants.
export const toRFC3339Start = (day: string): string => dayjs(day).startOf('day').toISOString();
export const toRFC3339End = (day: string): string => dayjs(day).endOf('day').toISOString();
