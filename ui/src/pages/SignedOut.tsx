import Button from '@mui/material/Button';
import { useEffect } from 'react';

import { Notice } from '../components/Notice';

export function SignedOut() {
  const error = new URLSearchParams(window.location.hash.slice(1)).get('error');

  useEffect(() => {
    document.title = `${error ? 'Sign-in did not work' : "You're signed out"} · SMTP Gateway`;
  }, [error]);

  if (error) {
    return (
      <div className="mx-auto max-w-prose p-6">
        <h1 className="text-2xl font-semibold">Sign-in did not work</h1>
        <Notice kind="error" className="mt-4">{error}</Notice>
        <Button className="mt-4" variant="contained" component="a" nativeButton={false} href="/login">
          Try again
        </Button>
      </div>
    );
  }
  return (
    <div className="mx-auto max-w-prose p-6">
      <h1 className="text-2xl font-semibold">You&apos;re signed out</h1>
      <p className="mt-2 text-ink2">Sign in again to manage logins and read the logs.</p>
      <Button className="mt-4" variant="contained" component="a" nativeButton={false} href="/login">
        Sign in
      </Button>
    </div>
  );
}
