// The town, over the whole app (town/mode.tsx): the team as a small city drawn as the offices are (CityView.tsx; city.ts lays it out).
// Every workspace is a building with a window per agent, lit while it works; a building nobody works in goes grey, and a car drives for
// each agent at work, up to a few. The founder's glass HQ holds every decision and the pinned agents. Walking into a building goes inside
// its office (Inside.tsx).
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useAgents, useNav, useRead } from '@handline/app-data';
import { useNow } from '#src/format/time.ts';
import { useTabState } from '#src/persist/tabState.ts';
import { useRequests } from '#src/requests/data.ts';
import { townOf, type House } from '#src/town/model.ts';
import { TownWindows, useTownShown } from '#src/town/Windows.tsx';
import { setTownGo, townGo } from '#src/town/goto.ts';
import { Face } from '#src/town/face.tsx';
import { figuresOf } from '#src/town/people.ts';
import { Inside } from '#src/town/Inside.tsx';
import { CityView, type Sign } from '#src/town/CityView.tsx';
import { cityOf, HQ, seatsOf } from '#src/town/city.ts';
import { SWATCH } from '#src/town/palette.ts';
import { TownList, usePictureShown } from '#src/town/List.tsx';

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
  const workspaces = useRead('workspaces.list', {}).data?.workspaces;
  const agents = useAgents().data?.agents;
  const { turn } = useRequests();
  // A prompt cache runs out with no event, so the town looks again each minute.
  const now = useNow(60_000);
  const town = useMemo(() => townOf(workspaces ?? [], agents ?? [], turn, now), [workspaces, agents, turn, now]);
  const [pane, aspect] = useAspect();
  const city = useMemo(() => cityOf(town.houses, aspect), [town.houses, aspect]);
  const [picked, setPicked] = useTabState<string | null>('house', null);
  // A person's chat tab shows their face, as drawn in their office.
  const rows = useMemo(() => new Map(town.houses.flatMap((h) => [...figuresOf(h.people)])), [town.houses]);
  const iconFor = (opens: string | undefined) => {
    const row = opens?.startsWith('chat:') ? rows.get(opens.slice(5)) : undefined;
    return row === undefined ? undefined : <Face row={row} />;
  };
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
  /** Into a house from the city; one found by a person's name brings up that person's chat there too. */
  const enterFor = (key: string, person?: string) => {
    const who = houseOf(key)?.people.find((p) => p.key === person);
    if (who) townGo(key, who);
    else enter(key);
  };
  const signs = useMemo(() => {
    const out: Record<string, Sign> = {
      [HQ]: {
        name: 'Your HQ',
        sub: `All ${town.decisions.length} decisions`,
        decisions: town.decisions.length,
        waiting: 0,
        stopped: 0,
        more: 0,
      },
    };
    for (const h of town.houses) {
      const lot = city.lots.find((l) => l.key === h.key);
      out[h.key] = {
        name: h.name,
        sub: statusOf(h),
        decisions: h.decisions.length,
        waiting: h.people.filter((p) => p.activity === 'you').length,
        stopped: h.people.filter((p) => p.activity === 'error').length,
        more: lot ? Math.max(0, h.people.length - seatsOf(lot.w)) : 0,
      };
    }
    return out;
  }, [town, city]);

  return (
    <TownWindows iconFor={iconFor}>
      <GoTo enter={enter} />
      <ListOr houses={town.houses}>
        {house ? (
          <Inside key={house.key} house={house} cameIn={left === house.key} onBack={() => setPicked(null)} />
        ) : (
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div ref={pane} className="flex min-h-0 flex-1">
              <CityView city={city} cars={Math.min(working, MAX_CARS)} signs={signs} houses={town.houses} left={left} onEnter={enterFor} />
            </div>
            <Legend />
          </div>
        )}
      </ListOr>
    </TownWindows>
  );
}

/** On a phone's width, the list in place of the picture until the picture is asked for. */
function ListOr({ houses, children }: { houses: readonly House[]; children: ReactNode }) {
  const { narrow } = useTownShown();
  const picture = usePictureShown();
  return narrow && !picture ? <TownList houses={houses} /> : children;
}

/** Takes the top strip's picks (goto.ts): into the house, and the person's chat in the town's windows. */
function GoTo({ enter }: { enter: (key: string) => void }) {
  const { show } = useTownShown();
  setTownGo((house, person) => {
    enter(house);
    if (person) show(`chat:${person.key}`, person.name);
  });
  return null;
}

/** What the windows and badges mean, under the city: in full where the pane is wide, in a word or two where it is narrow, so it never
 *  takes more than two rows. */
function Legend() {
  // A phone's width has the list, which says it all in words; the picture there goes without a key.
  if (useTownShown().narrow) return null;
  const square = (background: string) => <span className="size-3 rounded-sm" style={{ background }} />;
  const pin = (fill: string, text: string) => (
    <span
      className="flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-xs font-bold text-accent-fg"
      style={{ background: fill }}
    >
      {text}
    </span>
  );
  // Each mark, what it means in full, and in short.
  const rows: [ReactNode, string, string][] = [
    [square(SWATCH.lit), 'A window per agent: working', 'Working'],
    [square(SWATCH.dark), 'Idle', 'Idle'],
    [square(SWATCH.stopped), 'Stopped', 'Stopped'],
    [square(SWATCH.empty), 'Nobody at work', 'Nobody at work'],
    [pin('var(--warning)', '!'), 'Waiting for you', 'Waiting for you'],
    [pin('var(--danger)', '1'), 'Decisions for you', 'Decisions'],
  ];
  return (
    <div className="@container shrink-0 border-t border-border text-xs text-muted">
      <div className="flex min-h-9 flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2">
        {rows.map(([mark, word, short]) => (
          <span key={word} className="flex items-center gap-1.5 whitespace-nowrap">
            {mark}
            <span className="hidden @4xl:inline">{word}</span>
            <span className="@4xl:hidden">{short}</span>
          </span>
        ))}
      </div>
    </div>
  );
}
