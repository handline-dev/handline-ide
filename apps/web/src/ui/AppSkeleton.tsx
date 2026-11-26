// The app while the agent, people and workspace lists are first read: bars where names will be, so no raw key shows and nothing
// jumps when they arrive.
const SIDE = [70, 55, 80, 45, 65, 50, 75];
const MAIN = [90, 75, 40, 85, 60];

export function SidebarSkeleton() {
  return (
    <div aria-busy aria-label="Loading" className="flex animate-pulse flex-col gap-4 px-4 pt-4">
      {SIDE.map((w, i) => (
        <div key={i} className="h-3.5 rounded-sm bg-border" style={{ width: `${w}%` }} />
      ))}
    </div>
  );
}

export function MainSkeleton() {
  return (
    <div aria-busy aria-label="Loading" className="flex min-w-0 flex-1 animate-pulse flex-col gap-4 px-8 pt-16">
      {MAIN.map((w, i) => (
        <div key={i} className="h-3.5 max-w-150 rounded-sm bg-border" style={{ width: `${w}%` }} />
      ))}
    </div>
  );
}
