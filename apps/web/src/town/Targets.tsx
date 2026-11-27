// What takes a click or a hover in an office's picture (Inside.tsx), laid over it as page elements at the same boxes, in picture pixels
// times the picture's scale: the founder's desk (the decisions), each person and its desk (a card on hover, its chat on a click, the
// app's own agent menu on a right-click), the papers on a desk (its tasks), the empty desk and the HQ's dotted person (each hires an
// agent), the door (out) and the founder itself (its look). Page elements, not shapes in the picture, so a card and the app's menu can hang on them. A click first
// walks the founder there; what a tab bar shows stays outlined. Clicks between the targets fall through to the floor.
import { PreviewCard as P } from '@base-ui/react/preview-card';
import { useAgents, useNav, useRead } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { Plus } from 'lucide-react';
import { useState, type CSSProperties } from 'react';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent, newAgentIn } from '#src/NewAgent.tsx';
import { chatOf, withAgent } from '#src/layout/kinds.tsx';
import { SetGoingButton, useSetGoing } from '#src/chat/agent/SetGoing.tsx';
import { floatingSurface } from '#src/ui/Popover.tsx';
import { useFounder, type Founder } from '#src/town/founder.tsx';
import { usePapersTip } from '#src/town/Marks.tsx';
import { tipOf } from '#src/town/marks.ts';
import type { House, Person } from '#src/town/model.ts';
import type { Drawn } from '#src/town/office.ts';
import { figuresOf, type Box } from '#src/town/people.ts';
import { PersonCard } from '#src/town/PersonCard.tsx';
import { useTownShown } from '#src/town/Windows.tsx';
import { MyLook } from '#src/town/MyLook.tsx';

/** What the founder's own desk opens: the decisions waiting for them. */
export const DESK = { opens: 'requests', title: 'Requests' };

/** A target's look: outlined under the pointer or the keyboard's focus, and all the time while a tab bar shows what it opens. */
const look = (on: boolean) =>
  `pointer-events-auto absolute cursor-pointer rounded-sm outline-accent ${on ? 'outline' : 'hover:outline focus-visible:outline'}`;

/** A person or its desk: its card while the pointer rests on it, `onShow` on a click, the agent menu on a right-click. `agent` is
 *  missing only until the list is read. */
function PersonTarget({
  person,
  agent,
  row,
  style,
  shown,
  quiet,
  onShow,
  onTerminal,
}: {
  person: Person;
  agent: AgentView | undefined;
  row: number | undefined;
  style: CSSProperties;
  shown: boolean;
  /** No card opens: the founder is walking, to this person too; its card opens once the walk ends here. */
  quiet: boolean;
  onShow: () => void;
  /** Opens its terminal in the town's windows. */
  onTerminal: () => void;
}) {
  // The pointer has rested on it; and, right-clicked, its card stays shut until the pointer has left. Both end the moment the pointer
  // leaves, so a card never opens late, under the next click; a card with a button keeps open while the pointer crosses to it.
  const [resting, setResting] = useState(false);
  const [used, setUsed] = useState(false);
  const going = useSetGoing(person.key);
  const open = resting && !quiet && !used;
  const target = (
    <button
      type="button"
      aria-label={tipOf(person)}
      style={style}
      className={look(shown)}
      onClick={onShow}
      onContextMenu={() => setUsed(true)}
      onPointerLeave={() => {
        // A card that shows with a button is closed by its own leave, so the pointer can cross to it; any other rest ends here.
        if (!going || !open) setResting(false);
        setUsed(false);
      }}
    />
  );
  // The app's hover card (ui/HoverCard.tsx) on the same parts, but for one thing: its panel takes no pointer unless it has a button. In a
  // row of desks a panel that held the pointer would lie over the neighbours and keep them from being hovered or clicked.
  const card = (
    // Only the pointer opens it: the keys' focus comes back here by itself when the menu or a dialog closes.
    <P.Root open={open} onOpenChange={(open, e) => setResting(open && e.reason === 'trigger-hover')}>
      <P.Trigger render={target} delay={300} closeDelay={0} />
      <P.Portal>
        <P.Positioner
          side="right"
          align="start"
          sideOffset={6}
          collisionPadding={8}
          className={`z-50 ${going ? 'pointer-events-auto' : 'pointer-events-none'}`}
        >
          <P.Popup className={`${floatingSurface} p-1`}>
            <PersonCard
              person={person}
              row={row}
              action={going && <SetGoingButton agent={person.key} going={going} terminal={{ onClick: onTerminal }} />}
            />
          </P.Popup>
        </P.Positioner>
      </P.Portal>
    </P.Root>
  );
  if (!agent) return card;
  return (
    <AgentMenu agent={agent} opens={chatOf(person.key)} onShow={onShow}>
      {card}
    </AgentMenu>
  );
}

