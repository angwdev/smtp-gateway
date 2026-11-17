import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import TextField from '@mui/material/TextField';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router';

import type { ApiError } from '../api/client';
import type { CredentialCreate, CredentialPatch } from '../api/types';

export interface LoginFormValues {
  username: string;
  description: string;
  allowed_senders: string[];
  rate_per_hour: number;
  enabled: boolean;
}

export interface LoginFormProps {
  mode: 'new' | 'edit';
  initial: LoginFormValues;
  busy: boolean;
  error?: ApiError;
  onSubmit: (values: CredentialCreate | CredentialPatch) => void;
  cancelTo: string;
}

function parseSenders(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

export function LoginForm({ mode, initial, busy, error, onSubmit, cancelTo }: LoginFormProps) {
  const [username, setUsername] = useState(initial.username);
  const [description, setDescription] = useState(initial.description);
  const [sendersText, setSendersText] = useState(initial.allowed_senders.join('\n'));
  const [rate, setRate] = useState(String(initial.rate_per_hour));
  const [enabled, setEnabled] = useState(initial.enabled);
  const errorFieldRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (error?.field) errorFieldRef.current?.focus();
  }, [error]);

  const currentSenders = parseSenders(sendersText);
  const changed =
    mode === 'new' ||
    description !== initial.description ||
    currentSenders.join('\n') !== initial.allowed_senders.join('\n') ||
    Number(rate) !== initial.rate_per_hour ||
    enabled !== initial.enabled;

  const fieldError = (name: string) => (error?.field === name ? error.message : undefined);

  const submit = () => {
    if (mode === 'new') {
      onSubmit({ username, description, allowed_senders: currentSenders, rate_per_hour: Number(rate) || 0, enabled });
      return;
    }
    const patch: CredentialPatch = {};
    if (description !== initial.description) patch.description = description;
    if (currentSenders.join('\n') !== initial.allowed_senders.join('\n')) patch.allowed_senders = currentSenders;
    if (Number(rate) !== initial.rate_per_hour) patch.rate_per_hour = Number(rate) || 0;
    if (enabled !== initial.enabled) patch.enabled = enabled;
    onSubmit(patch);
  };

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      {mode === 'new' && (
        <TextField
          label="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          error={!!fieldError('username')}
          helperText={fieldError('username') ?? 'Letters, digits and . _ - + @. This is what the app signs in with.'}
          autoComplete="off"
          spellCheck={false}
          inputRef={error?.field === 'username' ? errorFieldRef : undefined}
          slotProps={{ htmlInput: { maxLength: 64, autoCapitalize: 'none' } }}
        />
      )}
      <TextField
        label="Description"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        error={!!fieldError('description')}
        helperText={fieldError('description') ?? 'Optional. What uses this login.'}
        inputRef={error?.field === 'description' ? errorFieldRef : undefined}
      />
      <TextField
        label="Allowed senders"
        value={sendersText}
        onChange={(e) => setSendersText(e.target.value)}
        multiline
        minRows={2}
        error={!!fieldError('allowed_senders')}
        helperText={
          fieldError('allowed_senders') ??
          'Optional. One per line. An address (alerts@example.com), a domain (@example.com) or a domain with subdomains (@*.example.com). Leave empty to allow any sender.'
        }
        inputRef={error?.field === 'allowed_senders' ? errorFieldRef : undefined}
      />
      <TextField
        label="Messages per hour"
        type="number"
        value={rate}
        onChange={(e) => setRate(e.target.value)}
        error={!!fieldError('rate_per_hour')}
        helperText={fieldError('rate_per_hour') ?? '0 means no limit.'}
        inputRef={error?.field === 'rate_per_hour' ? errorFieldRef : undefined}
        slotProps={{ htmlInput: { min: 0, inputMode: 'numeric' } }}
      />
      <FormControlLabel
        control={<Switch checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />}
        label="Turned on"
      />
      <p className="text-sm text-ink2">When off, sign-in is refused and nothing is sent.</p>
      {error && !error.field && <Alert severity="error">{error.message}</Alert>}
      <div className="flex items-center gap-3">
        <Button type="submit" variant="contained" loading={busy} disabled={mode === 'edit' && !changed}>
          {mode === 'new' ? 'Create login' : 'Save changes'}
        </Button>
        <Button variant="text" component={Link} to={cancelTo}>Cancel</Button>
        {mode === 'edit' && !changed && <span className="text-sm text-ink2">No changes to save.</span>}
      </div>
    </form>
  );
}
