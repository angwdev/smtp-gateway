import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import { Link } from 'react-router';

import type { Message } from '../api/types';
import { formatSize, formatTime, recipients } from '../lib/format';
import { Mono } from './Mono';
import { StatusMark } from './StatusMark';

export function MessageRow({ message: m }: { message: Message }) {
  const detailColor = m.status === 'failed' ? 'text-failed' : m.status === 'rejected' ? 'text-rejected' : '';
  return (
    <li className="border-b border-hairline">
      <Link to={`/messages/${m.id}`} className="block min-h-14 py-3 lg:min-h-11">
        <div className="lg:hidden">
          <div className="flex items-baseline gap-2">
            <StatusMark status={m.status} wordHidden />
            <span className={`truncate font-semibold ${m.subject ? '' : 'font-normal text-muted'}`}>
              {m.subject || '(no subject)'}
            </span>
            <span className="ml-auto font-mono text-sm text-muted">{formatTime(m.created_at)}</span>
          </div>
          <div className="flex items-center gap-1 truncate text-sm text-ink2">
            <Mono>{m.mail_from}</Mono>
            <ArrowForwardIcon sx={{ fontSize: 14 }} aria-label="to" />
            <Mono>{recipients(m.rcpt_to)}</Mono>
          </div>
          {m.detail && (m.status === 'failed' || m.status === 'rejected') && (
            <p className={`line-clamp-2 text-sm ${detailColor}`}>{m.detail}</p>
          )}
        </div>
        <div className="hidden grid-cols-[7rem_7rem_9rem_minmax(0,1.2fr)_minmax(0,1.2fr)_minmax(0,1.5fr)_5rem] items-center gap-4 text-sm lg:grid">
          <Mono title={m.created_at}>{formatTime(m.created_at)}</Mono>
          <StatusMark status={m.status} />
          <span>{m.credential_id === null && !m.username ? <span className="text-muted">(deleted)</span> : <Mono>{m.username}</Mono>}</span>
          <Mono className="truncate">{m.mail_from}</Mono>
          <Mono className="truncate">{recipients(m.rcpt_to)}</Mono>
          <span className="truncate" title={m.detail || undefined}>{m.subject || '(no subject)'}</span>
          <span className="text-right tabular-nums">{formatSize(m.size)}</span>
        </div>
      </Link>
    </li>
  );
}
