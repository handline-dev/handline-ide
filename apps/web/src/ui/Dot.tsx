/** The accent dot that says something waits to be read. `className` places it. */
export function UnreadDot({ label = 'Unread', className = '' }: { label?: string; className?: string }) {
  return <span role="img" aria-label={label} className={`size-1.5 shrink-0 rounded-full bg-accent ${className}`} />;
}
