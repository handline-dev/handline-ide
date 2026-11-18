// Someone in a task: their face (the initial in a circle) and name, the one look for a writer or an owner wherever a task shows one.
// An agent's face carries its state as a dot at the bottom right; a person's has none.
import { nameOf } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf } from '@handline/app-data';
import { activityOf, type Activity } from '#src/layout/agentList.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { goAgent } from '#src/layout/openTab.tsx';
import type { MouseEvent, ReactNode } from 'react';

const DOT: Record<Activity, string> = {
  working: 'bg-success',
  waking: 'bg-success animate-pulse',
  resting: 'bg-muted',
  asleep: 'border-2 border-muted bg-bg',
  you: 'bg-warning',
  held: 'bg-muted',
  error: 'bg-danger',
};
const WORD: Record<Activity, string> = {
  working: 'Working',
  waking: 'Starting',
  resting: 'On',
  asleep: 'Asleep',
  you: 'Needs you',
  held: 'Waiting',
  error: 'Needs attention',
};

/** An agent's state, or null for a person (or an agent not listed). */
function useActivity(key: string): Activity | null {
  const agent = useAgents().data?.agents.find((a) => a.key === key);
  return agent ? activityOf(agent) : null;
}

/** An agent's face or name, or both: a click brings up its workspace, as its sidebar row does (Cmd opens its chat in a tab, a drag
 *  places it). Inside a clickable card it takes the click for itself. A person's is not a link. */
function AgentLink({ who, mine, className, children }: { who: string; mine: boolean; className: string; children: ReactNode }) {
  const agent = useAgents().data?.agents.some((a) => a.key === who) ?? false;
  const open = useOpenable(agent && !mine ? chatOf(who) : null, { here: () => goAgent(who) });
  if (mine || !agent) return <span className={className}>{children}</span>;
  return (
    <span
      role="link"
      tabIndex={0}
      {...open}
      onClick={(e: MouseEvent) => {
        e.stopPropagation();
        open.onClick(e);
      }}
      className={`cursor-pointer rounded hover:bg-hover/30 ${className}`}
    >
      {children}
    </span>
  );
}

/** The circle with the initial, and an agent's state dot. */
export function Mark({ who, mine = false }: { who: string; mine?: boolean }) {
  const name = useNameOf()(who);
  const act = useActivity(who);
  return (
    // Its own size, so a row that stretches its children (a comment's) does not move the dot down with it.
    <span className="relative size-6 shrink-0">
      <span
        className={`grid size-full place-items-center rounded-full text-xs font-semibold text-accent-fg ${mine ? 'bg-accent' : 'bg-muted'}`}
      >
        {(name[0] ?? '?').toUpperCase()}
      </span>
      {act && !mine && (
        <span
          role="img"
          aria-label={WORD[act]}
          title={WORD[act]}
          className={`absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-bg ${DOT[act]}`}
        />
      )}
    </span>
  );
}

const nameText = (name: string, mine: boolean) => (mine ? 'You' : name);

/** An agent's key from its name or key as written after an @, or null. */
export function useAgentByName(): (word: string) => string | null {
  const d = useDirectory();
  return (word: string) => {
    const w = word.toLowerCase();
    return d.agents.find((a) => a.key.toLowerCase() === w || nameOf(d, a.key).toLowerCase() === w)?.key ?? null;
  };
}

/** An agent named in text (@name): marked as a mention, and a link to it like its face. */
export function Mention({ who }: { who: string }) {
  const name = useNameOf()(who);
  return (
    <AgentLink who={who} mine={false} className="rounded bg-accent/10 px-1 text-accent">
      @{name}
    </AgentLink>
  );
}

/** The face alone: `who` is an agent's key or a person's; `mine` paints it the founder's blue and names them You. */
export function Face({ who, mine = false }: { who: string; mine?: boolean }) {
  return (
    <AgentLink who={who} mine={mine} className="grid size-6 shrink-0 rounded-full">
      <Mark who={who} mine={mine} />
    </AgentLink>
  );
}

/** The name alone. */
export function Name({ who, mine = false, className = '' }: { who: string; mine?: boolean; className?: string }) {
  const name = useNameOf()(who);
  return (
    <AgentLink who={who} mine={mine} className={`-mx-1 truncate px-1 ${className}`}>
      {nameText(name, mine)}
    </AgentLink>
  );
}

/** The face and the name beside it, one link. */
export function FaceName({ who, mine = false, className = '' }: { who: string; mine?: boolean; className?: string }) {
  const name = useNameOf()(who);
  return (
    <AgentLink who={who} mine={mine} className={`-mx-1 inline-flex min-w-0 items-center gap-1.5 px-1 ${className}`}>
      <Mark who={who} mine={mine} />
      <span className="truncate">{nameText(name, mine)}</span>
    </AgentLink>
  );
}
