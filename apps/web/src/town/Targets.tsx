// What takes a click or a hover in an office's picture (Inside.tsx), laid over it as page elements at the same boxes, in picture pixels
// times the picture's scale: the founder's desk (the decisions), each person and its desk (a card on hover, its chat on a click, the
// app's own agent menu on a right-click), the papers on a desk (its tasks), the empty desk and the HQ's dotted person (each hires an
// agent), the door (out) and the founder itself (its look). Page elements, not shapes in the picture, so a card and the app's menu can hang on them. A click
// walks the founder there, and the nearby window opens on arrival (NearWindow.tsx); Cmd-click opens it in a tab bar at once. What a tab
// bar shows stays outlined. Clicks between the targets fall through to the floor.
import { PreviewCard as P } from '@base-ui/react/preview-card';
import { useAgents, useNav, useRead } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { Plus } from 'lucide-react';
import { useState, type CSSProperties, type MouseEvent } from 'react';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import { chatOf, withAgent } from '#src/layout/kinds.tsx';
import { SetGoingButton, useSetGoing } from '#src/chat/agent/SetGoing.tsx';
import { NamePicker } from '#src/ui/NamePicker.tsx';
import { floatingSurface } from '#src/ui/Popover.tsx';
import { useFounder, type Founder } from '#src/town/founder.tsx';
import { usePapersTip } from '#src/town/Marks.tsx';
import { tipOf } from '#src/town/marks.ts';
import type { House, Person } from '#src/town/model.ts';
import type { Drawn } from '#src/town/office.ts';
import { type Box } from '#src/town/people.ts';
import { figuresOf } from '#src/town/sheet.ts';
import { PersonCard } from '#src/town/PersonCard.tsx';
import { useTownShown } from '#src/town/Windows.tsx';
import { MyLook } from '#src/town/MyLook.tsx';

/** Cmd-click (Ctrl-click off a Mac): open in a tab bar at once, not on arrival in the nearby window. */
const pinChord = (e: MouseEvent) => e.metaKey || e.ctrlKey;

/** What the founder's own desk opens: the decisions waiting for them. */
export const DESK = { opens: 'requests', title: 'Requests' };

/** A target's look: outlined under the pointer or the keyboard's focus, and all the time while a tab bar shows what it opens. */
const look = (on: boolean) =>
  `pointer-events-auto absolute cursor-pointer rounded-sm outline-accent ${on ? 'outline' : 'hover:outline focus-visible:outline'}`;

/** A person or its desk: its card while the pointer rests on it, `onShow` on a click (`pin` with Cmd, or from the menu), the agent menu
 *  on a right-click. `agent` is missing only until the list is read. */
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
  onShow: (pin: boolean) => void;
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
      onClick={(e) => onShow(pinChord(e))}
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
    <AgentMenu agent={agent} opens={chatOf(person.key)} onShow={() => onShow(true)}>
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
  onSeek,
}: {
  drawn: Drawn;
  scale: number;
  /** The newest of its people: the drawing keeps them as they were when it was last drawn. */
  house: House;
  founder: Founder;
  /** The door was clicked. */
  onOut: () => void;
  /** A click sent the founder to the thing with this key (near.ts thingsOf). */
  onSeek: (key: string) => void;
}) {
  const { show, pin: keep, shows } = useTownShown();
  const papersTip = usePapersTip();
  // While the founder walks, by the keys or to where a click sent it, the pointer is not looking at anyone.
  const walking = useFounder(founder, (me) => !!me?.moving);
  const agents = useAgents().data?.agents;
  const workspaces = useRead('workspaces.list', {}).data?.workspaces ?? [];
  const workspace = workspaces.find((w) => w.key === house.key);
  const { nav, addPin } = useNav();
  // Who the HQ's dotted person can bring in: every agent not pinned yet, the last to change first.
  const unpinned = (agents ?? [])
    .filter((a) => !nav.pins.includes(a.key))
    .sort((a, b) => Date.parse(b.state_since) - Date.parse(a.state_since));
  const teamOf = (key: string) => {
    const ws = unpinned.find((a) => a.key === key)?.workspace;
    return workspaces.find((w) => w.key === ws)?.name ?? '';
  };
  const nameOf = (key: string) => unpinned.find((a) => a.key === key)?.name ?? key;
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
  const personAt = (s: Box & { person: Person; at: [number, number]; face?: 'up' }, id: string, kind: 'desk' | 'spot') => {
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
        onShow={(pin) => {
          onSeek(`${kind}:${person.key}`);
          founder.go(s.at, () => founder.turn(s.face ?? 'right'));
          if (pin) keep(opens, person.name);
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
          onClick={(e) => {
            onSeek('mine');
            founder.sit(mine.seat, mine.at);
            if (pinChord(e)) keep(DESK.opens, DESK.title);
          }}
        />
      )}
      {desks.map((d) => personAt(d, `desk-${d.person.key}`, 'desk'))}
      {papers.map((p) => (
        <button
          key={`papers-${p.person.key}`}
          type="button"
          title={papersTip(p.person)}
          style={at(p)}
          className={look(shows(`tasks|${p.person.key}`))}
          onClick={(e) => {
            onSeek(`papers:${p.person.key}`);
            founder.go(p.at, () => founder.turn('up'));
            if (pinChord(e)) keep(`tasks|${p.person.key}`, `Tasks · ${p.person.name}`);
          }}
        />
      ))}
      {spots.map((s) => personAt(s, `at-${s.person.key}`, 'spot'))}
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
      {/* The HQ's dotted person: an agent picked here is pinned, so it stands in the HQ. */}
      {join && unpinned.length > 0 && (
        <NamePicker
          everyone={unpinned.map((a) => a.key)}
          likely={unpinned.slice(0, 6).map((a) => a.key)}
          label={nameOf}
          words={(key) => `${nameOf(key)} ${teamOf(key)}`}
          note={teamOf}
          placeholder="Pin an agent"
          onOpen={() => founder.go(join.at, () => founder.turn('right'))}
          onPick={addPin}
          trigger={
            <button
              type="button"
              title="Pin an agent"
              style={at(join)}
              className="group pointer-events-auto absolute grid cursor-pointer place-items-center rounded-sm outline-accent focus-visible:outline"
            >
              <Plus size={20} strokeWidth={3} className="text-accent opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100" />
            </button>
          }
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
