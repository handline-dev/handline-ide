// What an agent's finished task looks like in its office (cheers.ts says who): a green tick pops over its head and confetti bursts round
// it twice, while its figure hops (Cast.tsx). All CSS animation in picture pixels, so nothing is drawn again frame by frame.
import { CHEER_MS, useElapsed } from '#src/town/cheers.ts';
import { CONFETTI, NAME_HALO, STEP_DONE } from '#src/town/palette.ts';

/** How long one burst flies; the second starts as the first lands. */
const BURST_MS = 1200;
const PIECES = 20;
/** How much larger than the marks a cheer is drawn: it is news, and a big office draws everyone small. */
const BIG = 2;

/** Each piece of a burst: where it flies to from the head, how it turns, its colour and which burst it is in. Fixed, so every cheer
 *  throws the same, evenly round the head and up more than down. */
const PIECE = Array.from({ length: PIECES }, (_, i) => {
  const second = i >= PIECES / 2;
  const angle = -Math.PI / 2 + ((i % (PIECES / 2)) - 4.5) * 0.3 + (second ? 0.15 : 0);
  const far = 9 + (i % 3) * 2.5;
  return {
    dx: Math.round(Math.cos(angle) * far * 10) / 10,
    dy: Math.round(Math.sin(angle) * far * 10) / 10,
    turn: (i % 2 ? 1 : -1) * (180 + (i % 4) * 90),
    colour: CONFETTI[i % CONFETTI.length],
    delay: second ? BURST_MS * 0.9 : 0,
  };
});

export const CHEER_CSS = [
  '@keyframes town-hop{0%,30%,60%,100%{translate:0 0}15%,45%{translate:0 -4px}}',
  '@keyframes town-tick{0%{opacity:0;scale:.2}12%{opacity:1;scale:1.15}20%{scale:1}85%{opacity:1}100%{opacity:0}}',
  '@keyframes town-confetti{0%{opacity:1;translate:0 0;rotate:0deg}55%{opacity:1;translate:var(--dx) var(--dy)}100%{opacity:0;translate:var(--dx) calc(var(--dy) + 9px);rotate:var(--turn)}}',
].join('');

/** How the figure of one cheering hops, three times through the cheer, `skip` ms in. */
export const hop = (skip: number) => ({ animation: `town-hop ${CHEER_MS / 3}ms ease-out ${-skip}ms 3` });

/** A tick and confetti over the head whose top's middle is (x, y), for a cheer begun at `began`. */
export function Cheer({ x, y, began }: { x: number; y: number; began: number }) {
  const skip = useElapsed(began);
  return (
    <g className="pointer-events-none" transform={`translate(${x} ${y - 2 * BIG}) scale(${BIG})`}>
      {PIECE.map((p, i) => (
        <rect
          key={i}
          x={-1.1}
          y={-0.7}
          width={2.2}
          height={1.4}
          fill={p.colour}
          opacity={0}
          style={{
            ['--dx' as string]: `${p.dx}px`,
            ['--dy' as string]: `${p.dy}px`,
            ['--turn' as string]: `${p.turn}deg`,
            transformBox: 'fill-box',
            transformOrigin: 'center',
            animation: `town-confetti ${BURST_MS}ms cubic-bezier(.2,.7,.4,1) ${p.delay - skip}ms forwards`,
          }}
        />
      ))}
      <g style={{ animation: `town-tick ${CHEER_MS}ms ease-out ${-skip}ms forwards`, transformBox: 'fill-box', transformOrigin: 'center' }}>
        <circle r={4.2} fill={STEP_DONE} stroke={NAME_HALO} strokeWidth={1} />
        <path d="M-1.9 0.1L-0.5 1.5L2 -1.3" fill="none" stroke={NAME_HALO} strokeWidth={1.2} strokeLinecap="round" strokeLinejoin="round" />
      </g>
    </g>
  );
}
