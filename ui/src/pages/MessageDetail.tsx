import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import { useState } from 'react';
import { Link, useParams } from 'react-router';

import { ApiError } from '../api/client';
import { useMessage } from '../api/hooks';
import { CopyButton } from '../components/CopyButton';
import { Empty, Failure, Loading } from '../components/States';
import { Facts } from '../components/Facts';
import { Mono } from '../components/Mono';
import { PageHeader } from '../components/PageHeader';
import { Section } from '../components/Section';
import { StatusMark } from '../components/StatusMark';
import { formatSize, formatDateTime, messageTitle } from '../lib/format';

export function MessageDetail() {
  const { id } = useParams();
  const messageId = Number(id);
  const message = useMessage(messageId);
  const [showRaw, setShowRaw] = useState(false);

  if (message.isPending) return <Loading label="Loading the message…" />;
  if (message.isError) {
    if (message.error instanceof ApiError && message.error.status === 404) {
      return <Empty title="That message is no longer in the log"><Link to="/messages">Back to messages</Link></Empty>;
    }
    return <Failure error={message.error} onRetry={() => void message.refetch()} />;
  }
  const m = message.data;

  return (
    <div>
      <PageHeader
        title={<span className="inline-flex items-center gap-2"><StatusMark status={m.status} size="title" />{formatDateTime(m.created_at)}</span>}
        titleText={messageTitle(m.status, m.created_at)}
        back={{ to: '/messages', label: 'Messages' }}
      />

      {(m.status === 'failed' || m.status === 'rejected') && (
        <Alert severity={m.status === 'failed' ? 'error' : 'warning'} className="mb-6">
          <p className="font-semibold">
            {m.status === 'failed' ? 'The upstream server did not accept this message' : 'The gateway refused this message'}
          </p>
          <Mono className="mt-1 block">{m.detail}</Mono>
        </Alert>
      )}

      <Section title="Envelope">
        <Facts
          items={[
            { label: 'Subject', value: m.subject || '(no subject)' },
            { label: 'From', value: <Mono>{m.mail_from}</Mono> },
            { label: 'To', value: <div>{m.rcpt_to.map((r) => <Mono key={r} className="block">{r}</Mono>)}</div> },
            {
              label: 'Login',
              value: m.credential_id !== null
                ? <Link to={`/logins/${m.credential_id}`}>{m.username}</Link>
                : '(deleted)',
            },
            { label: 'Sent from', value: <Mono>{m.remote_ip}</Mono> },
            { label: 'Size', value: formatSize(m.size) },
            ...(m.status === 'sent' ? [{ label: 'Server reply', value: <Mono>{m.detail}</Mono> }] : []),
          ]}
        />
      </Section>

      <Section title="Headers">
        {m.headers.length === 0 ? (
          <p className="text-ink2">Headers were not stored for this message.</p>
        ) : (
          <>
            <dl className="grid gap-1 sm:grid-cols-[12rem_1fr]">
              {m.headers.map((h, i) => (
                <div key={i} className="contents">
                  <dt className="font-mono text-sm text-ink2">{h.name}</dt>
                  <dd className="m-0 whitespace-pre-wrap break-words font-mono text-sm">{h.value}</dd>
                </div>
              ))}
            </dl>
            <Button variant="text" className="mt-3" onClick={() => setShowRaw((s) => !s)}>
              {showRaw ? 'Hide raw headers' : 'Show raw headers'}
            </Button>
            {showRaw && (
              <>
                <pre className="overflow-x-auto whitespace-pre border border-hairline bg-sunken p-4 text-sm font-mono">{m.headers_raw}</pre>
                <div className="mt-2">
                  <CopyButton value={m.headers_raw} label="Copy headers" />
                </div>
              </>
            )}
            {m.headers_truncated && <p className="mt-2 text-sm text-ink2">Only the first 16 KB of the headers are stored.</p>}
          </>
        )}
      </Section>

      <p className="pt-4 text-sm text-muted">The message body is never stored.</p>
    </div>
  );
}
