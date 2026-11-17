import BlockOutlinedIcon from '@mui/icons-material/BlockOutlined';
import ErrorOutlineOutlinedIcon from '@mui/icons-material/ErrorOutlineOutlined';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useId, useState } from 'react';

import type { AuditEvent } from '../api/types';
import { ACTOR_WORD, RESULT_WORD, changeRows, describeEvent } from '../lib/audit';
import { browserName, formatDateTime, formatTime } from '../lib/format';
import { Facts } from './Facts';
import { Mono } from './Mono';

const looksLikeAddress = (s: string) => s.includes('@');

export function AuditRow({ event: e }: { event: AuditEvent }) {
  const [open, setOpen] = useState(false);
  const panelId = useId();

  const actorMono = looksLikeAddress(e.actor) || e.actor_type === 'smtp';
  const sentence = describeEvent(e);
  const resultIcon = e.result === 'denied'
    ? <BlockOutlinedIcon sx={{ fontSize: 18 }} className="text-rejected" aria-hidden />
    : e.result === 'error'
      ? <ErrorOutlineOutlinedIcon sx={{ fontSize: 18 }} className="text-failed" aria-hidden />
      : null;

  return (
    <li className="border-b border-hairline">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-14 w-full items-start gap-3 py-3 text-left lg:min-h-11"
      >
        {resultIcon}
        <span className="sr-only">{RESULT_WORD[e.result]}</span>
        <span className="min-w-0 flex-1">
          <span className="font-semibold">{actorMono ? <Mono>{e.actor}</Mono> : e.actor}</span>{' '}
          {sentence}
          {e.action === 'api.call' && e.detail && <> <Mono className="text-sm">{e.detail}</Mono></>}
        </span>
        <span className="font-mono text-sm text-muted" title={formatDateTime(e.created_at)}>{formatTime(e.created_at)}</span>
        <ExpandMoreIcon sx={{ fontSize: 18, transform: open ? 'rotate(180deg)' : undefined }} aria-hidden />
      </button>
      {open && (
        <div id={panelId} className="border-b border-hairline bg-sunken px-4 py-3">
          <Facts
            items={[
              { label: 'When', value: formatDateTime(e.created_at) },
              { label: 'Who', value: `${e.actor} (${ACTOR_WORD[e.actor_type]})` },
              { label: 'Outcome', value: RESULT_WORD[e.result] },
              ...(e.ip ? [{ label: 'From address', value: <Mono>{e.ip}</Mono> }] : []),
              ...(e.user_agent
                ? [{ label: 'Using', value: <span>{browserName(e.user_agent)}<span className="mt-0.5 block break-all text-xs text-muted">{e.user_agent}</span></span> }]
                : []),
              ...(e.detail && !sentence.includes(e.detail) ? [{ label: 'Detail', value: e.detail }] : []),
            ]}
          />
          {Object.keys(e.changes).length > 0 && (
            <div className="mt-3">
              <h3 className="mb-2 text-sm font-semibold">What changed</h3>
              <table className="w-full text-sm">
                <thead>
                  <tr><th className="text-left text-ink2">Setting</th><th className="text-left text-ink2">Before</th><th className="text-left text-ink2">After</th></tr>
                </thead>
                <tbody>
                  {changeRows(e.changes).map((row) => (
                    <tr key={row.label} className="border-t border-hairline">
                      <td className="py-1 pr-2">{row.label}</td>
                      <td className="py-1 pr-2">{row.before}</td>
                      <td className="py-1">{row.after}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </li>
  );
}
