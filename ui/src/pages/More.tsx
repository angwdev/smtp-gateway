import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import Button from '@mui/material/Button';
import { Link } from 'react-router';

import { useOverview, useSession } from '../api/hooks';
import { useSignOutAction } from '../app/Shell';
import { ThemeSwitch } from '../app/ThemeSwitch';
import { PageHeader } from '../components/PageHeader';

export function More() {
  const session = useSession();
  const overview = useOverview();
  const signOutAction = useSignOutAction();

  return (
    <div>
      <PageHeader title="More" />
      <ul className="border-t border-hairline">
        <li className="border-b border-hairline">
          <Link to="/tokens" className="flex min-h-14 items-center justify-between gap-3 py-3">
            API tokens
            <ChevronRightIcon aria-hidden />
          </Link>
        </li>
        <li className="flex min-h-14 items-center justify-between gap-3 border-b border-hairline py-3">
          Theme
          <ThemeSwitch />
        </li>
        {session.data?.user && (
          <li className="border-b border-hairline py-3">
            <p>Signed in as</p>
            <p className="text-ink2">{session.data.user.name} · {session.data.user.email}</p>
          </li>
        )}
        <li className="border-b border-hairline">
          <a href="/api/v1/openapi.yaml" className="flex min-h-14 items-center justify-between gap-3 py-3">
            API reference (OpenAPI)
            <ChevronRightIcon aria-hidden />
          </a>
        </li>
      </ul>
      <Button variant="outlined" className="mt-4 w-full" onClick={signOutAction.signOut} disabled={signOutAction.isPending}>
        Sign out
      </Button>
      {overview.data && <p className="mt-6 text-sm text-muted">Version {overview.data.version}</p>}
    </div>
  );
}
