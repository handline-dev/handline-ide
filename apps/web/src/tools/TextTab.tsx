import { read } from '#src/persist/kv.ts';
import { Markdown } from '#src/ui/Markdown.tsx';

/** Where a text a tab shows (`text:<key>`) is kept, so the tab still has it after a reload. */
export const textKey = (key: string) => `text/${key}`;

/** A whole text shown read-only as Markdown, such as a part a restart changed (CompactionLine's Show all). */
export function TextTab({ id }: { id: string }) {
  const text = read<string | null>(textKey(id), null);
  return (
    <div className="h-full overflow-auto">
      <div className="mx-auto max-w-3xl px-6 py-5 text-sm text-fg">
        {text == null ? <p className="text-muted">This text is no longer kept in this browser.</p> : <Markdown text={text} />}
      </div>
    </div>
  );
}
