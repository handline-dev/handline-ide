// The town on a phone's width: its teams as a list, each with its counts, opening to its people in the sidebar's own rows; a person
// opens its chat in the town's sheet. The picture is a tap away, by the button in the town's top strip.
import { ChevronDown, ChevronRight, List, ListChecks, Map as MapIcon } from 'lucide-react';
import { useState } from 'react';
import { chatOf, workspaceOf } from '#src/layout/kinds.tsx';
import { createStore } from '#src/persist/store.ts';
import { Row } from '#src/sidebar/Row.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { WORDS, type House, type Person } from '#src/town/model.ts';
import { teamLine } from '#src/town/rollcall.ts';
import { useTownShown } from '#src/town/Windows.tsx';

const picture = createStore(false);
/** Whether a phone's width shows the picture rather than the list. */
export const usePictureShown = picture.useValue;

/** List or picture, shown on a phone's width only. */
export function ShapeButton() {
  const shown = usePictureShown();
  return (
    <span className="app-no-drag sm:hidden">
      <IconButton label={shown ? 'Show list' : 'Show picture'} onClick={() => picture.set(!shown)}>
        {shown ? <List size={16} /> : <MapIcon size={16} />}
      </IconButton>
    </span>
  );
}

export function TownList({ houses }: { houses: readonly House[] }) {
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set());
  const flip = (key: string) =>
    setOpen((was) => {
      const now = new Set(was);
      if (!now.delete(key)) now.add(key);
      return now;
    });
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-2">
      {houses.map((h) => (
        <Team key={h.key} house={h} open={open.has(h.key)} onFlip={() => flip(h.key)} />
      ))}
    </div>
  );
}

function Team({ house, open, onFlip }: { house: House; open: boolean; onFlip: () => void }) {
  return (
    <>
      <Row
        opens={workspaceOf(house.key)}
        title={house.name}
        icon={open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        note={teamLine(house)}
        onShow={onFlip}
      />
      {open && (
        <div className="flex flex-col gap-0.5 pl-4">
          {house.people.map((p) => (
            <PersonRow key={p.key} person={p} />
          ))}
        </div>
      )}
    </>
  );
}

function PersonRow({ person }: { person: Person }) {
  const { show, shows } = useTownShown();
  const opens = chatOf(person.key);
  return (
    <Row
      opens={opens}
      title={person.name}
      icon={<ActivityIcon act={person.activity} />}
      selected={shows(opens)}
      trail={
        person.queue > 0 && (
          <span className="flex items-center gap-0.5" title="tasks queued">
            <ListChecks size={12} />
            {person.queue}
          </span>
        )
      }
      note={person.why ?? person.doing ?? WORDS[person.activity]}
      onShow={() => show(opens, person.name)}
    />
  );
}
