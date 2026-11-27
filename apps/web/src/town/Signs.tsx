// What the city shows over each building (signs.ts lays it out for the scale): the whole building to click, its name on its sign, and
// its counts beside it. Zoomed out, the name and the counts keep their size on screen, on a plate on the roof's edge, and the founder
// carries a tag that can be found among the roofs. The building site's sign is an action: a plate in the accent colour at any scale.
import { useFounder, type Founder, type Ground } from '#src/town/founder.tsx';
import { SIGN_INK, SIGN_PLATE } from '#src/town/palette.ts';
import type { Box } from '#src/town/people.ts';
import { badgeWidth, platesOf, tagFont, type Measure, type Plate, type Post } from '#src/town/signs.ts';
import type { Door } from '#src/town/street.ts';

/** What the city shows of a building besides its windows: its name, a line under it or in its tooltip, and its counts. `action`: it is
 *  something to do, not a team; `short`: what it reads where there is no room for its name. */
export type Sign = {
  name: string;
  short?: string;
  sub: string;
  decisions: number;
  waiting: number;
  stopped: number;
  more: number;
  action?: boolean;
};
/** A count beside a sign; `fault` is drawn as a warning triangle, the others as pills. */
type Mark = { key: string; text: string; fill: string; fault?: boolean };

/** A building's counts in the order drawn: decisions, agents waiting, agents stopped on a fault, agents past its windows. */
const marksOf = (sign: Sign): Mark[] =>
  [
    sign.decisions > 0 && { key: 'decisions', text: String(sign.decisions), fill: 'var(--danger)' },
    sign.waiting > 0 && { key: 'waiting', text: `!${sign.waiting > 1 ? sign.waiting : ''}`, fill: 'var(--warning)' },
    sign.stopped > 0 && { key: 'stopped', text: '!', fill: 'var(--danger)', fault: true },
    sign.more > 0 && { key: 'more', text: `+${sign.more}`, fill: 'var(--muted)' },
  ].filter((m) => m !== false);

const widths = new Map<string, number>();
let pen: CanvasRenderingContext2D | null = null;
/** A text's width in the page's bold letters, for each pixel of its height; measured once. */
const measure: Measure = (text) => {
  const known = widths.get(text);
  if (known !== undefined) return known;
  pen ??= document.createElement('canvas').getContext('2d');
  if (!pen) return text.length * 0.6;
  pen.font = `700 100px ${getComputedStyle(document.body).fontFamily}`;
  const w = pen.measureText(text).width / 100;
  widths.set(text, w);
  return w;
};

export function Signs({
  doors,
  signs,
  boxes,
  scale,
  onPick,
}: {
  doors: Door[];
  signs: Record<string, Sign>;
  boxes: Map<string, Box>;
  scale: number;
  /** A building clicked; `twice` on the second click of a double click. */
  onPick: (door: Door, twice: boolean) => void;
}) {
  const posts = doors.flatMap((d): Post[] => {
    const [s, house, box] = [signs[d.key], boxes.get(`house:${d.key}`), boxes.get(`sign:${d.key}`)];
    if (!s || !house || !box) return [];
    const [roof, marks] = [boxes.get(`eave:${d.key}`)?.y, marksOf(s).map((m) => m.text)];
    return [{ key: d.key, box, room: house.w, row: d.front, roof, name: s.name, short: s.short, sub: s.sub, marks }];
  });
  const plates = new Map(platesOf(posts, scale, measure).map((p) => [p.key, p]));
  return doors.map((d) => {
    const s = signs[d.key];
    const house = boxes.get(`house:${d.key}`);
    const plate = plates.get(d.key);
    if (!s || !house) return null;
    return (
      <g key={d.key} className="group cursor-pointer" onClick={() => onPick(d, false)} onDoubleClick={() => onPick(d, true)}>
        <title>{[s.name, s.sub].filter(Boolean).join(' · ')}</title>
        <rect
          x={house.x - 1}
          y={house.y - 1}
          width={house.w + 2}
          height={house.h + 2}
          rx={2}
          fill="transparent"
          stroke="transparent"
          strokeWidth={1.5 / Math.min(1, scale)}
          className="group-hover:stroke-accent"
        />
        {plate && <Board plate={plate} marks={marksOf(s)} action={!!s.action} />}
      </g>
    );
  });
}

