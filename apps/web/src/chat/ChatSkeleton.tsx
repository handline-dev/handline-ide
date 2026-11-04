/** The chat while its first page loads: lines shaped like the ones to come, the agent's on the left in a few lines of
 * text, the founder's on the right in a bubble, so the layout does not jump when they arrive. */
const lines: { mine: boolean; widths: number[] }[] = [
  { mine: false, widths: [90, 75, 40] },
  { mine: true, widths: [35] },
  { mine: false, widths: [85, 60] },
  { mine: true, widths: [50, 25] },
  { mine: false, widths: [95, 80, 70, 30] },
];

export function ChatSkeleton() {
  return (
    <div aria-busy aria-label="Loading the chat" className="flex animate-pulse flex-col gap-6">
      {lines.map(({ mine, widths }, i) => (
        <div key={i} className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
          <div className={`flex w-full max-w-150 flex-col gap-2 ${mine ? 'items-end' : 'items-start'}`}>
            {widths.map((w, j) => (
              <div key={j} className="h-3.5 rounded-sm bg-border" style={{ width: `${w}%` }} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
