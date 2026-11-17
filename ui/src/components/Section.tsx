import type { ReactNode } from 'react';
import { useId } from 'react';

export interface SectionProps {
  title: string;
  action?: ReactNode;
  children: ReactNode;
}

export function Section({ title, action, children }: SectionProps) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="border-t border-hairline py-6">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 id={headingId} className="text-lg font-semibold">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}
