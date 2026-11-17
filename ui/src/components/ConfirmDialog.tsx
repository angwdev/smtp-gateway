import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import { useTheme } from '@mui/material/styles';
import useMediaQuery from '@mui/material/useMediaQuery';
import type { ReactNode } from 'react';
import { useEffect, useRef } from 'react';

export interface ConfirmDialogProps {
  open: boolean;
  title: string;
  children: ReactNode;
  confirmLabel: string;
  destructive?: boolean;
  busy?: boolean;
  error?: string;
  onConfirm: () => void;
  onClose: () => void;
}

export function ConfirmDialog({
  open, title, children, confirmLabel, destructive = false, busy = false, error, onConfirm, onClose,
}: ConfirmDialogProps) {
  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
  const cancelRef = useRef<HTMLButtonElement>(null);
  const firedRef = useRef(false);

  // Reset the once-guard every time the dialog opens.
  useEffect(() => {
    if (open) firedRef.current = false;
  }, [open]);

  const confirmOnce = () => {
    if (firedRef.current || busy) return;
    firedRef.current = true;
    onConfirm();
  };

  return (
    <Dialog
      open={open}
      onClose={busy ? undefined : onClose}
      fullWidth
      maxWidth="xs"
      fullScreen={fullScreen}
      slotProps={{ transition: { onEntered: () => { if (destructive) cancelRef.current?.focus(); } } }}
    >
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {children}
        {error && <Alert severity="error" className="mt-3">{error}</Alert>}
      </DialogContent>
      <DialogActions>
        <Button ref={cancelRef} variant="text" onClick={onClose} disabled={busy}>Cancel</Button>
        <Button variant="contained" color={destructive ? 'error' : 'primary'} loading={busy} onClick={confirmOnce}>
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
