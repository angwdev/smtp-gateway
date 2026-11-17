import BlockOutlinedIcon from '@mui/icons-material/BlockOutlined';
import CheckCircleOutlinedIcon from '@mui/icons-material/CheckCircleOutlined';
import ErrorOutlineOutlinedIcon from '@mui/icons-material/ErrorOutlineOutlined';

import type { MessageStatus } from '../api/types';
import { STATUS_WORD } from '../lib/format';

const ICON: Record<MessageStatus, typeof CheckCircleOutlinedIcon> = {
  sent: CheckCircleOutlinedIcon,
  failed: ErrorOutlineOutlinedIcon,
  rejected: BlockOutlinedIcon,
};
const COLOR: Record<MessageStatus, string> = { sent: 'text-sent', failed: 'text-failed', rejected: 'text-rejected' };

export interface StatusMarkProps {
  status: MessageStatus;
  size?: 'row' | 'title';
  // Hides the visible word but keeps it for assistive tech (used in dense rows
  // that already carry the word elsewhere).
  wordHidden?: boolean;
}

export function StatusMark({ status, size = 'row', wordHidden = false }: StatusMarkProps) {
  const Icon = ICON[status];
  const sizePx = size === 'title' ? 24 : 18;
  return (
    <span className={`inline-flex items-center gap-1.5 font-semibold ${COLOR[status]}`}>
      <Icon aria-hidden sx={{ fontSize: sizePx }} />
      <span className={wordHidden ? 'sr-only' : undefined}>{STATUS_WORD[status]}</span>
    </span>
  );
}
