import ErrorOutlineIcon from '@mui/icons-material/ErrorOutlineOutlined';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import TextField from '@mui/material/TextField';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import { Suspense, lazy, useState } from 'react';

import { ApiError } from '../api/client';
import { useCreateToken, useRevokeToken, useTokens } from '../api/hooks';
import type { Scope, TokenSecret } from '../api/types';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { Empty, Failure, Loading } from '../components/States';
import { Mono } from '../components/Mono';
import { PageHeader } from '../components/PageHeader';
import { SecretReveal } from '../components/SecretReveal';
import { CopyButton } from '../components/CopyButton';
import { formatDateTime, formatRelative, toRFC3339End } from '../lib/format';

const DateField = lazy(() => import('../lib/dates').then((m) => ({ default: m.DateField })));

function expiryNote(expiresAt: string | null, revokedAt: string | null): { text: string; failed: boolean } {
  if (revokedAt) return { text: `Revoked ${formatDateTime(revokedAt)}`, failed: false };
  if (!expiresAt) return { text: 'Does not expire', failed: false };
  const expired = new Date(expiresAt).getTime() < Date.now();
  return { text: `${expired ? 'Expired' : 'Expires'} ${formatDateTime(expiresAt)}`, failed: expired };
}

export function Tokens() {
  const tokens = useTokens();
  const createToken = useCreateToken();
  const revokeToken = useRevokeToken();

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [scope, setScope] = useState<Scope>('read');
  const [expiresDay, setExpiresDay] = useState<string | null>(null);
  const [created, setCreated] = useState<TokenSecret | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<{ id: number; name: string } | null>(null);

  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

  if (tokens.isPending) return <Loading label="Loading the tokens…" />;
  if (tokens.isError) {
    if (tokens.error instanceof ApiError && tokens.error.status === 403) {
      return <Empty title="Sign in as a person to manage tokens">Tokens cannot manage other tokens.</Empty>;
    }
    return <Failure error={tokens.error} onRetry={() => void tokens.refetch()} />;
  }

  const active = tokens.data.filter((t) => !t.revoked_at);
  const revoked = tokens.data.filter((t) => t.revoked_at);

  const closeCreate = () => {
    setCreateOpen(false);
    setName('');
    setScope('read');
    setExpiresDay(null);
    setCreated(null);
    createToken.reset();
  };

  return (
    <div>
      <PageHeader
        title="API tokens"
        description="Let a script or dashboard read the logs or manage logins without signing in. Every call made with a token is written to the audit log."
        action={<Button variant="contained" onClick={() => setCreateOpen(true)}>New token</Button>}
      />

      {tokens.data.length === 0 ? (
        <Empty title="No API tokens">Create one when a script needs to read the logs or manage logins.</Empty>
      ) : (
        <ul className="border-t border-hairline">
          {active.map((t) => {
            const expiry = expiryNote(t.expires_at, t.revoked_at);
            return (
              <li key={t.id} className="flex items-start justify-between gap-3 border-b border-hairline py-3">
                <div>
                  <p className="font-semibold">{t.name}</p>
                  <p className="text-sm text-ink2">
                    {t.scope === 'write' ? 'Read and change' : 'Read only'} · ends in <Mono>{t.hint}</Mono>
                  </p>
                  <p className="text-sm text-ink2">Created {formatDateTime(t.created_at)} by {t.created_by}</p>
                  <p className="text-sm text-ink2">Last used {formatRelative(t.last_used_at)}{t.last_used_ip ? ` from ${t.last_used_ip}` : ''}</p>
                  <p className={`text-sm ${expiry.failed ? 'text-failed' : 'text-ink2'}`}>
                    {expiry.failed && <ErrorOutlineIcon sx={{ fontSize: 14 }} aria-hidden />} {expiry.text}
                  </p>
                </div>
                <Button variant="outlined" color="error" onClick={() => setRevokeTarget({ id: t.id, name: t.name })}>Revoke</Button>
              </li>
            );
          })}
          {revoked.length > 0 && (
            <>
              <li className="border-b border-hairline py-3"><h2 className="text-lg font-semibold">Revoked</h2></li>
              {revoked.map((t) => {
                const expiry = expiryNote(t.expires_at, t.revoked_at);
                return (
                  <li key={t.id} className="border-b border-hairline py-3 text-ink2">
                    <p className="font-semibold">{t.name}</p>
                    <p className="text-sm">{expiry.text}</p>
                  </li>
                );
              })}
            </>
          )}
        </ul>
      )}

      <Dialog open={createOpen} onClose={closeCreate} fullWidth maxWidth="xs" fullScreen={fullScreen}>
        <DialogTitle>New token</DialogTitle>
        <DialogContent>
          {created ? (
            <SecretReveal title={`Token for ${created.token.name}`} secret={created.secret} copyLabel="Copy token" doneLabel="Done" onDone={closeCreate}>
              <pre className="mt-3 overflow-x-auto whitespace-pre border border-hairline bg-sunken p-4 text-sm font-mono">
                {`curl -H "Authorization: Bearer ***" ${window.location.origin}/api/v1/messages?limit=5`}
              </pre>
              <div className="mt-2">
                <CopyButton value={`curl -H "Authorization: Bearer ***" ${window.location.origin}/api/v1/messages?limit=5`} label="Copy example" />
              </div>
              <p className="mt-3"><a href="/api/v1/openapi.yaml">API reference (OpenAPI)</a></p>
            </SecretReveal>
          ) : (
            <div className="flex flex-col gap-4">
              <TextField
                label="Name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                helperText={createToken.error instanceof ApiError && createToken.error.status === 409 ? 'A token with this name already exists.' : 'What will use it, for example grafana.'}
                error={createToken.error instanceof ApiError && createToken.error.status === 409}
              />
              <RadioGroup value={scope} onChange={(e) => setScope(e.target.value as Scope)}>
                <FormControlLabel value="read" control={<Radio />} label="Read only: overview, logins, messages, audit" />
                <FormControlLabel value="write" control={<Radio />} label="Read and change: also create, change and delete logins" />
              </RadioGroup>
              <Suspense fallback={null}>
                <DateField label="Expires" value={expiresDay} onChange={setExpiresDay} />
              </Suspense>
              <p className="text-sm text-ink2">Leave empty for a token that does not expire.</p>
            </div>
          )}
        </DialogContent>
        {!created && (
          <DialogActions>
            <Button variant="text" onClick={closeCreate}>Cancel</Button>
            <Button
              variant="contained"
              loading={createToken.isPending}
              onClick={() => createToken.mutate(
                { name, scope, expires_at: expiresDay ? toRFC3339End(expiresDay) : null },
                { onSuccess: setCreated },
              )}
            >
              Create token
            </Button>
          </DialogActions>
        )}
      </Dialog>

      <ConfirmDialog
        open={revokeTarget !== null}
        title={`Revoke ${revokeTarget?.name}?`}
        confirmLabel="Revoke token"
        destructive
        busy={revokeToken.isPending}
        onClose={() => setRevokeTarget(null)}
        onConfirm={() => {
          if (revokeTarget) revokeToken.mutate(revokeTarget.id, { onSuccess: () => setRevokeTarget(null) });
        }}
      >
        Anything using this token will be refused at once.
      </ConfirmDialog>
    </div>
  );
}
