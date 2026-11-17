import type { ReactNode } from 'react';
import { useEffect } from 'react';

export interface PageHeaderProps {
  title: ReactNode;
  // When title is not plain text (e.g. an icon plus text), titleText supplies
  // the string used for document.title.
  titleText?: string;
  description?: string;
  action?: ReactNode;
  back?: { to: string; label: string };
}

export function PageHeader({ title, titleText, description, action, back }: PageHeaderProps) {
  const docTitle = titleText ?? (typeof title === 'string' ? title : '');
  useEffect(() => {
    if (docTitle) document.title = `${docTitle} · SMTP Gateway`;
  }, [docTitle]);

  return (
    <div className="mb-6">
      {back && (
        <a href={back.to} className="mb-2 flex min-h-11 items-center gap-1 text-sm text-ink2">
          <span aria-hidden>←</span>
          {back.label}
        </a>
      )}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold lg:text-[1.75rem]">{title}</h1>
          {description && <p className="mt-1 max-w-prose text-ink2">{description}</p>}
        </div>
        {action && <div className="w-full sm:w-auto">{action}</div>}
      </div>
    </div>
  );
}
