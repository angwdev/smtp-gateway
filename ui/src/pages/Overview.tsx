import ErrorOutlineIcon from '@mui/icons-material/ErrorOutlineOutlined';
import WarningAmberIcon from '@mui/icons-material/WarningAmberOutlined';
import Button from '@mui/material/Button';
import { Link } from 'react-router';

import { useOverview, useRenewCertificate, useTestUpstream } from '../api/hooks';
import { ApiError } from '../api/client';
import { CopyButton } from '../components/CopyButton';
import { Empty, Failure, Loading } from '../components/States';
import { Facts } from '../components/Facts';
import { HourBars } from '../components/HourBars';
import { Mono } from '../components/Mono';
import { Notice } from '../components/Notice';
import { PageHeader } from '../components/PageHeader';
import { Section } from '../components/Section';
import { StatusMark } from '../components/StatusMark';
import { formatDateTime } from '../lib/format';
import type { MessageStatus } from '../api/types';

const TLS_WORD: Record<string, string> = { starttls: 'STARTTLS', tls: 'TLS from the start', none: 'None (not encrypted)' };

function CountCell({ status, count }: { status: MessageStatus; count: number }) {
  const colorClass = count > 0 ? { sent: 'text-sent', failed: 'text-failed', rejected: 'text-rejected' }[status] : 'text-ink';
  return (
    <Link
      to={`/messages?status=${status}`}
      aria-label={`${count} ${status} in the last 24 hours, show them`}
      className="flex flex-col items-center gap-1 px-3 py-4 sm:px-6"
    >
      <span className={`text-3xl font-bold tabular-nums ${colorClass}`}>{count}</span>
      <StatusMark status={status} />
    </Link>
  );
}

export function Overview() {
  const overview = useOverview();
  const testUpstream = useTestUpstream();
  const renewCertificate = useRenewCertificate();

  if (overview.isPending) return <Loading label="Loading the overview…" />;
  if (overview.isError) return <Failure error={overview.error} onRetry={() => void overview.refetch()} />;
  const data = overview.data;

  const cert = data.certificate;
  const expiresNode = (() => {
    if (!cert.not_after) return 'Unknown';
    const base = `${formatDateTime(cert.not_after)} (in ${cert.days_left} days)`;
    if (cert.level === 'warn') return <><WarningAmberIcon sx={{ fontSize: 16 }} aria-hidden /> Soon: {base}</>;
    if (cert.level === 'bad') return <><ErrorOutlineIcon sx={{ fontSize: 16 }} aria-hidden /> Expired or about to: {base}</>;
    return base;
  })();

  return (
    <div>
      <PageHeader title="Overview" description="The last 24 hours, and the state of the connection to your mail server." />

      <div className="grid grid-cols-3 divide-x divide-hairline border-y border-hairline">
        <CountCell status="sent" count={data.counts_24h.sent} />
        <CountCell status="rejected" count={data.counts_24h.rejected} />
        <CountCell status="failed" count={data.counts_24h.failed} />
      </div>

      <div className="py-6">
        <HourBars series={data.series} />
      </div>

      <Section title="Logins">
        {data.credentials.total === 0 ? (
          <Empty title="Create the first login" action={<Button component={Link} to="/logins/new" variant="contained">New login</Button>}>
            Each app or device gets its own username and password. Mail is forwarded through your upstream account.
          </Empty>
        ) : (
          <p>
            {data.credentials.active} of {data.credentials.total} logins are turned on.{' '}
            <Link to="/logins">Manage logins</Link>
          </p>
        )}
      </Section>

      <div className="lg:grid lg:grid-cols-3 lg:gap-8">
        <Section title="Upstream">
          <Facts
            items={[
              { label: 'Server', value: <Mono>{data.upstream.addr}</Mono> },
              { label: 'Encryption', value: TLS_WORD[data.upstream.tls] ?? data.upstream.tls },
              { label: 'Signs in as', value: data.upstream.username ? <Mono>{data.upstream.username}</Mono> : 'No sign-in' },
              ...(data.upstream.envelope_from ? [{ label: 'Envelope sender', value: <Mono>{data.upstream.envelope_from}</Mono> }] : []),
            ]}
          />
          <Button
            variant="outlined"
            className="mt-3"
            loading={testUpstream.isPending}
            onClick={() => testUpstream.mutate()}
          >
            Test connection
          </Button>
          {testUpstream.data && (
            <Notice kind={testUpstream.data.ok ? 'success' : 'error'} className="mt-3">{testUpstream.data.message}</Notice>
          )}
          {testUpstream.isError && (
            <Notice kind="error" className="mt-3">
              {testUpstream.error instanceof ApiError ? testUpstream.error.message : 'Something went wrong on the gateway.'}
            </Notice>
          )}
        </Section>

        <Section title="Certificate">
          <Facts
            items={[
              { label: 'Source', value: cert.label },
              { label: 'Names', value: <Mono>{cert.names.join(', ')}</Mono> },
              { label: 'Expires', value: expiresNode },
              ...(cert.acme
                ? [
                  {
                    label: 'Automatic renewal',
                    value: cert.acme.next_check ? `On, next check ${formatDateTime(cert.acme.next_check)}` : 'On',
                  },
                ]
                : []),
            ]}
          />
          {cert.acme && cert.acme.last_error && (
            <Notice kind="error" className="mt-3">{cert.acme.last_error}</Notice>
          )}
          {cert.acme && (
            <>
              <Button
                variant="outlined"
                className="mt-3"
                disabled={cert.acme.renewing}
                onClick={() => renewCertificate.mutate()}
              >
                {cert.acme.renewing ? 'Renewal is running…' : 'Renew now'}
              </Button>
              {renewCertificate.isSuccess && (
                <Notice kind="info" className="mt-3">Renewal started. This can take a few minutes.</Notice>
              )}
            </>
          )}
        </Section>

        <Section title="Settings for mail clients">
          <Facts
            items={[
              {
                label: 'Server',
                value: <span className="flex min-w-0 flex-wrap items-center gap-2"><Mono className="min-w-0">{data.client_settings.hostname}</Mono><CopyButton value={data.client_settings.hostname} label="Copy server" /></span>,
              },
              ...(data.client_settings.submission_port
                ? [{ label: 'Port with STARTTLS', value: <Mono>{data.client_settings.submission_port}</Mono> }]
                : []),
              ...(data.client_settings.smtps_port
                ? [{ label: 'Port with TLS', value: <Mono>{data.client_settings.smtps_port}</Mono> }]
                : []),
              { label: 'Username and password', value: 'The login you create for each app' },
            ]}
          />
        </Section>
      </div>

      <p className="pt-6 text-sm text-muted">Version {data.version}</p>
    </div>
  );
}
