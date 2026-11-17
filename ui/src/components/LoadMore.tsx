import Button from '@mui/material/Button';
import type { ReactNode } from 'react';

import { formatCount } from '../lib/format';

export interface LoadMoreProps {
  hasMore: boolean;
  loading: boolean;
  onClick: () => void;
  shown: number;
  noun: string;
  children?: ReactNode;
}

export function LoadMore({ hasMore, loading, onClick, shown, noun, children }: LoadMoreProps) {
  if (hasMore) {
    return (
      <div className="pt-4">
        <Button variant="outlined" className="w-full sm:w-auto" loading={loading} onClick={onClick}>
          Show older
        </Button>
        <p className="mt-2 text-sm text-muted">Showing {formatCount(shown, noun)}</p>
      </div>
    );
  }
  if (shown > 0) {
    return <p className="pt-4 text-sm text-muted">That is everything. {children}</p>;
  }
  return null;
}
