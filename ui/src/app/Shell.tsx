import Button from '@mui/material/Button';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect, Suspense } from 'react';
import { Link, Outlet, useLocation } from 'react-router';

import { useSession, useSignOut } from '../api/hooks';
import { Notice } from '../components/Notice';
import { NAV, isActive } from './nav';
import { ThemeSwitch } from './ThemeSwitch';

// Shared by the rail and the More page: signs out, clears all cached data so
// a later sign-in never shows a previous user's data, then follows the
// server's redirect.
export function useSignOutAction() {
  const qc = useQueryClient();
  const mutation = useSignOut();
  const signOut = () => {
    mutation.mutate(undefined, {
      onSuccess: (data) => {
        qc.clear();
        window.location.assign(data.redirect_url);
      },
    });
  };
  return { signOut, isPending: mutation.isPending, error: mutation.error };
}

function railLinkClass(active: boolean): string {
  const base = 'flex min-h-11 items-center gap-3 px-6';
  return active ? `${base} font-semibold border-l-[3px] border-accent pl-[21px]` : `${base} text-ink2 hover:text-ink`;
}

function tabLinkClass(active: boolean): string {
  const base = 'flex min-h-14 flex-col items-center justify-center gap-0.5 text-xs border-t-[3px]';
  return active ? `${base} font-semibold text-ink border-accent` : `${base} text-ink2 border-transparent`;
}

export function Shell() {
  const location = useLocation();
  const session = useSession();
  const signOutAction = useSignOutAction();
  const railItems = NAV.filter((item) => item.where === 'both' || item.where === 'rail');
  const tabItems = NAV.filter((item) => item.where === 'both' || item.where === 'tabs');

  useEffect(() => {
    document.getElementById('main')?.focus();
    window.scrollTo(0, 0);
  }, [location.pathname]);

  return (
    <div className="min-h-dvh bg-paper text-ink lg:grid lg:grid-cols-[232px_1fr]">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:bg-sheet focus:px-3 focus:py-2"
      >
        Skip to content
      </a>

      <nav
        aria-label="Main"
        className="hidden lg:flex lg:flex-col lg:sticky lg:top-0 lg:h-dvh lg:border-r lg:border-hairline lg:bg-sheet"
      >
        <div className="p-6 text-lg font-semibold">SMTP Gateway</div>
        {railItems.map((item) => {
          const active = isActive(item, location.pathname);
          return (
            <Link key={item.to} to={item.to} className={railLinkClass(active)} aria-current={active ? 'page' : undefined}>
              <item.icon sx={{ fontSize: 20 }} aria-hidden />
              {item.label}
            </Link>
          );
        })}
        <div className="mt-auto flex flex-col gap-3 border-t border-hairline p-4">
          {session.data?.user && (
            <p className="truncate text-sm" title={session.data.user.email}>{session.data.user.name}</p>
          )}
          <ThemeSwitch />
          <Button variant="text" onClick={signOutAction.signOut} disabled={signOutAction.isPending}>
            Sign out
          </Button>
        </div>
      </nav>

      <div>
        <header className="sticky top-0 z-10 flex min-h-14 items-center border-b border-hairline bg-sheet px-4 lg:hidden">
          <span className="text-lg font-semibold">SMTP Gateway</span>
        </header>

        <main id="main" tabIndex={-1} className="mx-auto w-full max-w-[1120px] px-4 pb-24 pt-6 sm:px-6 lg:px-8 lg:pb-12">
          {signOutAction.error && (
            <Notice kind="error" className="mb-6">{signOutAction.error.message}</Notice>
          )}
          <Suspense fallback={<p role="status" className="text-ink2">Loading…</p>}>
            <Outlet />
          </Suspense>
        </main>
      </div>

      <nav
        aria-label="Main"
        className="fixed inset-x-0 bottom-0 z-10 grid grid-cols-5 border-t border-hairline bg-sheet pb-[env(safe-area-inset-bottom)] lg:hidden"
      >
        {tabItems.map((item) => {
          const active = isActive(item, location.pathname);
          return (
            <Link key={item.to} to={item.to} className={tabLinkClass(active)} aria-current={active ? 'page' : undefined}>
              <item.icon sx={{ fontSize: 22 }} aria-hidden />
              {item.label}
            </Link>
          );
        })}
      </nav>
    </div>
  );
}
