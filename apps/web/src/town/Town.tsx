// The Town tab: the team as a small city from Kenney's Roguelike Modern City tiles (city.ts lays it out and draws it). Every workspace is
// a building with a window per agent, lit while it works; a building nobody works in goes grey, and cars drive while anyone works. The
// city fills the pane. The founder's glass HQ holds every decision. A click on a building goes inside its office (Inside.tsx).
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Hooks, useAgents } from '@handline/app-data';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { useRequests } from '#src/requests/data.ts';
import { townOf, type House } from '#src/town/model.ts';
import { Decisions, Panel } from '#src/town/panel.tsx';
import { Inside } from '#src/town/Inside.tsx';
import { CARS, cityOf, draw, HQ, seatsOf, SHEET, T, windowsOf, type Art, type City, type Lot } from '#src/town/city.ts';

const MAX_CARS = 8;
const PIXEL = { imageRendering: 'pixelated' } as const;

/** The sheet and its lit and dark windows, once the sheet has loaded. */
function useArt(): Art | null {
  const [art, setArt] = useState<Art | null>(null);
  useEffect(() => {
    const img = new Image();
    img.onload = () => setArt({ sheet: img, windows: windowsOf(img) });
    img.src = SHEET;
  }, []);
  return art;
}

/** The city drawn once per change, as an image URL. */
function usePicture(art: Art | null, city: City): string | null {
  return useMemo(() => {
    if (!art) return null;
    const canvas = document.createElement('canvas');
    canvas.width = city.w * T;
    canvas.height = city.h * T;
    const ctx = canvas.getContext('2d');
    if (!ctx) return null;
    draw(ctx, art, city);
    return canvas.toDataURL();
  }, [art, city]);
}

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
  const town = useMemo(() => townOf(workspaces ?? [], agents ?? [], turn), [workspaces, agents, turn]);
  const [pane, aspect] = useAspect();
  const city = useMemo(() => cityOf(town.houses, aspect), [town.houses, aspect]);
  const art = useArt();
  const picture = usePicture(art, city);
  // eslint-disable-next-line no-restricted-syntax -- the clicks land on shapes over a drawn picture (a person, a desk, a paper), not on elements that can take useOpenable's props
  const open = useOpenTab();
  const [picked, setPicked] = useState<string | null>(null);
  const [mail, setMail] = useState(false);
  const status = (h: House) => (h.working ? `${h.working} of ${h.people.length} at work` : 'Lights off');
  const houseOf = (key: string) => town.houses.find((h) => h.key === key);
  const house = picked && picked !== HQ ? houseOf(picked) : undefined;
  const working = town.houses.reduce((n, h) => n + h.working, 0);
  const W = city.w * T;
  const H = city.h * T;

  return (
    <div className="relative flex min-h-0 min-w-0 flex-1 bg-bg">
      {house ? (
        <Inside
          house={house}
          onBack={() => {
            setPicked(null);
            setMail(false);
          }}
          onTalk={(p) => open(chatOf(p.key), 'side')}
          onMail={() => setMail((m) => !m)}
        />
      ) : (
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div ref={pane} className="min-h-0 flex-1 overflow-hidden">
            <svg
              viewBox={`0 0 ${W} ${H}`}
              preserveAspectRatio="xMidYMid slice"
              className="size-full"
              role="img"
              aria-label="Your team as a city"
            >
              {picture && <image href={picture} width={W} height={H} style={PIXEL} />}
              {picture && <Traffic cars={Math.min(working, MAX_CARS)} roads={city.roads} w={W} h={H} />}
              {city.lots.map((lot) => {
                const h = lot.key === HQ ? null : houseOf(lot.key);
                return (
                  <Building
                    key={lot.key}
                    lot={lot}
                    name={h ? h.name : 'Your HQ'}
                    sub={h ? status(h) : `All ${town.decisions.length} decisions`}
                    decisions={h ? h.decisions.length : town.decisions.length}
                    waiting={h ? h.people.filter((p) => p.activity === 'you').length : 0}
                    more={h ? Math.max(0, h.people.length - seatsOf(lot.w)) : 0}
                    picked={picked === lot.key}
                    onPick={() => setPicked(lot.key)}
                  />
                );
              })}
            </svg>
          </div>
          <Legend windows={art?.windows} />
        </div>
      )}
      {picked === HQ && (
        <Panel title="Your HQ" sub="Every decision waiting for you" onClose={() => setPicked(null)}>
          <Decisions
            papers={town.decisions}
            groups={[...town.houses, { key: '', name: 'Other' }]}
            onOpen={(d) => open(d.task ? taskOf(d.task) : 'requests', 'side')}
          />
        </Panel>
      )}
      {house && mail && (
        <Panel title="Decisions" sub={house.name} onClose={() => setMail(false)}>
          <Decisions papers={house.decisions} onOpen={(d) => open(d.task ? taskOf(d.task) : 'requests', 'side')} />
        </Panel>
      )}
    </div>
  );
}

