import type { ComponentProps } from 'react';

/** Square icon-only button, 28px; `size="sm"` is 16px for inline use (a tab's close). Passes other button
 * props through, so a popover or menu can use it as its trigger. */
export function IconButton({
  label,
  size = 'md',
  className = '',
  ...props
}: ComponentProps<'button'> & { label: string; size?: 'sm' | 'md' }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...props}
      className={`grid shrink-0 place-items-center text-muted hover:bg-hover hover:text-fg data-[popup-open]:bg-hover data-[popup-open]:text-fg ${
        size === 'sm' ? 'size-4 rounded-sm' : 'size-7 rounded-md'
      } ${className}`}
    />
  );
}
