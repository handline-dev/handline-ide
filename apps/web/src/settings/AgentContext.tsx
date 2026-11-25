// An agent's settings tab, its top: who it is (its role and standing context, each edited in a modal and in force from its next
// fresh start) and its memory notes, each removable.
import { useAgentContext, useAgentMemory, useWrite, why } from '@handline/app-data';
import { Trash2 } from 'lucide-react';
import { Block } from '#src/settings/Meter.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { prompt } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

type Part = 'role' | 'base';
const TITLE: Record<Part, string> = { role: 'Role', base: 'Standing context' };

/** Its role and standing context, a row each with its first lines; Edit opens the whole text in a modal. */
export function Role({ agent }: { agent: string }) {
  const context = useAgentContext(agent).data;
  const set = useWrite('agent.context.set', agent);
  if (!context) return null;
  const edit = async (part: Part) => {
    const was = context[part]?.text ?? '';
    const text = await prompt({ title: TITLE[part], initial: was, submitText: 'Save', long: true });
    if (text === undefined || text.trim() === was.trim()) return;
    set.mutate({ agent, [part]: text }, { onError: (e) => toast(`Did not save: ${why(e) || 'try again'}`, { tone: 'warning' }) });
  };
  return (
    <Block title="Who it is" note="An edit applies when it next starts fresh.">
      <ul className="-mx-2 flex flex-col">
        {(['role', 'base'] as const).map((part) => (
          <li key={part} className="flex items-start gap-3 border-t border-border px-2 py-2 first:border-t-0">
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="text-sm text-fg">{TITLE[part]}</span>
              <span className="line-clamp-2 text-xs whitespace-pre-line text-muted">{context[part]?.text || 'None'}</span>
            </span>
            <Button variant="ghost" disabled={set.isPending} onClick={() => void edit(part)}>
              Edit
            </Button>
          </li>
        ))}
      </ul>
    </Block>
  );
}

/** Its memory notes, one row each: the note's name and what it is about, removable. */
export function Memory({ agent }: { agent: string }) {
  const notes = useAgentMemory(agent).data?.notes;
  const remove = useWrite('agent.memory.remove', agent);
  if (!notes) return null;
  return (
    <Block title={`Memory · ${notes.length}`}>
      {!notes.length && <p className="text-sm text-muted">Nothing yet.</p>}
      {!!notes.length && (
        <ul className="-mx-2 flex flex-col">
          {notes.map((n) => (
            <li key={n.file} className="group flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0 hover:bg-hover/30">
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="text-sm text-fg">{n.name}</span>
                <span className="line-clamp-2 text-xs text-muted">{n.description || n.body}</span>
              </span>
              <IconButton
                label="Remove this note"
                className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                disabled={remove.isPending}
                onClick={() =>
                  remove.mutate(
                    { agent, name: n.file },
                    { onError: (e) => toast(`Did not remove: ${why(e) || 'try again'}`, { tone: 'warning' }) },
                  )
                }
              >
                <Trash2 size={14} />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
    </Block>
  );
}