export function Targets({
  drawn,
  scale,
  house,
  founder,
  onOut,
}: {
  drawn: Drawn;
  scale: number;
  /** The newest of its people: the drawing keeps them as they were when it was last drawn. */
  house: House;
  founder: Founder;
  /** The door was clicked. */
  onOut: () => void;
}) {
  const { show, shows } = useTownShown();
  const papersTip = usePapersTip();
  // While the founder walks, by the keys or to where a click sent it, the pointer is not looking at anyone.
  const walking = useFounder(founder, (me) => !!me?.moving);
  const agents = useAgents().data?.agents;
  const workspaces = useRead('workspaces.list', {}).data?.workspaces ?? [];
  const workspace = workspaces.find((w) => w.key === house.key);
  const { addPin } = useNav();
  const rows = figuresOf(drawn.stage.people);
  const { mine, hire, join, desks, papers, spots } = drawn.cast;
  const door = drawn.front.door;
  const decisions = house.decisions.length;
  /** A box of the picture on the page, a picture pixel wider all round. */
  const at = (b: Box, pad = 1): CSSProperties => ({
    left: (b.x - pad) * scale,
    top: (b.y - pad) * scale,
    width: (b.w + 2 * pad) * scale,
    height: (b.h + 2 * pad) * scale,
  });
  const personAt = (s: Box & { person: Person; at: [number, number]; face?: 'up' }, id: string) => {
    const person = house.people.find((p) => p.key === s.person.key) ?? s.person;
    const opens = chatOf(person.key);
    return (
      <PersonTarget
        key={id}
        person={person}
        agent={agents?.find((a) => a.key === person.key)}
        row={rows.get(person.key)}
        style={at(s)}
        shown={shows(opens)}
        quiet={walking}
        onShow={() => {
          founder.go(s.at, () => founder.turn(s.face ?? 'right'));
          show(opens, person.name);
        }}
        onTerminal={() => show(withAgent('shell', person.key), `Terminal · ${person.name}`)}
      />
    );
  };
  return (
    <div className="pointer-events-none absolute inset-0">
      {/* First, so a person, desk or papers the founder stands over still take the click. */}
      <MyLook founder={founder} ground={drawn.ground} scale={scale} />
      {mine && (
        <button
          type="button"
          title={`Your desk · ${decisions} ${decisions === 1 ? 'decision' : 'decisions'}`}
          style={at(mine)}
          className={look(shows(DESK.opens))}
          onClick={() => {
            founder.sit(mine.seat, mine.at);
            show(DESK.opens, DESK.title);
          }}
        />
      )}
      {desks.map((d) => personAt(d, `desk-${d.person.key}`))}
      {papers.map((p) => (
        <button
          key={`papers-${p.person.key}`}
          type="button"
          title={papersTip(p.person)}
          style={at(p)}
          className={look(shows(`tasks|${p.person.key}`))}
          onClick={() => {
            founder.go(p.at, () => founder.turn('up'));
            show(`tasks|${p.person.key}`, `Tasks · ${p.person.name}`);
          }}
        />
      ))}
      {spots.map((s) => personAt(s, `at-${s.person.key}`))}
      {hire && workspace && (
        <button
          type="button"
          title={`Hire an agent for ${workspace.name}`}
          style={at(hire)}
          className={look(false)}
          onClick={() => {
            founder.go(hire.at, () => founder.turn('right'));
            void newAgent(workspace);
          }}
        />
      )}
      {/* The HQ's dotted person: one hired here also stands in the HQ, pinned. */}
      {join && workspaces.length > 0 && (
        <button
          type="button"
          title="Hire an agent"
          style={at(join)}
          className="group pointer-events-auto absolute grid cursor-pointer place-items-center rounded-sm outline-accent focus-visible:outline"
          onClick={() => {
            founder.go(join.at, () => founder.turn('right'));
            void newAgentIn(workspaces).then((key) => key && addPin(key));
          }}
        >
          <Plus size={20} strokeWidth={3} className="text-accent opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100" />
        </button>
      )}
      {door && (
        <button
          type="button"
          title="Go out"
          style={at(door, 0)}
          className="pointer-events-auto absolute cursor-pointer rounded-sm outline-accent focus-visible:outline"
          onClick={onOut}
        />
      )}
    </div>
  );
}
