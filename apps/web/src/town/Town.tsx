// The town, over the whole app (town/mode.tsx): the team as a small city drawn as the offices are (CityView.tsx; city.ts lays it out).
// Every workspace is a building with a window per agent, lit while it works; a building nobody works in goes grey, and a car drives for
// each agent at work, up to a few. The founder's glass HQ holds every decision and the pinned agents. Walking into a building goes inside
// its office (Inside.tsx).
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Hooks, useAgents, useNav } from '@handline/app-data';
import { useNow } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { useTabState } from '#src/persist/tabState.ts';
import { useRequests } from '#src/requests/data.ts';
import { townOf, type House } from '#src/town/model.ts';
import { Decisions, Panel } from '#src/town/panel.tsx';
import { Inside } from '#src/town/Inside.tsx';
import { CityView, type Sign } from '#src/town/CityView.tsx';
import { cityOf, HQ, seatsOf } from '#src/town/city.ts';
import { SWATCH } from '#src/town/palette.ts';

const MAX_CARS = 8;
const statusOf = (h: House) => (h.working ? `${h.working} of ${h.people.length} at work` : 'Lights off');
/** The pane's width over its height, kept as it resizes; a callback ref, so it follows the pane back from an office. */
function useAspect(): [(el: HTMLDivElement | null) => void, number] {
  const [el, setEl] = useState<HTMLDivElement | null>(null);
  const [aspect, setAspect] = useState(16 / 9);
  useEffect(() => {
    if (!el) return;
    const seen = new ResizeObserver(([e]) => e.contentRect.height > 0 && setAspect(e.contentRect.width / e.contentRect.height));
    seen.observe(el);
    return () => seen.disconnect();
  }, [el]);
  return [setEl, aspect];
}

export function TownTab() {
  const workspaces = Hooks.useWorkspacesList().data?.workspaces;
  const agents = useAgents().data?.agents;
  const { turn } = useRequests();
  // A prompt cache runs out with no event, so the town looks again each minute.
  const now = useNow(60_000);
  const town = useMemo(() => townOf(workspaces ?? [], agents ?? [], turn, now), [workspaces, agents, turn, now]);
  const [pane, aspect] = useAspect();
  const city = useMemo(() => cityOf(town.houses, aspect), [town.houses, aspect]);
  // eslint-disable-next-line no-restricted-syntax -- the clicks land on shapes over a drawn picture (a person, a desk, a paper), not on elements that can take useOpenable's props
  const open = useOpenTab();
  const [picked, setPicked] = useTabState<string | null>('house', null);
  const [mail, setMail] = useTabState('mail', false);
  const houseOf = (key: string) => town.houses.find((h) => h.key === key);
  const pins = useNav().nav.pins;
  const hq = useMemo((): House => {
    const people = town.houses.flatMap((h) => h.people);
    const pinned = pins.flatMap((k) => people.filter((p) => p.key === k));
    return {
      key: HQ,
      name: 'Your HQ',
      lead: null,
      people: pinned,
      working: pinned.filter((p) => p.activity === 'working').length,
      decisions: town.decisions,
      hq: true,
    };
  }, [town, pins]);
  const house = picked === HQ ? hq : picked ? houseOf(picked) : undefined;
  const working = town.houses.reduce((n, h) => n + h.working, 0);
  // The founder comes back out of a house at its own door; the city first shown starts at the HQ.
  const [left, setLeft] = useState<string | null>(null);
  const enter = (key: string) => {
    setLeft(key);
    setPicked(key);
  };
  const signs = useMemo(() => {
    const out: Record<string, Sign> = {
      [HQ]: { name: 'Your HQ', sub: `All ${town.decisions.length} decisions`, decisions: town.decisions.length, waiting: 0, more: 0 },
    };
    for (const h of town.houses) {
      const lot = city.lots.find((l) => l.key === h.key);
      out[h.key] = {
        name: h.name,
        sub: statusOf(h),
        decisions: h.decisions.length,
        waiting: h.people.filter((p) => p.activity === 'you').length,
        more: lot ? Math.max(0, h.people.length - seatsOf(lot.w)) : 0,
      };
    }
    return out;
  }, [town, city]);

  return (
    <div className="relative flex min-h-0 min-w-0 flex-1 bg-bg">
      {house ? (
        <Inside
          key={house.key}
          house={house}
          cameIn={left === house.key}
          onBack={() => {
            setPicked(null);
            setMail(false);
          }}
          onOpen={(d) => open(d.task ? taskOf(d.task) : 'requests', 'side')}
          onMail={() => setMail((m) => !m)}
        />
      ) : (
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div ref={pane} className="flex min-h-0 flex-1">
            <CityView city={city} cars={Math.min(working, MAX_CARS)} signs={signs} left={left} onEnter={enter} />
          </div>
          <Legend />
        </div>
      )}
      {house && mail && (
        <Panel title="Decisions" sub={house.name} onClose={() => setMail(false)}>
          <Decisions papers={house.decisions} onOpen={(d) => open(d.task ? taskOf(d.task) : 'requests', 'side')} />
        </Panel>
      )}
    </div>
  );
}

/** What the windows and badges mean, under the city. */
function Legend() {
  const square = (background: string) => <span className="size-3 rounded-sm" style={{ background }} />;
  const pin = (fill: string, text: string) => (
    <span
      className="flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-xs font-bold text-accent-fg"
      style={{ background: fill }}
    >
      {text}
    </span>
  );
  const rows: [ReactNode, string][] = [
    [square(SWATCH.lit), 'A window per agent: working'],
    [square(SWATCH.dark), 'Idle or asleep'],
    [square(SWATCH.empty), 'Nobody at work'],
    [pin('var(--warning)', '!'), 'Waiting for you'],
    [pin('var(--danger)', '1'), 'Decisions for you'],
  ];
  return (
    <div className="flex min-h-9 shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-t border-border px-4 py-2 text-xs text-muted">
      {rows.map(([mark, word]) => (
        <span key={word} className="flex items-center gap-1.5">
          {mark}
          {word}
        </span>
      ))}
    </div>
  );
}
