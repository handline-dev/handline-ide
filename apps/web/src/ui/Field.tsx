import type { ComponentProps, ReactNode } from 'react';

const control =
  'h-8 w-full rounded-md border border-border bg-bg px-2 text-sm text-fg outline-none placeholder:text-muted focus:border-accent disabled:opacity-40';

/** A labeled form row: the label above, the control, then a muted hint or a danger error under it. */
export function Field({ label, hint, error, children }: { label: string; hint?: ReactNode; error?: ReactNode; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
      {error ? <span className="text-xs text-danger">{error}</span> : hint && <span className="text-xs text-muted">{hint}</span>}
    </label>
  );
}

export function Input({ className = '', ...props }: ComponentProps<'input'>) {
  return <input {...props} className={`${control} ${className}`} />;
}

export function Select({ className = '', ...props }: ComponentProps<'select'>) {
  return <select {...props} className={`${control} ${className}`} />;
}
