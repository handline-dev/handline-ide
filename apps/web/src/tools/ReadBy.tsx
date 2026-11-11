import type { ActivityItem } from '@handline/app-core';
import { Eye } from 'lucide-react';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { Who } from '#src/ui/Who.tsx';

/** How many of the agents a founder's comment was told to have read it (eye 1/2); on hover, who has and who has not. */
export function ReadBy({ readers }: { readers?: ActivityItem['readers'] }) {
  if (!readers?.length) return null;
  const read = readers.filter((r) => r.state === 'read');
  const not = readers.filter((r) => r.state !== 'read');
  return (
    <HoverCard
      trigger={
        <span className={`flex items-center gap-1 font-normal ${not.length ? '' : 'text-fg'}`}>
          <Eye size={12} />
          {read.length}/{readers.length}
        </span>
      }
    >
      <div className="flex flex-col gap-1 p-2 text-sm">
        {not.length > 0 && <span className="text-xs text-muted">Not read yet</span>}
        {not.map((r) => (
          <span key={r.agent}>
            <Who k={r.agent} />
            {r.state === 'held' && <span className="text-muted"> · waits for its next digest</span>}
          </span>
        ))}
        {read.length > 0 && <span className={`text-xs text-muted ${not.length ? 'pt-1' : ''}`}>Read</span>}
        {read.map((r) => (
          <span key={r.agent}>
            <Who k={r.agent} />
          </span>
        ))}
      </div>
    </HoverCard>
  );
}
