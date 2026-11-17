import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import { lazy, useMemo, useState } from 'react';

import { useAudit } from '../api/hooks';
import type { AuditResult } from '../api/types';
import { AuditRow } from '../components/AuditRow';
import { Empty, Failure, Loading } from '../components/States';
import { FilterBar } from '../components/FilterBar';
import { LoadMore } from '../components/LoadMore';
import { PageHeader } from '../components/PageHeader';
import { ACTION_FILTERS } from '../lib/audit';
import { toRFC3339End, toRFC3339Start } from '../lib/format';
import { useFilterParams } from '../lib/useFilterParams';

const DateField = lazy(() => import('../lib/dates').then((m) => ({ default: m.DateField })));

const OUTCOME_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'ok', label: 'Done' },
  { value: 'denied', label: 'Refused' },
  { value: 'error', label: 'Failed' },
];

export function Audit() {
  const { filter, setFilter, clear, activeCount } = useFilterParams<Record<string, string | undefined>>();
  const [search, setSearch] = useState(filter.q ?? '');

  const apiFilter = useMemo(() => ({
    action: filter.action,
    target_type: filter.target_type,
    result: filter.result as AuditResult | undefined,
    q: filter.q,
    from: filter.from ? toRFC3339Start(filter.from) : undefined,
    to: filter.to ? toRFC3339End(filter.to) : undefined,
  }), [filter]);

  const audit = useAudit(apiFilter);

  if (audit.isPending) return <Loading label="Loading the audit log…" />;
  if (audit.isError) return <Failure error={audit.error} onRetry={() => void audit.refetch()} />;

  const items = audit.data.pages.flatMap((p) => p.items);
  const hasMore = Boolean(audit.data.pages.at(-1)?.next_before);

  const summary: string[] = [];
  const actionLabel = ACTION_FILTERS.find((o) => o.value === filter.action)?.label;
  if (actionLabel && filter.action) summary.push(actionLabel);
  if (filter.result) summary.push(OUTCOME_OPTIONS.find((o) => o.value === filter.result)?.label ?? '');
  if (filter.from) summary.push(`from ${filter.from}`);
  if (filter.to) summary.push(`to ${filter.to}`);

  return (
    <div>
      <PageHeader title="Audit" description="Who did what, when and from where." />

      <FilterBar activeCount={activeCount} onClear={clear} summary={summary}>
        <TextField
          select
          label="What"
          size="small"
          value={filter.action ?? ''}
          onChange={(e) => setFilter({ action: e.target.value || undefined })}
          className="min-w-44"
        >
          {ACTION_FILTERS.map((opt) => <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>)}
        </TextField>
        <ToggleButtonGroup
          exclusive
          value={filter.result ?? ''}
          onChange={(_e, value: string | null) => setFilter({ result: value || undefined })}
          aria-label="Outcome"
        >
          {OUTCOME_OPTIONS.map((opt) => <ToggleButton key={opt.value} value={opt.value}>{opt.label}</ToggleButton>)}
        </ToggleButtonGroup>
        <DateField label="From day" value={filter.from ?? null} onChange={(day) => setFilter({ from: day ?? undefined })} />
        <DateField label="To day" value={filter.to ?? null} onChange={(day) => setFilter({ to: day ?? undefined })} />
        <TextField
          label="Search"
          size="small"
          helperText="Person, login name, address or detail"
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

      {items.length === 0 ? (
        activeCount === 0 ? (
          <Empty title="Nothing recorded yet">Actions in the panel and API appear here.</Empty>
        ) : (
          <Empty title="Nothing matches these filters">Try a different filter.</Empty>
        )
      ) : (
        <>
          <ul className="border-t border-hairline">
            {items.map((e) => <AuditRow key={e.id} event={e} />)}
          </ul>
          <LoadMore
            hasMore={hasMore}
            loading={audit.isFetchingNextPage}
            onClick={() => void audit.fetchNextPage()}
            shown={items.length}
            noun="event"
          >
            Events older than 30 days are removed.
          </LoadMore>
        </>
      )}
    </div>
  );
}
