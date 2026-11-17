import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutlineOutlined';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import { useState } from 'react';
import { Link } from 'react-router';

import { useCredentials } from '../api/hooks';
import { Empty, Failure, Loading } from '../components/States';
import { Mono } from '../components/Mono';
import { PageHeader } from '../components/PageHeader';
import { formatCount, formatRelative } from '../lib/format';

export function Logins() {
  const credentials = useCredentials();
  const [query, setQuery] = useState('');

  if (credentials.isPending) return <Loading label="Loading the logins…" />;
  if (credentials.isError) return <Failure error={credentials.error} onRetry={() => void credentials.refetch()} />;

  const items = credentials.data;
  const sorted = [...items].sort((a, b) => {
    if (a.enabled !== b.enabled) return a.enabled ? -1 : 1;
    return a.username.localeCompare(b.username);
  });
  const filtered = query
    ? sorted.filter((c) => `${c.username} ${c.description}`.toLowerCase().includes(query.toLowerCase()))
    : sorted;

  return (
    <div>
      <PageHeader
        title="Logins"
        description="One username and password per app or device."
        action={<Button component={Link} to="/logins/new" variant="contained">New login</Button>}
      />

      {items.length === 0 ? (
        <Empty title="Create the first login" action={<Button component={Link} to="/logins/new" variant="contained">New login</Button>}>
          Each app or device gets its own username and password. Mail is forwarded through your upstream account.
        </Empty>
      ) : (
        <>
          {items.length > 8 && (
            <TextField
              label="Find a login"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              size="small"
              className="mb-4"
            />
          )}
          <ul className="border-t border-hairline">
            {filtered.map((c) => (
              <li key={c.id} className="border-b border-hairline">
                <Link
                  to={`/logins/${c.id}`}
                  className={`flex min-h-14 items-center gap-3 py-3 lg:min-h-11 ${c.enabled ? '' : 'text-ink2'}`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {!c.enabled && <PauseCircleOutlineIcon sx={{ fontSize: 18 }} aria-hidden />}
                      {!c.enabled && <span>Off</span>}
                      <Mono className="font-semibold">{c.username}</Mono>
                      <span className="hidden text-ink2 lg:inline">{c.description}</span>
                    </div>
                    <div className="text-sm text-ink2 lg:hidden">{c.description}</div>
                  </div>
                  <div className="shrink-0 text-right text-sm text-muted">
                    <div>Last used {formatRelative(c.last_used_at)}</div>
                    <div className="hidden sm:block">{formatCount(c.sent_count, 'message')}</div>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
