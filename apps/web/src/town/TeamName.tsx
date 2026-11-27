// A team's name over its office: a click on it edits it in place, Enter renames the team (workspace.set), Escape or a click away
// leaves it as it was. A name the daemon refuses says why in a toast.
import { useWrite, why } from '@handline/app-data';
import { useState } from 'react';
import { workspaceNameOf } from '#src/onboarding/firstTask.ts';
import { toast } from '#src/ui/toast.tsx';

export function TeamName({ team, name, fixed }: { team: string; name: string; fixed?: boolean }) {
  const set = useWrite('workspace.set');
  const [editing, setEditing] = useState<string | null>(null);
  if (fixed) return <span className="truncate font-medium">{name}</span>;
  if (editing === null)
    return (
      <button type="button" onClick={() => setEditing(name)} className="truncate rounded-md px-1 font-medium hover:bg-hover">
        {name}
      </button>
    );
  const done = () => {
    const next = workspaceNameOf(editing.trim());
    setEditing(null);
    if (!editing.trim() || next === name) return;
    set.mutate({ workspace: team, name: next }, { onError: (e) => toast(why(e), { tone: 'warning' }) });
  };
  return (
    <input
      aria-label="Team name"
      autoFocus
      value={editing}
      onFocus={(e) => e.currentTarget.select()}
      onChange={(e) => setEditing(e.target.value)}
      onBlur={() => setEditing(null)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') done();
        if (e.key === 'Escape') setEditing(null);
        e.stopPropagation();
      }}
      className="h-6 w-40 rounded-md border border-accent bg-transparent px-1 text-sm font-medium outline-none"
    />
  );
}
