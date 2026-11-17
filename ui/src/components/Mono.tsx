import type { ReactNode } from 'react';

export function Mono({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <span className={`font-mono text-[0.9375em] break-all ${className}`}>{children}</span>;
}
