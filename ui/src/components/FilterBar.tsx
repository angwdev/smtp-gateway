import FilterListIcon from '@mui/icons-material/FilterListOutlined';
import Button from '@mui/material/Button';
import Drawer from '@mui/material/Drawer';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { Suspense, useState } from 'react';

export interface FilterBarProps {
  children: ReactNode;
  activeCount: number;
  onClear: () => void;
  summary: string[];
}

export function FilterBar({ children, activeCount, onClear, summary }: FilterBarProps) {
  const theme = useTheme();
  const wide = useMediaQuery(theme.breakpoints.up('lg'));
  const [open, setOpen] = useState(false);

  if (wide) {
    return (
      <div className="flex flex-wrap items-end gap-3 border-b border-hairline pb-4">
        <Suspense fallback={null}>{children}</Suspense>
        {activeCount > 0 && <Button variant="text" onClick={onClear}>Clear filters</Button>}
      </div>
    );
  }

  return (
    <div className="border-b border-hairline pb-4">
      <Button variant="outlined" startIcon={<FilterListIcon />} onClick={() => setOpen(true)}>
        Filter{activeCount > 0 ? ` (${activeCount})` : ''}
      </Button>
      {summary.length > 0 && <p className="mt-2 text-sm text-ink2">{summary.join(' · ')}</p>}
      <Drawer
        anchor="bottom"
        open={open}
        onClose={() => setOpen(false)}
        slotProps={{ paper: { className: 'bg-sheet border-t border-hairline max-h-[85dvh] overflow-auto p-4 pb-[calc(1rem+env(safe-area-inset-bottom))]' } }}
      >
        <h2 className="mb-4 text-lg font-semibold">Filter</h2>
        <div className="flex flex-col gap-4">
          <Suspense fallback={null}>{children}</Suspense>
        </div>
        <div className="mt-4 flex items-center gap-3">
          <Button variant="contained" onClick={() => setOpen(false)}>Show results</Button>
          {activeCount > 0 && <Button variant="text" onClick={onClear}>Clear filters</Button>}
        </div>
      </Drawer>
    </div>
  );
}
