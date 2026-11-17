import type { ReactNode } from 'react';

export function Mono({ children, className = '', title }: { children: ReactNode; className?: string; title?: string }) {
  return <span className={`font-mono text-[0.9375em] break-all ${className}`} title={title}>{children}</span>;
}
