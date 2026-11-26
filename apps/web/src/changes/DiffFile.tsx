// One file of a diff: its path and line counts on a header that folds it, its lines under it (DiffRows), and the comments written on it.
// The header carries the + for a comment on the whole file, which is the only kind a file takes where its line numbers do not hold.
import { anchorOf, commentCount, lineNumbersHold, type DiffFile } from '@handline/app-core';
import { ChevronDown, ChevronRight, MessageSquarePlus } from 'lucide-react';
import { Box, boxHint, Draft, Rows, rowUnder, type Commenting, type Picked } from '#src/changes/DiffRows.tsx';
import { useTabState } from '#src/persist/tabState.ts';
import { IconButton } from '#src/ui/IconButton.tsx';

/** A file the diff has no lines of, shown as one note in their place. */
export const noted = (path: string, note: string): DiffFile => ({
  path,
  status: 'modified',
  binary: false,
  rows: [{ kind: 'meta', text: note }],
  sections: 1,
  cut: false,
  combined: false,
});

/** Why a file's lines carry no numbers, said where its first line would be. */
function unnumbered(file: DiffFile): string {
  if (file.combined) return "A merge's combined diff: its lines are not numbered. A comment here is on the whole file.";
  if (file.sections > 1)
    return 'These changes come from commits that do not follow each other, so their lines are not numbered. A comment here is on the whole file; pick one commit to comment on a line.';
  return 'The diff was cut off inside this file, so its lines are not numbered. A comment here is on the whole file.';
}

/** The place's drafts on files that are not among `paths`, the files drawn: another commit's, or a file that left the diff. Listed
 *  with their path, so every draft the bar counts can be read, changed and removed. */
export function Elsewhere({ commenting, paths }: { commenting: Commenting; paths: string[] }) {
  const rest = commenting.drafts.list.filter((c) => !paths.includes(c.path));
  if (!rest.length) return null;
  return (
    <section className="flex flex-col rounded-md border border-border py-1">
      <h3 className="px-3 pt-1 text-xs text-muted">On files not shown here</h3>
      {rest.map((c) => (
        <Draft key={c.id} c={c} drafts={commenting.drafts} path />
      ))}
    </section>
  );
}

export function File({
  file,
  added,
  deleted,
  open,
  setOpen,
  commenting,
}: {
  file: DiffFile;
  added?: number;
  deleted?: number;
  open: boolean;
  setOpen: (open: boolean) => void;
  /** With it, the file's lines take comments; without, it is only read. */
  commenting?: Commenting;
}) {
  // The lines a box is open on and the words typed in it are kept with the tab, per place and per diff shown: one tab draws several
  // tasks' code (a story's sub-tasks), and a row's place means another line in another of a place's diffs.
  const at = `${commenting?.drafts.key ?? ''}:${commenting?.view ?? ''}:${file.path}`;
  const [picked, setPicked] = useTabState<Picked | null>(`comment-on:${at}`, null);
  const [text, setText] = useTabState(`comment-words:${at}`, '');
  const lines = lineNumbersHold(file);
  const mine = commenting?.drafts.list.filter((c) => c.path === file.path) ?? [];
  const loose = mine.filter((c) => rowUnder(file, lines, commenting!.view, c) < 0);
  const drop = () => {
    setPicked(null);
    setText('');
  };
  const keep = () => {
    const words = text.trim();
    const anchor = picked === 'file' ? { quote: [] } : picked && anchorOf(file.rows, picked.a, picked.b);
    if (!words || !anchor || !commenting) return;
    const { view, commit } = commenting;
    commenting.drafts.add({ path: file.path, view, ...(commit ? { commit } : {}), ...anchor, text: words });
    drop();
  };
  const box = picked && <Box text={text} setText={setText} hint={boxHint(file, picked)} onKeep={keep} onDrop={drop} />;
  return (
    <section
      data-file={file.path}
      ref={(el) => {
        // A reveal (the file asked for from elsewhere) opens it if it was folded.
        const reveal = () => setOpen(true);
        el?.addEventListener('reveal', reveal);
        return () => el?.removeEventListener('reveal', reveal);
      }}
      className="scroll-mt-4 overflow-hidden rounded-md border border-border"
    >
      <div className="flex items-center bg-surface pr-1">
        <button
          type="button"
          onClick={() => setOpen(!open)}
          className="flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          {open ? <ChevronDown size={14} className="text-muted" /> : <ChevronRight size={14} className="text-muted" />}
          <span className="min-w-0 flex-1 truncate font-mono text-fg">{file.path}</span>
          {!open && mine.length > 0 && <span className="shrink-0 text-xs text-accent">{commentCount(mine.length)}</span>}
          {added != null && <span className="text-xs text-success tabular-nums">+{added}</span>}
          {deleted != null && <span className="text-xs text-danger tabular-nums">−{deleted}</span>}
        </button>
        {commenting && (
          <IconButton
            label="Comment on this file"
            onClick={() => {
              setOpen(true);
              setPicked('file');
            }}
          >
            <MessageSquarePlus size={14} />
          </IconButton>
        )}
      </div>
      {open && (
        <pre className="scrollbar-auto overflow-x-auto font-mono text-xs leading-5">
          {commenting && !lines && <p className="px-3 py-1 font-sans text-xs whitespace-normal text-muted">{unnumbered(file)}</p>}
          {loose.map((c) => (
            <Draft key={c.id} c={c} drafts={commenting!.drafts} />
          ))}
          {picked === 'file' && box}
          <Rows file={file} lines={lines} commenting={commenting} picked={picked} setPicked={setPicked} box={box} />
        </pre>
      )}
    </section>
  );
}
