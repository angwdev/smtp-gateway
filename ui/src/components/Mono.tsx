import { Fragment, type ReactNode } from 'react';

// Offers a break opportunity after each separator in host:port style and
// address-like text (mail.example.com:587, a@b.example.com), so long values
// wrap at a readable point instead of mid-word when a column is narrow.
// break-all still applies as a fallback for any segment that is itself too
// long to fit (an all-alphanumeric run with no separators).
function withBreakHints(text: string): ReactNode {
  const parts = text.split(/(?<=[.:@/])/);
  if (parts.length < 2) return text;
  return parts.map((part, i) => (
    <Fragment key={i}>
      {part}
      <wbr />
    </Fragment>
  ));
}

export function Mono({ children, className = '', title }: { children: ReactNode; className?: string; title?: string }) {
  const content = typeof children === 'string' ? withBreakHints(children) : children;
  // break-words (not break-all): the <wbr> hints above already offer a break
  // point at every separator; break-all would ignore those and chop at every
  // character instead. break-words is the fallback for a single unbreakable
  // run that is still too wide (no separator) once wbr hints are exhausted.
  return <span className={`font-mono text-[0.9375em] break-words ${className}`} title={title}>{content}</span>;
}
