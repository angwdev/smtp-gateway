import Button from '@mui/material/Button';
import type { ReactNode } from 'react';
import { useEffect } from 'react';

import { ApiError, setUnauthorizedHandler } from '../api/client';
import { useSession } from '../api/hooks';

// Sends the browser to the server-side sign-in, remembering where it was.
export function goToSignIn(): void {
  const here = window.location.pathname + window.location.search;
  window.location.assign(`/login?next=${encodeURIComponent(here)}`);
}

export function AuthGate({ children }: { children: ReactNode }) {
  const session = useSession();

  // Any later 401 (the session expired or was revoked) also goes to sign-in.
  useEffect(() => {
    setUnauthorizedHandler(goToSignIn);
    return () => setUnauthorizedHandler(() => undefined);
  }, []);

  if (session.isPending) {
    return <p className="p-6 text-ink2" role="status">Checking your sign-in…</p>;
  }
  if (session.isError) {
    if (session.error instanceof ApiError && session.error.status === 401) {
      return <p className="p-6 text-ink2" role="status">Taking you to sign in…</p>;
    }
    return (
      <div className="mx-auto max-w-prose p-6" role="alert">
        <h1 className="text-2xl font-semibold">The gateway did not answer</h1>
        <p className="mt-2 text-ink2">{session.error.message}</p>
        <Button className="mt-4" variant="contained" onClick={() => void session.refetch()}>Try again</Button>
      </div>
    );
  }
  return children;
}
