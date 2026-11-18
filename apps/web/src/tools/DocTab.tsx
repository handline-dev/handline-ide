import { useReadAttachment } from '@handline/app-data';
import { useEffect, useState } from 'react';
import { Download } from 'lucide-react';
import { saveAttachment } from '#src/chat/MessageImages.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';

/** A Markdown file attached to a message or a task, read in a tab (`doc:<id>/<name>`), with Save for the file itself. */
export function DocTab({ at }: { at: string }) {
  const id = at.slice(0, at.indexOf('/'));
  const name = at.slice(at.indexOf('/') + 1);
  const read = useReadAttachment();
  const [doc, setDoc] = useState<{ text: string } | { failed: string } | null>(null);
  useEffect(() => {
    let live = true;
    read(id)
      .then((b) => b.text())
      .then(
        (text) => live && setDoc({ text }),
        (e: unknown) => live && setDoc({ failed: e instanceof Error ? e.message : String(e) }),
      );
    return () => void (live = false);
  }, [read, id]);
  const save = saveAttachment(read);
  return (
    <div className="h-full overflow-auto">
      <div className="mx-auto max-w-3xl px-6 py-5 text-sm text-fg">
        <div className="mb-4 flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-muted">{name}</span>
          <button
            type="button"
            onClick={() => void save(id, name)}
            className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
          >
            <Download size={14} />
            Save
          </button>
        </div>
        {!doc ? (
          <p className="text-muted">Loading…</p>
        ) : 'failed' in doc ? (
          <p className="text-danger">Did not load: {doc.failed}</p>
        ) : (
          <Markdown text={doc.text} />
        )}
      </div>
    </div>
  );
}
