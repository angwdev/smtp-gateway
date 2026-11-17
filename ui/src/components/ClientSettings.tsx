import { useOverview } from '../api/hooks';
import { CopyButton } from './CopyButton';
import { Facts } from './Facts';
import { Mono } from './Mono';

export function ClientSettings({ username }: { username: string }) {
  const overview = useOverview();
  if (!overview.data) return null;
  const { client_settings: cs } = overview.data;
  return (
    <Facts
      items={[
        { label: 'Server', value: <span className="flex min-w-0 flex-wrap items-center gap-2"><Mono className="min-w-0">{cs.hostname}</Mono><CopyButton value={cs.hostname} label="Copy server" /></span> },
        ...(cs.submission_port ? [{ label: 'Port with STARTTLS', value: <Mono>{cs.submission_port}</Mono> }] : []),
        ...(cs.smtps_port ? [{ label: 'Port with TLS', value: <Mono>{cs.smtps_port}</Mono> }] : []),
        { label: 'Username', value: <span className="flex min-w-0 flex-wrap items-center gap-2"><Mono className="min-w-0">{username}</Mono><CopyButton value={username} label="Copy username" /></span> },
        { label: 'Password', value: 'The one shown above' },
      ]}
    />
  );
}
