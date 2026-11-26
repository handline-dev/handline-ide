// One "Who it is" part edited in place: the box opens under its row, Save or Cancel closes it.
import { motion } from 'framer-motion';
import { useState } from 'react';
import { spring } from '#src/motion.ts';
import { Button } from '#src/ui/Button.tsx';

const BOX =
  'h-72 w-full resize-y rounded-md border border-border bg-bg px-2 py-1.5 font-mono text-sm text-fg outline-none focus:border-accent';

export function RoleEditor({
  initial,
  busy,
  save,
  close,
}: {
  initial: string;
  busy: boolean;
  save: (text: string) => void;
  close: () => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <motion.div
      initial={{ height: 0, opacity: 0 }}
      animate={{ height: 'auto', opacity: 1 }}
      exit={{ height: 0, opacity: 0 }}
      transition={spring}
      className="overflow-hidden"
    >
      <form
        className="flex flex-col gap-2 px-2 pt-1 pb-3"
        onSubmit={(ev) => {
          ev.preventDefault();
          if (!busy) save(value);
        }}
      >
        <textarea
          autoFocus
          value={value}
          onChange={(ev) => setValue(ev.target.value)}
          onKeyDown={(ev) => {
            const go =
              ev.key === 'Escape' ? close : ev.key === 'Enter' && (ev.metaKey || ev.ctrlKey) ? () => !busy && save(value) : undefined;
            if (!go) return;
            ev.preventDefault();
            go();
          }}
          className={BOX}
        />
        <div className="flex justify-end gap-2">
          <Button onClick={close}>Cancel</Button>
          <Button type="submit" variant="primary" busy={busy}>
            Save
          </Button>
        </div>
      </form>
    </motion.div>
  );
}
