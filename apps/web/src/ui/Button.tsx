import type { ComponentProps } from 'react';

const variants = {
  primary: 'bg-accent text-accent-fg hover:opacity-90',
  secondary: 'border border-border bg-bg text-fg hover:bg-hover',
  danger: 'bg-danger text-accent-fg hover:opacity-90',
  soft: 'bg-surface text-fg hover:bg-hover',
  ghost: 'text-muted hover:bg-hover hover:text-fg',
};

const sizes = { md: 'h-7 px-3 text-sm', lg: 'h-10 px-4 text-base [&>svg]:size-4' };

/** Text button: md 28px, lg 40px. */
export function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  ...props
}: ComponentProps<'button'> & { variant?: keyof typeof variants; size?: keyof typeof sizes }) {
  return (
    <button
      type="button"
      {...props}
      className={`inline-flex shrink-0 items-center justify-center gap-2 rounded-md font-medium outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none disabled:opacity-40 ${sizes[size]} ${variants[variant]} ${className}`}
    />
  );
}
