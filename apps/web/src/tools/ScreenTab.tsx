// An agent's screen (its browser, its simulator), live beside its chat: watch what it clicks and types, take it over, give it back.
import { useAgents, useNameOf, useScreen, useWrite, why } from '@handline/app-data';
import { useStore } from '@handline/app-react';
import type { ScreenSource } from '@handline/client';
import { Hand, Monitor, MousePointerClick } from 'lucide-react';
import { useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';
import { ScreenFrameView } from '#src/tools/ScreenFrameView.tsx';

const WORD: Record<ScreenSource, string> = { browser: 'Browser', sim: 'Simulator' };

/** The sources the agent has open, the one in use first. */
export function useScreens(agent: string) {
  const list = useAgents().data?.agents.find((a) => a.key === agent)?.screens ?? [];
  return [...list].sort((a, b) => Number(b.in_use) - Number(a.in_use));
}

export function ScreenTab({ agent }: { agent: string }) {
  const screens = useScreens(agent);
  const [pick, setPick] = useState<ScreenSource | null>(null);
  const source = screens.find((s) => s.source === pick)?.source ?? screens[0]?.source ?? null;
  if (!source)
    return (
      <div className="grid min-h-0 flex-1 place-items-center">
        <span className="flex flex-col items-center gap-2 text-sm text-muted">
          <Monitor size={20} aria-hidden />
          No screen open
        </span>
      </div>
    );
  return (
    <Source
      key={source}
      agent={agent}
      source={source}
      inUse={!!screens.find((s) => s.source === source)?.in_use}
      picker={
        screens.length > 1 && (
          <Segmented label="Screen" value={source} options={screens.map((s) => s.source)} word={(s) => WORD[s]} onChange={setPick} />
        )
      }
    />
  );
}

function Source({ agent, source, inUse, picker }: { agent: string; source: ScreenSource; inUse: boolean; picker: React.ReactNode }) {
  const store = useStore();
  const name = useNameOf()(agent);
  const w = useScreen(agent, source);
  const take = useWrite('screen.take', agent);
  const give = useWrite('screen.give_back', agent);
  const holder = w.holder;
  const mine = !!holder && holder.kind !== 'agent' && holder.name === store.callerName;
  const other = !!holder && holder.kind !== 'agent' && !mine;
  const failed = (e: unknown) => toast(why(e) || 'Did not change', { tone: 'warning' });
  const takeOver = () => take.mutate({ agent, source, force: other, request_id: uuid() }, { onError: failed });
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
        <span className="min-w-0 flex-1 overflow-hidden">{picker || <span className="text-sm">{WORD[source]}</span>}</span>
        {mine ? (
          <Button
            variant="primary"
            size="sm"
            busy={give.isPending}
            onClick={() => give.mutate({ agent, source, request_id: uuid() }, { onError: failed })}
          >
            Give back
          </Button>
        ) : (
          <Button size="sm" busy={take.isPending} disabled={!w.frame} onClick={takeOver}>
            Take over
          </Button>
        )}
      </div>
      {mine ? (
        <Bar tone="warning" icon={<Hand size={12} />} text="You have it" />
      ) : other ? (
        <Bar tone="muted" icon={<Hand size={12} />} text={`${holder.name} has it`} />
      ) : (
        inUse && <Bar tone="accent" icon={<MousePointerClick size={12} />} text={`${name} is using the ${WORD[source].toLowerCase()}`} />
      )}
      <ScreenFrameView agent={agent} source={source} watched={w} holds={mine} />
    </div>
  );
}

const TONE = { accent: 'bg-accent text-accent-fg', warning: 'bg-warning text-bg', muted: 'bg-surface text-muted' } as const;

/** A thin bar at the view's top edge: who drives the screen now. */
function Bar({ tone, icon, text }: { tone: keyof typeof TONE; icon: React.ReactNode; text: string }) {
  return (
    <div role="status" className={`flex h-6 shrink-0 items-center gap-1.5 px-3 text-xs font-medium ${TONE[tone]}`}>
      {icon}
      {text}
    </div>
  );
}
