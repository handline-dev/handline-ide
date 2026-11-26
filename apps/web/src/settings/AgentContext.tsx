// An agent's settings tab, its top: who it is (its role and standing context, each edited in place under its row and in force from its next
// fresh start) and its memory notes, each removable.
import { useAgentContext, useAgentMemory, useWrite, why } from '@handline/app-data';
import { AnimatePresence } from 'framer-motion';
import { Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Block } from '#src/settings/Meter.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { RoleEditor } from '#src/settings/RoleEditor.tsx';
import { toast } from '#src/ui/toast.tsx';

type Part = 'role' | 'base';
const TITLE: Record<Part, string> = { role: 'Role', base: 'Standing context' };

/** The text with its blank lines dropped, so the preview's lines carry words. */
const preview = (text: string) => text.replace(/\n\s*\n+/g, '\n').trim();

/** Its role and standing context, a row each with its first lines; Edit opens the whole text under the row. */
export function Role({ agent }: { agent: string }) {
  const context = useAgentContext(agent).data;
  const set = useWrite('agent.context.set', agent);
  const [open, setOpen] = useState<Part>();
  if (!context) return null;
  const save = (part: Part, text: string) => {
    if (text.trim() === (context[part]?.text ?? '').trim()) return setOpen(undefined);
    set.mutate(
      { agent, [part]: text },
      {
        onSuccess: () => setOpen((o) => (o === part ? undefined : o)),
        onError: (e) => toast(`Did not save: ${why(e) || 'try again'}`, { tone: 'warning' }),
      },
    );
  };
  return (
    <Block title="Who it is" note="An edit applies when it next starts fresh.">
      <ul className="-mx-2 flex flex-col">
        {(['role', 'base'] as const).map((part) => (
          <li key={part} className="border-t border-border first:border-t-0">
            <div className="flex items-start gap-3 px-2 py-2">
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="text-sm text-fg">{TITLE[part]}</span>
                {open !== part && (
                  <span className="line-clamp-4 text-xs whitespace-pre-line text-muted">
                    {preview(context[part]?.text ?? '') || 'None'}
                  </span>
                )}
              </span>
              <Button
                variant="ghost"
                aria-expanded={open === part}
                className={open === part ? 'invisible' : ''}
                onClick={() => setOpen(part)}
              >
                Edit
              </Button>
            </div>
            <AnimatePresence initial={false}>
              {open === part && (
                <RoleEditor
                  initial={context[part]?.text ?? ''}
                  busy={set.isPending}
                  save={(text) => save(part, text)}
                  close={() => setOpen(undefined)}
                />
              )}
            </AnimatePresence>
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
