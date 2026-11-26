// The task page's More menu: the verbs its state allows (Done, Back to draft, Reopen, Discard).
import { useWrite } from '@handline/app-data';
import { Check, Ellipsis, FilePen, RotateCcw, Trash2 } from 'lucide-react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { toast } from '#src/ui/toast.tsx';

/** The task's verbs: Done or Reopen, Discard. */
export function Verbs({ id, open, state }: { id: number; open: boolean; state: string }) {
  const done = useWrite('task.done');
  const reopen = useWrite('task.reopen');
  const discard = useWrite('task.discard');
  const draft = useWrite('task.draft');
  const failed = (what: string) => (e: unknown) =>
    toast(`${what} did not work: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
  return (
    // Every verb in the one menu: the state is read at the title, the verbs are asked for here.
    <div className="flex shrink-0 items-center">
      <Menu
        align="end"
        trigger={
          <IconButton label="More">
            <Ellipsis size={16} />
          </IconButton>
        }
      >
        {open && (
          <MenuItem icon={<Check size={14} />} onClick={() => done.mutate({ task: id }, { onError: sayRefusal })}>
            Done
          </MenuItem>
        )}
        {state === 'queued' && (
          <MenuItem icon={<FilePen size={14} />} onClick={() => draft.mutate({ tasks: [id] }, { onError: sayRefusal })}>
            Back to draft
          </MenuItem>
        )}
        {state === 'done' && (
          <MenuItem icon={<RotateCcw size={14} />} onClick={() => reopen.mutate({ task: id }, { onError: failed('Reopen') })}>
            Reopen
          </MenuItem>
        )}
        {open && (
          <MenuItem danger icon={<Trash2 size={14} />} onClick={() => discard.mutate({ task: id }, { onError: sayRefusal })}>
            Discard
          </MenuItem>
        )}
      </Menu>
    </div>
  );
}
