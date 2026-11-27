// What an office shows over and under its people so it reads at a glance (marks.ts and pops.ts decide what; Inside.tsx and Cast.tsx place
// it): an hourglass over one on hold, a red warning over one stopped on a fault, Zzz over the desk of one asleep; over one at work the
// steps done of its task as a bar; and a thought bubble that pops with the new work when it changes. All vector, in picture pixels, so it stays crisp however large the room is drawn, and none of it takes
// a click. Whoever is walking gets nothing until it sits, as with the bubbles of those waiting for the founder.
import { useTasks } from '@handline/app-data';
import { useMovers, type Crowd } from '#src/town/crowd.ts';
import { markOf, papersTip, queueOf, stepsOf, type At, type Mark } from '#src/town/marks.ts';
import type { Person } from '#src/town/model.ts';
import type { Drawn } from '#src/town/office.ts';
import { NAME_HALO, NAME_INK, NAME_MUTED, STEP_DONE, STEP_LEFT } from '#src/town/palette.ts';
import { POP_MS, usePops, type Pop } from '#src/town/pops.ts';

/** A halo's width for text `size` tall: thinner than the names', whose letters are bolder. */
const HALO = 2 / 11;

let pen: CanvasRenderingContext2D | null | undefined;
let family = '';
/** How wide `text` is drawn `size` picture pixels tall at `weight`, in the page's own font; a guess where nothing can measure. Measured
 *  at that very size: a font spaces its letters wider the smaller it is set, and the picture's text is set small and drawn enlarged. */
function widthOf(text: string, size: number, weight: number): number {
  if (pen === undefined) pen = document.createElement('canvas').getContext('2d');
  if (!pen) return text.length * size * 0.62;
  family ||= getComputedStyle(document.body).fontFamily;
  pen.font = `${weight} ${size}px ${family}`;
  return pen.measureText(text).width;
}

const haloed = (size: number) => ({ stroke: NAME_HALO, strokeWidth: size * HALO, paintOrder: 'stroke' as const });

/** A pause sign in a round badge, in a quiet colour: waiting by design. */
function Paused() {
  return (
    <>
      <circle r={4.8} fill={NAME_HALO} stroke={NAME_MUTED} strokeWidth={0.9} />
      <path d="M-1.6 -2.2V2.2M1.6 -2.2V2.2" stroke={NAME_MUTED} strokeWidth={1.5} strokeLinecap="round" />
    </>
  );
}

/** A red warning triangle: stopped on a fault. Not the orange speech bubble of one waiting for the founder. */
function Warning() {
  const d = 'M0 -4.8L5.2 3.9H-5.2Z';
  return (
    <>
      <path d={d} fill={NAME_HALO} stroke={NAME_HALO} strokeWidth={2.6} strokeLinejoin="round" />
      <path d={d} fill="var(--danger)" stroke="var(--danger)" strokeWidth={1.2} strokeLinejoin="round" />
      <path d="M0 -2.1V1" stroke={NAME_HALO} strokeWidth={1.3} strokeLinecap="round" />
      <circle cy={2.7} r={0.75} fill={NAME_HALO} />
    </>
  );
}

/** Zzz rising over an empty desk: its agent is asleep. */
function Asleep() {
  const letters: [string, number, number, number][] = [
    ['z', -4.6, 0, 4.6],
    ['z', -0.8, -3.2, 5.8],
    ['Z', 3.8, -7.4, 7.4],
  ];
  return letters.map(([letter, x, y, size]) => (
    <text key={x} x={x} y={y} textAnchor="middle" fontSize={size} fontWeight={800} fill={NAME_MUTED} {...haloed(size * 1.2)}>
      {letter}
    </text>
  ));
}

/** A person's mark by the head whose top's middle is `head`: beside it, where the bubble of one waiting for the founder goes; Zzz over
 *  the middle of the empty chair. */
function StateMark({ mark, head }: { mark: Mark; head: At }) {
  const [dx, dy] = mark === 'asleep' ? [0, -2] : [9, 0];
  return (
    <g className="pointer-events-none" transform={`translate(${head.x + dx} ${head.y + dy})`}>
      {mark === 'held' ? <Paused /> : mark === 'error' ? <Warning /> : <Asleep />}
    </g>
  );
}

