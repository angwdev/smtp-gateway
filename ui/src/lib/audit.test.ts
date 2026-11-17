import type { AuditChange, AuditEvent } from '../api/types';
import { ACTION_FILTERS, changeRows, describeEvent } from './audit';

function event(overrides: Partial<AuditEvent>): AuditEvent {
  return {
    id: 1,
    created_at: '2026-11-17T03:12:09Z',
    actor: 'ana@example.com',
    actor_type: 'user',
    action: 'session.login',
    target_type: '',
    target_id: null,
    target_label: '',
    result: 'ok',
    ip: '',
    user_agent: '',
    request_id: 'req-fixture-0001',
    detail: '',
    changes: {},
    ...overrides,
  };
}

describe('describeEvent', () => {
  it.each([
    ['session.login', {}, 'signed in'],
    ['session.login_denied', {}, 'was refused sign-in'],
    ['session.logout', {}, 'signed out'],
    ['credential.create', { target_label: 'nextcloud' }, 'created login nextcloud'],
    ['credential.update', { target_label: 'nextcloud' }, 'changed login nextcloud'],
    ['credential.enable', { target_label: 'nextcloud' }, 'turned on login nextcloud'],
    ['credential.disable', { target_label: 'nextcloud' }, 'turned off login nextcloud'],
    ['credential.rotate', { target_label: 'nextcloud' }, 'replaced the password of login nextcloud'],
    ['credential.delete', { target_label: 'nextcloud' }, 'deleted login nextcloud'],
    ['token.create', { target_label: 'grafana' }, 'created API token grafana'],
    ['token.revoke', { target_label: 'grafana' }, 'revoked API token grafana'],
    ['token.auth_failed', {}, 'sent an API token that was not accepted'],
    ['certificate.renew', {}, 'asked for a certificate renewal'],
    ['upstream.test', {}, 'tested the connection to the upstream server'],
    ['smtp.auth_failed', { target_label: 'nextcloud' }, 'failed to sign in over SMTP as nextcloud'],
    ['smtp.lockout', {}, 'was blocked from SMTP after repeated failed sign-ins'],
    ['api.call', {}, 'called the API'],
  ] satisfies [string, Partial<AuditEvent>, string][])('%s -> %s', (action, overrides, expected) => {
    expect(describeEvent(event({ action, ...overrides }))).toBe(expected);
  });

  it('returns the action string itself for an unknown action', () => {
    expect(describeEvent(event({ action: 'unknown.action' }))).toBe('unknown.action');
  });
});

describe('changeRows', () => {
  it.each([
    [{ rate_per_hour: { from: 0, to: 100 } }, [{ label: 'Hourly limit', before: 'No limit', after: '100 per hour' }]],
    [{ enabled: { from: true, to: false } }, [{ label: 'Enabled', before: 'Yes', after: 'No' }]],
    [
      { allowed_senders: { from: [], to: ['@example.com'] } },
      [{ label: 'Allowed senders', before: 'Any sender', after: '@example.com' }],
    ],
    [{ description: { from: '', to: 'x' } }, [{ label: 'Description', before: '(empty)', after: 'x' }]],
    [{ nickname: { from: 'old', to: 'new' } }, [{ label: 'nickname', before: 'old', after: 'new' }]],
  ] satisfies [Record<string, AuditChange>, { label: string; before: string; after: string }[]][])(
    '%j -> %j',
    (changes, expected) => {
      expect(changeRows(changes)).toEqual(expected);
    },
  );
});

describe('ACTION_FILTERS', () => {
  it('has no duplicate values', () => {
    const values = ACTION_FILTERS.map((f) => f.value);
    expect(new Set(values).size).toBe(values.length);
  });
});
