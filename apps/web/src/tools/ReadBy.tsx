import type { ActivityItem } from '@handline/app-core';
import { Check } from 'lucide-react';
import { Who } from '#src/ui/Who.tsx';
import { clock } from '#src/format/time.ts';

/** Under a founder's comment: each agent it was told to that read it, and when ("legacy read it 09:34"); one still waiting says so. */
export function ReadBy({ readers }: { readers?: ActivityItem['readers'] }) {
  if (!readers?.length) return null;
  return (
    <div className="flex flex-col gap-0.5 pt-1 text-xs text-muted">
      {readers.map((r) => (
        <span key={r.agent} className="flex items-center gap-1">
          {r.state === 'read' && <Check size={12} className="text-success" />}
          <Who k={r.agent} />
          {r.state === 'read'
            ? ` read it${r.at ? ` ${clock(r.at)}` : ''}`
            : r.state === 'held'
              ? ' gets it with its next digest'
              : ' has not read it yet'}
        </span>
      ))}
    </div>
  );
}
