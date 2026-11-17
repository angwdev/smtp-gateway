import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import type { ReactNode } from 'react';

import { CopyButton } from './CopyButton';

export interface SecretRevealProps {
  title: string;
  secret: string;
  children?: ReactNode;
  copyLabel?: string;
  doneLabel: string;
  onDone: () => void;
}

export function SecretReveal({ title, secret, children, copyLabel = 'Copy password', doneLabel, onDone }: SecretRevealProps) {
  return (
    <div>
      <h2 className="text-lg font-semibold">{title}</h2>
      <Alert severity="warning" className="mt-3">
        This is shown once. Copy it now: it cannot be shown again.
      </Alert>
      <div className="mt-3 select-all break-all border border-hairline bg-sunken p-4 font-mono text-lg">{secret}</div>
      <div className="mt-3">
        <CopyButton value={secret} label={copyLabel} />
      </div>
      {children}
      <Button variant="contained" className="mt-4" onClick={onDone}>{doneLabel}</Button>
    </div>
  );
}
