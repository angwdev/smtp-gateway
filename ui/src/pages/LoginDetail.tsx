import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import { useState } from 'react';
import { Link, useParams, useNavigate } from 'react-router';

import { ApiError } from '../api/client';
import {
  useCredential, useDeleteCredential, useMessages, useRotateCredential, useUpdateCredential,
} from '../api/hooks';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { Empty, Failure, Loading } from '../components/States';
import { LoginForm } from '../components/LoginForm';
import { MessageRow } from '../components/MessageRow';
import { PageHeader } from '../components/PageHeader';
import { SecretReveal } from '../components/SecretReveal';
import { Section } from '../components/Section';
import { formatDateTime, formatRelative } from '../lib/format';

export function LoginDetail() {
  const { id } = useParams();
  const credentialId = Number(id);
  const navigate = useNavigate();
  const credential = useCredential(credentialId);
  const updateCredential = useUpdateCredential(credentialId);
  const rotateCredential = useRotateCredential(credentialId);
  const deleteCredential = useDeleteCredential(credentialId);
  const recentMessages = useMessages({ credential_id: credentialId });

  const [saved, setSaved] = useState(false);
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotatedSecret, setRotatedSecret] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (credential.isPending) return <Loading label="Loading the login…" />;
  if (credential.isError) {
    if (credential.error instanceof ApiError && credential.error.status === 404) {
      return <Empty title="That login no longer exists"><Link to="/logins">Back to logins</Link></Empty>;
    }
    return <Failure error={credential.error} onRetry={() => void credential.refetch()} />;
  }
  const c = credential.data;

  return (
    <div>
      <PageHeader
        title={c.username}
        back={{ to: '/logins', label: 'Logins' }}
        description={`Created ${formatDateTime(c.created_at)} by ${c.created_by}. Last used ${formatRelative(c.last_used_at)}${c.last_used_ip ? ` from ${c.last_used_ip}` : ''}.`}
      />

      <Section title="Settings">
        {saved && <Alert severity="success" className="mb-4">Saved.</Alert>}
        <LoginForm
          mode="edit"
          initial={{
            username: c.username, description: c.description, allowed_senders: c.allowed_senders,
            rate_per_hour: c.rate_per_hour, enabled: c.enabled,
          }}
          busy={updateCredential.isPending}
          error={updateCredential.error instanceof ApiError ? updateCredential.error : undefined}
          cancelTo="/logins"
          onSubmit={(patch) => updateCredential.mutate(patch, { onSuccess: () => setSaved(true) })}
        />
      </Section>

      <Section title="Password">
        <p className="text-ink2">
          Replacing the password stops the old one working at once. The app using this login will fail until you give it the new one.
        </p>
        {rotatedSecret ? (
          <SecretReveal
            title={`New password for ${c.username}`}
            secret={rotatedSecret}
            doneLabel="I have copied it"
            onDone={() => setRotatedSecret(null)}
          />
        ) : (
          <Button variant="outlined" className="mt-3" onClick={() => setRotateOpen(true)}>Replace password</Button>
        )}
        <ConfirmDialog
          open={rotateOpen}
          title={`Replace the password for ${c.username}?`}
          confirmLabel="Replace password"
          destructive
          busy={rotateCredential.isPending}
          error={rotateCredential.error instanceof ApiError ? rotateCredential.error.message : undefined}
          onClose={() => setRotateOpen(false)}
          onConfirm={() => rotateCredential.mutate(undefined, {
            onSuccess: (data) => {
              setRotatedSecret(data.secret);
              setRotateOpen(false);
            },
          })}
        >
          The app using this login will fail until you give it the new password.
        </ConfirmDialog>
      </Section>

      <Section title="Recent messages">
        {recentMessages.data?.pages[0]?.items.length ? (
          <>
            <ul className="border-t border-hairline">
              {recentMessages.data.pages[0].items.slice(0, 10).map((m) => (
                <MessageRow key={m.id} message={m} />
              ))}
            </ul>
            <Link to={`/messages?credential_id=${credentialId}`} className="mt-3 inline-block">All messages from this login</Link>
          </>
        ) : (
          <p className="text-ink2">Nothing sent with this login yet.</p>
        )}
      </Section>

      <Section title="Delete">
        <p className="text-ink2">Deleting removes the login. Its messages stay in the log until they expire.</p>
        <Button variant="outlined" color="error" className="mt-3" onClick={() => setDeleteOpen(true)}>Delete login</Button>
        <ConfirmDialog
          open={deleteOpen}
          title={`Delete ${c.username}?`}
          confirmLabel="Delete login"
          destructive
          busy={deleteCredential.isPending}
          error={deleteCredential.error instanceof ApiError ? deleteCredential.error.message : undefined}
          onClose={() => setDeleteOpen(false)}
          onConfirm={() => deleteCredential.mutate(undefined, { onSuccess: () => navigate('/logins') })}
        >
          Apps using this login will stop sending mail. This cannot be undone.
        </ConfirmDialog>
      </Section>
    </div>
  );
}
