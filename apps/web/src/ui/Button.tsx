import { Spinner } from '#src/ui/Loading.tsx';
import { useState, type ComponentProps } from 'react';

const variants = {
  primary: 'bg-accent text-accent-fg hover:opacity-90',
  secondary: 'border border-border bg-bg text-fg hover:bg-hover',
  danger: 'bg-danger text-accent-fg hover:opacity-90',
  soft: 'bg-surface text-fg hover:bg-hover',
  ghost: 'text-muted hover:bg-hover hover:text-fg',
};

const sizes = { sm: 'h-6 gap-1.5 px-2 text-xs', md: 'h-7 gap-2 px-3 text-sm', lg: 'h-10 gap-2 px-4 text-base [&>svg]:size-4' };

/** Text button: sm 24px, md 28px, lg 40px. While `busy`, a spinner over its words at their width, and disabled. */
export function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  busy = false,
  children,
  disabled,
  ...props
}: ComponentProps<'button'> & { variant?: keyof typeof variants; size?: keyof typeof sizes; busy?: boolean }) {
  return (
    <button
      type="button"
      {...props}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`relative inline-flex shrink-0 items-center justify-center rounded-md font-medium outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none ${busy ? '' : 'disabled:opacity-40'} ${sizes[size]} ${variants[variant]} ${className}`}
    >
      {busy ? (
        <>
          <span className="contents [&>*]:invisible [color:transparent]">{children}</span>
          <span aria-hidden className="absolute inset-0 grid place-items-center">
            <Spinner />
          </span>
        </>
      ) : (
        children
      )}
    </button>
  );
}

/** A call a button makes: `busy` while it runs, the refusal in words if it failed; `go` resolves true on success. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const go = async (call: () => Promise<unknown>): Promise<boolean> => {
    setBusy(true);
    setError(null);
    try {
      await call();
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, go, reset: () => setError(null) };
}