/** A sign: its plate where the building's own is too small or it is an action, its name, the line under it, and its badges in a row
 *  from its right end. */
function Board({ plate, marks, action }: { plate: Plate; marks: Mark[]; action: boolean }) {
  const mid = plate.x + plate.w / 2;
  const ink = action ? 'var(--accent-fg)' : SIGN_INK;
  return (
    <>
      {(action || !plate.drawn) && (
        <rect
          x={plate.x}
          y={plate.y}
          width={plate.w}
          height={plate.h}
          rx={plate.font * 0.25}
          fill={action ? 'var(--accent)' : SIGN_PLATE}
        />
      )}
      <g className="pointer-events-none">
        <text x={mid} y={plate.line + plate.font * 0.35} textAnchor="middle" fontSize={plate.font} fontWeight={700} fill={ink}>
          {plate.name}
        </text>
        {plate.sub && (
          <text
            x={mid}
            y={plate.y + plate.h - plate.subFont * 0.45}
            textAnchor="middle"
            fontSize={plate.subFont}
            fontWeight={500}
            fill={SIGN_INK}
            opacity={0.75}
          >
            {plate.sub}
          </text>
        )}
        <g transform={`translate(${plate.marks.x} ${plate.marks.y}) scale(${plate.marks.scale})`}>
          {marks.map((m, i) => (
            <Badge key={m.key} left={marks.slice(0, i).reduce((n, b) => n + badgeWidth(b.text) + 2, 0)} mark={m} />
          ))}
        </g>
      </g>
    </>
  );
}

/** A badge, its left end `left` along the row and its middle on the row's line: a pill, or for a fault a warning triangle, the shape an
 *  office puts over one stopped, so it is not taken for the red count of decisions. */
const Badge = ({ left, mark: { text, fill, fault } }: { left: number; mark: Mark }) => {
  const w = badgeWidth(text);
  const mid = left + w / 2;
  const triangle = `M${mid} -7.5L${mid + 8.5} 6.5H${mid - 8.5}Z`;
  return (
    <>
      {fault ? (
        <>
          <path d={triangle} fill="var(--bg)" stroke="var(--bg)" strokeWidth={4.5} strokeLinejoin="round" />
          <path d={triangle} fill={fill} stroke={fill} strokeWidth={2} strokeLinejoin="round" />
        </>
      ) : (
        <rect x={left} y={-8} width={w} height={16} rx={8} fill={fill} stroke="var(--bg)" strokeWidth={1.5} />
      )}
      <text x={mid} y={fault ? 5 : 3.5} textAnchor="middle" fontSize={10} fontWeight={700} fill="var(--accent-fg)">
        {text}
      </text>
    </>
  );
};

/** The founder's tag zoomed out, under its feet: as large on screen however small the founder is drawn. */
export function YouTag({ founder, ground, scale }: { founder: Founder; ground: Ground; scale: number }) {
  const at = useFounder(founder, (me) => (me ? ground.toPixel(me.at[0], 0, me.at[1]).map(Math.round).join(',') : null));
  if (!at) return null;
  const [x, y] = at.split(',').map(Number);
  const font = tagFont(scale);
  const [w, h] = [font * 2.7, font * 1.5];
  return (
    <g className="pointer-events-none">
      <rect
        x={x - w / 2}
        y={y + font * 0.2}
        width={w}
        height={h}
        rx={h / 2}
        fill="var(--accent)"
        stroke="var(--bg)"
        strokeWidth={font * 0.12}
      />
      <text x={x} y={y + font * 0.2 + h / 2 + font * 0.35} textAnchor="middle" fontSize={font} fontWeight={700} fill="var(--accent-fg)">
        You
      </text>
    </g>
  );
}