/** The steps done of a person's task as a thin bar with the count beside it, centred over (x, y). */
function Steps({ steps, x, y }: { steps: { text: string; part: number }; x: number; y: number }) {
  // One light pill holds the bar and the count, so both read over a lit screen as over the floor.
  const [bar, tall, size, pad] = [13, 2.4, 4.8, 1.6];
  const [w, h] = [pad + bar + 1.4 + widthOf(steps.text, size, 700) + pad, size + 1.6];
  return (
    <g className="pointer-events-none" transform={`translate(${x - w / 2} ${y - h})`}>
      <rect width={w} height={h} rx={h / 2} fill={NAME_HALO} stroke={NAME_INK} strokeWidth={0.7} />
      <rect x={pad} y={(h - tall) / 2} width={bar} height={tall} rx={tall / 2} fill={STEP_LEFT} />
      {steps.part > 0 && (
        <rect x={pad} y={(h - tall) / 2} width={Math.max(tall, bar * steps.part)} height={tall} rx={tall / 2} fill={STEP_DONE} />
      )}
      <text x={pad + bar + 1.4} y={h / 2 + size * 0.36} fontSize={size} fontWeight={700} fill={NAME_INK}>
        {steps.text}
      </text>
    </g>
  );
}

const POP_CSS = '@keyframes town-pop{0%{opacity:0;translate:0 1.5px}5%{opacity:1;translate:0 0}85%{opacity:1}100%{opacity:0}}';

/** A thought bubble saying `pop.text` over the head at `head`, kept inside a picture `w` wide; it fades as its time ends. */
function Thought({ pop, head, w, scale }: { pop: Pop; head: At; w: number; scale: number }) {
  const size = Math.min(10 / scale, 6);
  const [bw, bh] = [widthOf(pop.text, size, 600) + size * 1.2, size * 1.7];
  const x = Math.min(Math.max(head.x - bw / 2, 2), Math.max(2, w - bw - 2));
  const y = Math.max(1, head.y - 8 - bh);
  return (
    <g className="pointer-events-none" style={{ animation: `town-pop ${POP_MS}ms ease-out forwards` }}>
      <circle cx={head.x + 0.5} cy={head.y - 2.6} r={0.9} fill={NAME_HALO} stroke={NAME_INK} strokeWidth={0.6} />
      <circle cx={head.x + 1.4} cy={head.y - 5.6} r={1.5} fill={NAME_HALO} stroke={NAME_INK} strokeWidth={0.7} />
      <rect x={x} y={y} width={bw} height={bh} rx={bh / 2} fill={NAME_HALO} stroke={NAME_INK} strokeWidth={0.8} />
      <text x={x + bw / 2} y={y + bh / 2 + size * 0.35} textAnchor="middle" fontSize={size} fontWeight={600} fill={NAME_INK}>
        {pop.text}
      </text>
    </g>
  );
}

/** Over the people of an office, once they sit: the mark of each one on hold, stopped or asleep, the steps of the task of each one at
 *  work, and a thought bubble where work just changed (in place of the steps while it shows). `people` is the newest. */
export function Marks({ drawn, crowd, people, scale }: { drawn: Drawn; crowd: Crowd; people: readonly Person[]; scale: number }) {
  const movers = useMovers(crowd);
  const pops = usePops(people);
  const byKey = new Map(people.map((p) => [p.key, p]));
  const seated = drawn.cast.heads.filter((h) => !movers.has(h.key));
  return (
    <>
      <style>{POP_CSS}</style>
      {seated.map((h) => {
        const p = byKey.get(h.key);
        const mark = p && markOf(p.activity);
        if (mark) return <StateMark key={h.key} mark={mark} head={h} />;
        const steps = p?.activity === 'working' && stepsOf(p.steps);
        if (!steps || pops.some((pop) => pop.key === h.key)) return null;
        return <Steps key={h.key} steps={steps} x={h.x} y={h.y - 3} />;
      })}
      {pops.map((pop) => {
        const head = seated.find((h) => h.key === pop.key);
        return head && <Thought key={`${pop.key}:${pop.at}`} pop={pop} head={head} w={drawn.w} scale={scale} />;
      })}
    </>
  );
}

/** What the papers on a person's desk say on hover: its tasks by title, from the open tasks the app already keeps. */
export function usePapersTip(): (p: Pick<Person, 'key' | 'name'>) => string {
  const open = useTasks({ state: 'open' }).data?.items;
  return (p) => papersTip(p.name, queueOf(open ?? [], p.key));
}
