import { useState } from 'react';

import { useCreateCredential } from '../api/hooks';
import { ApiError } from '../api/client';
import { ClientSettings } from '../components/ClientSettings';
import { LoginForm } from '../components/LoginForm';
import { PageHeader } from '../components/PageHeader';
import { SecretReveal } from '../components/SecretReveal';
import type { CredentialCreate, CredentialSecret } from '../api/types';

export function LoginNew() {
  const createCredential = useCreateCredential();
  const [created, setCreated] = useState<CredentialSecret | null>(null);

  if (created) {
    return (
      <div>
        <PageHeader title="New login" back={{ to: '/logins', label: 'Logins' }} />
        <SecretReveal
          title={`Password for ${created.credential.username}`}
          secret={created.secret}
          doneLabel="Done"
          onDone={() => {
            window.location.assign(`/logins/${created.credential.id}`);
          }}
        >
          <div className="mt-4">
            <ClientSettings username={created.credential.username} />
          </div>
        </SecretReveal>
      </div>
    );
  }

  return (
    <div>
      <PageHeader title="New login" back={{ to: '/logins', label: 'Logins' }} />
      <LoginForm
        mode="new"
        initial={{ username: '', description: '', allowed_senders: [], rate_per_hour: 0, enabled: true }}
        busy={createCredential.isPending}
        error={createCredential.error instanceof ApiError ? createCredential.error : undefined}
        cancelTo="/logins"
        onSubmit={(values) => createCredential.mutate(values as CredentialCreate, { onSuccess: setCreated })}
      />
    </div>
  );
}
