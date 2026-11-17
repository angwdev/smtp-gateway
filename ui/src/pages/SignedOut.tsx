import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';

export function SignedOut() {
  const error = new URLSearchParams(window.location.hash.slice(1)).get('error');

  if (error) {
    return (
      <div className="mx-auto max-w-prose p-6">
        <h1 className="text-2xl font-semibold">Sign-in did not work</h1>
        <Alert className="mt-4" severity="error">{error}</Alert>
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
