import CheckCircleOutlinedIcon from '@mui/icons-material/CheckCircleOutlined';
import ErrorOutlineOutlinedIcon from '@mui/icons-material/ErrorOutlineOutlined';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import WarningAmberOutlinedIcon from '@mui/icons-material/WarningAmberOutlined';
import type { ReactNode } from 'react';

export type NoticeKind = 'success' | 'info' | 'warning' | 'error';

const ICON: Record<NoticeKind, typeof InfoOutlinedIcon> = {
  success: CheckCircleOutlinedIcon,
  info: InfoOutlinedIcon,
  warning: WarningAmberOutlinedIcon,
  error: ErrorOutlineOutlinedIcon,
};

export interface NoticeProps {
  kind: NoticeKind;
  children: ReactNode;
  className?: string;
}

// DESIGN.md reserves sent/rejected/failed (green, amber, red) for a result:
// a status mark, a chart bar or a count. Every other message — a saved
// confirmation, the secret-reveal warning, a sign-out or form error, the
// upstream connectivity test, a certificate renewal note — is not a result,
// so it gets this neutral ink-on-sunken treatment instead: a hairline-edged
// sheet, an icon and words. Severity is read from the icon and the wording,
// never from colour.
export function Notice({ kind, children, className = '' }: NoticeProps) {
  const Icon = ICON[kind];
  const role = kind === 'error' ? 'alert' : 'status';
  return (
    <div role={role} className={`flex items-start gap-3 border border-hairline bg-sunken p-4 text-ink ${className}`}>
      <Icon aria-hidden sx={{ fontSize: 20 }} className="mt-0.5 shrink-0 text-ink2" />
      <div className="min-w-0 text-sm">{children}</div>
    </div>
  );
}