/** A building's click area, its name and state on the pavement under it, and its counts on the roof: decisions, agents waiting for the
 * founder, and agents past its windows. */
function Building({
  lot,
  name,
  sub,
  decisions,
  waiting,
  more,
  picked,
  onPick,
}: {
  lot: Lot;
  name: string;
  sub: string;
  decisions: number;
  waiting: number;
  more: number;
  picked: boolean;
  onPick: () => void;
}) {
  const x = lot.x * T;
  const y = lot.y * T;
  const w = lot.w * T;
  const mid = x + w / 2;
  return (
    <g className="cursor-pointer" onClick={onPick}>
      <title>{`${name} · ${sub}`}</title>
      <rect
        x={x - 2}
        y={y - 2}
        width={w + 4}
        height={(lot.h + 1) * T + 2}
        rx={3}
        fill="transparent"
        stroke={picked ? 'var(--accent)' : 'transparent'}
        strokeWidth={2}
        className="hover:stroke-accent"
      />
      <text x={mid} y={y + lot.h * T + 9} textAnchor="middle" fontSize={8} fontWeight={700} fill="var(--fg)" {...HALO}>
        {name}
      </text>
      {decisions > 0 && <Badge at={[x + w - 10, y + 9]} text={String(decisions)} fill="var(--danger)" />}
      {waiting > 0 && <Badge at={[x + 10, y + 9]} text={`!${waiting > 1 ? waiting : ''}`} fill="var(--warning)" />}
      {more > 0 && <Badge at={[mid, y + 9]} text={`+${more}`} fill="var(--muted)" />}
    </g>
  );
}

// A label stays readable over the pavement.
const HALO = { stroke: 'var(--bg)', strokeWidth: 2.5, paintOrder: 'stroke', strokeLinejoin: 'round' } as const;

const Badge = ({ at, text, fill }: { at: [number, number]; text: string; fill: string }) => {
  const r = 6;
  const w = Math.max(2 * r, text.length * 5 + 6);
  return (
    <g>
      <rect x={at[0] - w / 2} y={at[1] - r} width={w} height={2 * r} rx={r} fill={fill} stroke="var(--bg)" strokeWidth={1.5} />
      <text x={at[0]} y={at[1] + 2.6} textAnchor="middle" fontSize={7.5} fontWeight={700} fill="var(--accent-fg)">
        {text}
      </text>
    </g>
  );
};

/** Cars driving the streets, one per agent at work up to a few: a road one way, the next the other, its cars half a lap apart. */
function Traffic({ cars, roads, w, h }: { cars: number; roads: number[]; w: number; h: number }) {
  return (
    <g className="pointer-events-none" clipPath="url(#town-streets)">
      <clipPath id="town-streets">
        <rect width={w} height={h} />
      </clipPath>
      {Array.from({ length: cars }, (_, i) => {
        const n = i % roads.length;
        const road = roads[n];
        const right = n % 2 === 1;
        const [c, r] = CARS[i % CARS.length];
        const y = (road + 0.4) * T;
        const dur = 16 + (n % 3) * 4;
        const from = right ? -3 * T : w;
        const to = right ? w : -3 * T;
        return (
          <g key={i}>
            <animateTransform
              attributeName="transform"
              type="translate"
              from={`${from} ${y}`}
              to={`${to} ${y}`}
              dur={`${dur}s`}
              begin={`-${(Math.floor(i / roads.length) * dur) / 2 + n * 2.3}s`}
              repeatCount="indefinite"
            />
            <svg width={3 * T} height={2 * T} viewBox={`${c * T} ${r * T} ${3 * T} ${2 * T}`} overflow="hidden">
              <g transform={right ? `translate(${(2 * c + 3) * T} 0) scale(-1 1)` : undefined}>
                <image href={SHEET} width={37 * T} height={28 * T} style={PIXEL} />
              </g>
            </svg>
          </g>
        );
      })}
    </g>
  );
}

/** What the windows and badges mean, under the city. */
function Legend({ windows }: { windows: HTMLCanvasElement | undefined }) {
  const lit = useMemo(() => windows?.toDataURL(), [windows]);
  const tile = (url: string | undefined, size: [number, number], at: [number, number], filter?: string) => (
    <span
      className="size-4"
      style={{
        ...PIXEL,
        filter,
        backgroundImage: url ? `url(${url})` : undefined,
        backgroundSize: `${size[0] * 16}px ${size[1] * 16}px`,
        backgroundPosition: `-${at[0] * 16}px -${at[1] * 16}px`,
      }}
    />
  );
  const pin = (fill: string, text: string) => (
    <span
      className="flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-xs font-bold text-accent-fg"
      style={{ background: fill }}
    >
      {text}
    </span>
  );
  const rows: [ReactNode, string][] = [
    [tile(lit, [4, 1], [0, 0]), 'A window per agent: working'],
    [tile(lit, [4, 1], [1, 0]), 'Idle or asleep'],
    [tile(SHEET, [37, 28], [2, 7], 'grayscale(1)'), 'Nobody at work'],
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
