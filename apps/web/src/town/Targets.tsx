// What takes a click or a hover in an office's picture (Inside.tsx), laid over it as page elements at the same boxes, in picture pixels
// times the picture's scale: the founder's desk (the decisions), each person and its desk (a card on hover, its chat on a click, the
// app's own agent menu on a right-click), the papers on a desk (its tasks), the empty desk (hires an agent), and the door (out). Page
// elements, not shapes in the picture, so a card and the app's menu can hang on them. A click first walks the founder there; what
// a tab bar shows stays outlined. Clicks between the targets fall through to the floor.
import { PreviewCard as P } from '@base-ui/react/preview-card';
import { useAgents, useRead } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { useState, type CSSProperties } from 'react';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import { chatOf } from '#src/layout/kinds.tsx';
import { floatingSurface } from '#src/ui/Popover.tsx';
import { useFounder, type Founder } from '#src/town/founder.tsx';
import { usePapersTip } from '#src/town/Marks.tsx';
import { tipOf } from '#src/town/marks.ts';
import type { House, Person } from '#src/town/model.ts';
import type { Drawn } from '#src/town/office.ts';
import { figuresOf, type Box } from '#src/town/people.ts';
import { PersonCard } from '#src/town/PersonCard.tsx';
import { useTownShown } from '#src/town/Windows.tsx';

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
}: {
  person: Person;
  agent: AgentView | undefined;
  row: number | undefined;
  style: CSSProperties;
  shown: boolean;
  /** No card opens: the founder is walking. */
  quiet: boolean;
  onShow: () => void;
}) {
  // The pointer has rested on it; and, clicked or right-clicked, its card stays shut until the pointer has left. Both end the moment
  // the pointer leaves, so a card never opens late, under the next click.
  const [resting, setResting] = useState(false);
  const [used, setUsed] = useState(false);
  const target = (
    <button
      type="button"
      aria-label={tipOf(person)}
      style={style}
      className={look(shown)}
      onClick={() => {
        setUsed(true);
        onShow();
      }}
      onContextMenu={() => setUsed(true)}
      onPointerLeave={() => {
        setResting(false);
        setUsed(false);
      }}
    />
  );
  // The app's hover card (ui/HoverCard.tsx) on the same parts, but for one thing: its panel takes no pointer. In a row of desks a panel
  // that held the pointer would lie over the neighbours and keep them from being hovered or clicked.
  const card = (
    // Only the pointer opens it: the keys' focus comes back here by itself when the menu or a dialog closes.
    <P.Root open={resting && !quiet && !used} onOpenChange={(open, e) => setResting(open && e.reason === 'trigger-hover')}>
      <P.Trigger render={target} delay={300} closeDelay={0} />
      <P.Portal>
        <P.Positioner side="right" align="start" sideOffset={6} collisionPadding={8} className="pointer-events-none z-50">
          <P.Popup className={`${floatingSurface} p-1`}>
            <PersonCard person={person} row={row} />
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
  const workspace = useRead('workspaces.list', {}).data?.workspaces.find((w) => w.key === house.key);
  const rows = figuresOf(drawn.stage.people);
  const { mine, hire, desks, papers, spots } = drawn.cast;
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
      />
    );
  };
  return (
    <div className="pointer-events-none absolute inset-0">
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
