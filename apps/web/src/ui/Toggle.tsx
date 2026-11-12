/** An on/off switch for one setting; the label says what it turns. */
export function Toggle({
  on,
  label,
  disabled,
  onChange,
}: {
  on: boolean;
  label: string;
  disabled?: boolean;
  onChange: (on: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={`relative h-5 w-9 shrink-0 rounded-full outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 ${on ? 'bg-accent' : 'bg-border'}`}
    >
      <span className={`absolute left-0.5 top-0.5 size-4 rounded-full bg-bg transition-transform ${on ? 'translate-x-4' : ''}`} />
    </button>
  );
}
