import type { ReactNode } from 'react';

export interface FactItem { label: string; value: ReactNode }

export function Facts({ items }: { items: FactItem[] }) {
  return (
    <dl>
      {items.map((item, index) => (
        <div key={index} className="grid gap-1 border-b border-hairline py-3 sm:grid-cols-[12rem_1fr] sm:gap-4">
          <dt className="text-sm text-ink2">{item.label}</dt>
          <dd className="m-0 min-w-0">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}
