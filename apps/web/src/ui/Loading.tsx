/** Placeholders while data loads, shaped like what comes, so a page does not read "empty" or jump when it arrives. */

/** A list's rows: a title line and a shorter one under it, each row the height of a real one. */
export function ListLoading({ rows = 5, label = 'Loading' }: { rows?: number; label?: string }) {
  return (
    <div aria-busy="true" aria-label={label} className="flex animate-pulse flex-col motion-reduce:animate-none">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex h-11 flex-col justify-center gap-1.5 border-b border-border px-2">
          <span className="h-3 rounded-sm bg-border" style={{ width: `${[55, 40, 65, 45, 50][i % 5]}%` }} />
          <span className="h-2.5 w-24 rounded-sm bg-border/60" />
        </div>
      ))}
    </div>
  );
}

/** A block of text: lines of varied width. */
export function LinesLoading({ lines = 6, label = 'Loading' }: { lines?: number; label?: string }) {
  return (
    <div aria-busy="true" aria-label={label} className="flex animate-pulse flex-col gap-2.5 motion-reduce:animate-none">
      {Array.from({ length: lines }, (_, i) => (
        <span key={i} className="h-3 rounded-sm bg-border" style={{ width: `${[90, 75, 85, 40, 70, 60][i % 6]}%` }} />
      ))}
    </div>
  );
}
