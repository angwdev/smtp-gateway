import Button from '@mui/material/Button';
import type { ReactNode } from 'react';

export function Loading({ label }: { label: string }) {
  return <p role="status" className="py-8 text-ink2">{label}</p>;
}

export function Empty({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="py-12">
      <p className="text-lg font-semibold">{title}</p>
      <p className="mt-2 max-w-prose text-ink2">{children}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function Failure({ error, onRetry }: { error: Error; onRetry: () => void }) {
  return (
    <div role="alert" className="py-8">
      <p className="font-semibold">Could not load this.</p>
      <p className="mt-1 text-ink2">{error.message}</p>
      <Button className="mt-4" variant="outlined" onClick={onRetry}>Try again</Button>
    </div>
  );
}
