import type { ReactNode } from 'react';

/** Every page's column: one max width for all, centred. */
export function Page({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <div className={`mx-auto w-full max-w-3xl px-4 ${className}`}>{children}</div>;
}
