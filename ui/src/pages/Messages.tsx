import RefreshIcon from '@mui/icons-material/RefreshOutlined';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import { lazy, useMemo, useState } from 'react';

import { useCredentials, useMessages } from '../api/hooks';
import type { MessageStatus } from '../api/types';
import { Empty, Failure, Loading } from '../components/States';
import { FilterBar } from '../components/FilterBar';
import { LoadMore } from '../components/LoadMore';
import { MessageRow } from '../components/MessageRow';
import { PageHeader } from '../components/PageHeader';
import { toRFC3339End, toRFC3339Start } from '../lib/format';
import { useFilterParams } from '../lib/useFilterParams';

const DateField = lazy(() => import('../lib/dates').then((m) => ({ default: m.DateField })));

const RESULT_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'sent', label: 'Sent' },
  { value: 'rejected', label: 'Rejected' },
  { value: 'failed', label: 'Failed' },
];

export function Messages() {
  const { filter, setFilter, clear, activeCount } = useFilterParams<Record<string, string | undefined>>();
  const credentials = useCredentials();
  const [search, setSearch] = useState(filter.q ?? '');

  const apiFilter = useMemo(() => ({
    status: filter.status as MessageStatus | undefined,
    credential_id: filter.credential_id ? Number(filter.credential_id) : undefined,
    q: filter.q,
    from: filter.from ? toRFC3339Start(filter.from) : undefined,
    to: filter.to ? toRFC3339End(filter.to) : undefined,
  }), [filter]);

  const messages = useMessages(apiFilter);

  if (messages.isPending) return <Loading label="Loading the messages…" />;
  if (messages.isError) return <Failure error={messages.error} onRetry={() => void messages.refetch()} />;

  const items = messages.data.pages.flatMap((p) => p.items);
  const hasMore = Boolean(messages.data.pages.at(-1)?.next_before);

  const credentialName = (id: string | undefined) => credentials.data?.find((c) => String(c.id) === id)?.username;
  const summary: string[] = [];
  if (filter.status) summary.push(filter.status.charAt(0).toUpperCase() + filter.status.slice(1));
  if (filter.credential_id) summary.push(credentialName(filter.credential_id) ?? '');
  if (filter.from) summary.push(`from ${filter.from}`);
  if (filter.to) summary.push(`to ${filter.to}`);

  return (
    <div>
      <PageHeader title="Messages" description="Every message the gateway accepted or refused." />

      <div className="mb-4 flex items-center justify-between gap-3">
        <FilterBar activeCount={activeCount} onClear={clear} summary={summary}>
          <ToggleButtonGroup
            exclusive
            value={filter.status ?? ''}
            onChange={(_e, value: string | null) => setFilter({ status: value || undefined })}
            aria-label="Result"
          >
            {RESULT_OPTIONS.map((opt) => <ToggleButton key={opt.value} value={opt.value}>{opt.label}</ToggleButton>)}
          </ToggleButtonGroup>
          <TextField
            select
            label="Login"
            size="small"
            value={filter.credential_id ?? ''}
            onChange={(e) => setFilter({ credential_id: e.target.value || undefined })}
            className="min-w-40"
          >
            <MenuItem value="">Any login</MenuItem>
            {credentials.data?.map((c) => <MenuItem key={c.id} value={String(c.id)}>{c.username}</MenuItem>)}
          </TextField>
          <DateField label="From day" value={filter.from ?? null} onChange={(day) => setFilter({ from: day ?? undefined })} />
          <DateField label="To day" value={filter.to ?? null} onChange={(day) => setFilter({ to: day ?? undefined })} />
          <TextField
            label="Search"
            size="small"
            helperText="Sender, recipient, subject or server reply"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setFilter({ q: e.target.value || undefined }, { debounceQ: true });
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') setFilter({ q: search || undefined });
            }}
          />
        </FilterBar>
        <Button
          variant="text"
          startIcon={<RefreshIcon />}
          onClick={() => void messages.refetch()}
        >
          {messages.isFetching ? 'Refreshing…' : 'Refresh'}
        </Button>
      </div>

      {items.length === 0 ? (
        activeCount === 0 ? (
          <Empty title="No messages yet">Messages appear here as soon as an app sends through the gateway.</Empty>
        ) : (
          <Empty title="Nothing matches these filters" action={<Button variant="text" onClick={clear}>Clear filters</Button>}>
            Try a different filter.
          </Empty>
        )
      ) : (
        <>
          <ul className="border-t border-hairline">
            {items.map((m) => <MessageRow key={m.id} message={m} />)}
          </ul>
          <LoadMore
            hasMore={hasMore}
            loading={messages.isFetchingNextPage}
            onClick={() => void messages.fetchNextPage()}
            shown={items.length}
            noun="message"
          >
            Messages older than 30 days are removed.
          </LoadMore>
        </>
      )}
    </div>
  );
}
