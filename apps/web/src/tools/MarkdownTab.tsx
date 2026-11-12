import { classify, hooks, useDocCalls, useServes } from '@handline/app-data';
import { redo, undo } from '@codemirror/commands';
import type { EditorView } from '@codemirror/view';
import {
  Bold,
  Code,
  FilePlus,
  FolderOpen,
  Heading1,
  Heading2,
  Heading3,
  Italic,
  List,
  ListOrdered,
  Quote,
  Redo2,
  SquareCode,
  Strikethrough,
  Undo2,
} from 'lucide-react';
import { useId, useRef, useState, type ReactNode } from 'react';
import { insertCodeBlock, toggleLinePrefix, toggleWrap } from '#src/editor/commands.ts';
import {
  MarkdownEditor,
  type DocSource,
  type EditorHandle,
  type FileSource,
  type SharedSource,
  type SyncStatus,
} from '#src/editor/MarkdownEditor.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { Input } from '#src/ui/Field.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';

type Mode = 'markdown' | 'rich';
const HINT = 'Start writing. Markdown works: # heading, - list, **bold**.';
const UNNAMED = 'Name it first ↑';
const clean = (p: string) => p.trim().replace(/^\.?\//, '');

/** A Markdown file of the agent's worktree that the person and the agents edit at once (editor/MarkdownEditor). Laid out like the
 * Web tab: New, Open and the path on top, the file under it, and a toolbar at the bottom with undo and redo, formatting in the rich
 * look, what is saved, and the switch between the source and the rich look. With no file it is a new one, written once named. */
export function MarkdownTab({ agent, tab, start = '' }: { agent: string; tab: string; start?: string }) {
  const [path, setPath] = usePersisted(`tab/${tab}/path`, start);
  const [mode, setMode] = usePersisted<Mode>(`tab/${tab}/mode`, 'markdown');
  // The file the editor was opened on: a new file keeps its editor when it is named, so what was written stays.
  const [opened, setOpened] = useState(path);
  const [typed, setTyped] = useState(path);
  const [status, setStatus] = useState<SyncStatus>('loading');
  const list = useId();
  const address = useRef<HTMLInputElement>(null);
  const editor = useRef<EditorHandle>(null);
  const files = hooks.useFilesList({ agent, query: typed.endsWith('.md') ? typed : `${typed}.md`, limit: 20 });
  const md = (files.data?.files ?? []).filter((f) => /\.md$/i.test(f.path));
  const calls = useDocCalls(agent);
  // Where writes go, read as they happen: a new file gets its path when it is named.
  const target = useRef(path);
  const [named, setNamed] = useState(path);
  const name = (p: string) => {
    target.current = p;
    setNamed(p);
  };
  const [file] = useState<FileSource>(() => ({
    kind: 'file',
    read: async () => {
      if (!target.current) return { text: '', etag: '' };
      const r = await calls.read(target.current);
      return { text: r.text, etag: r.etag, readOnly: r.truncated };
    },
    write: async (text, etag) => {
      if (!target.current) throw new Error(UNNAMED);
      const r = await calls.write(target.current, text, etag);
      setPath(target.current);
      return { etag: r.etag };
    },
    isStale: (e) => {
      const f = classify(e);
      return f.kind === 'refused' && f.gate === 'stale';
    },
  }));
  // A file that is there is edited through the daemon's shared doc, with everyone else on it; a new one is written as a file.
  const docs = useServes('doc.open').served;
  const [shared] = useState<SharedSource>(() => ({
    kind: 'shared',
    open: () => calls.open(target.current),
    push: (doc, version, updates) => calls.push(target.current, doc, version, updates),
    pull: (doc, version) => calls.pull(target.current, doc, version),
    close: () => calls.close(target.current),
    listen: (f) =>
      calls.listen(
        () => target.current,
        (n) =>
          f(
            n.kind === 'doc_changed'
              ? { kind: 'changed', doc: n.doc, version: n.version, updates: n.updates }
              : { kind: 'closed', doc: n.doc, reason: n.reason },
          ),
      ),
    refusal: (e) => {
      const f = classify(e);
      return f.kind === 'refused' && (f.gate === 'behind' || f.gate === 'reopen') ? f.gate : null;
    },
  }));
  const source: DocSource = opened && docs ? shared : file;
  const open = (p: string) => {
    name(p);
    setPath(p);
    setOpened(p);
    setTyped(p);
  };
  const act = (f: (v: EditorView) => unknown) => {
    const v = editor.current?.view;
    if (v) f(v);
  };
  const isNew = !opened && !named;

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <form
        className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-2"
        onSubmit={(e) => {
          e.preventDefault();
          const p = clean(typed);
          if (!p) return;
          // A new file is named in place; an open one makes way for the file typed.
          if (isNew) {
            name(p);
            setTyped(p);
            editor.current?.view?.focus();
          } else open(p);
        }}
      >
        <span className="flex shrink-0 items-center">
          <IconButton label="New file" disabled={isNew} onClick={() => open('')}>
            <FilePlus size={14} />
          </IconButton>
          <IconButton
            label="Open a file"
            onClick={() => {
              setTyped('');
              address.current?.focus();
            }}
          >
            <FolderOpen size={14} />
          </IconButton>
        </span>
        <Input
          ref={address}
          list={list}
          className="h-7 flex-1"
          placeholder={isNew ? 'New file: type its path, or find one' : 'docs/README.md'}
          value={typed}
          onChange={(e) => {
            setTyped(e.target.value);
            // A pick from the list opens at once.
            if (md.some((f) => f.path === e.target.value)) open(e.target.value);
          }}
          onFocus={(e) => e.target.select()}
        />
        <datalist id={list}>
          {md.map((f) => (
            <option key={f.path} value={f.path} />
          ))}
        </datalist>
      </form>
      <MarkdownEditor
        key={opened || 'new'}
        keep={`file:${agent}:${opened || tab}`}
        ref={editor}
        source={source}
        rich={mode === 'rich'}
        hint={HINT}
        onStatus={setStatus}
      />
      <div className="flex h-9 shrink-0 items-center gap-0.5 border-t border-border px-2 text-xs text-muted">
        <Tool label="Undo (⌘Z)" icon={<Undo2 size={14} />} onClick={() => act(undo)} />
        <Tool label="Redo (⇧⌘Z)" icon={<Redo2 size={14} />} onClick={() => act(redo)} />
        {mode === 'rich' && (
          <>
            <Gap />
            <Tool label="Bold (⌘B)" icon={<Bold size={14} />} onClick={() => act((v) => toggleWrap(v, '**'))} />
            <Tool label="Italic (⌘I)" icon={<Italic size={14} />} onClick={() => act((v) => toggleWrap(v, '*'))} />
            <Tool label="Strikethrough" icon={<Strikethrough size={14} />} onClick={() => act((v) => toggleWrap(v, '~~'))} />
            <Tool label="Code" icon={<Code size={14} />} onClick={() => act((v) => toggleWrap(v, '`'))} />
            <Gap />
            <Tool label="Heading 1" icon={<Heading1 size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '# '))} />
            <Tool label="Heading 2" icon={<Heading2 size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '## '))} />
            <Tool label="Heading 3" icon={<Heading3 size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '### '))} />
            <Gap />
            <Tool label="Bulleted list" icon={<List size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '- '))} />
            <Tool label="Numbered list" icon={<ListOrdered size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '1. '))} />
            <Tool label="Quote" icon={<Quote size={14} />} onClick={() => act((v) => toggleLinePrefix(v, '> '))} />
            <Tool label="Code block" icon={<SquareCode size={14} />} onClick={() => act(insertCodeBlock)} />
          </>
        )}
        <span className="mr-2 ml-auto">{statusText(status)}</span>
        <div role="radiogroup" aria-label="View" className="flex rounded-lg bg-surface p-1">
          {(['markdown', 'rich'] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              onClick={() => setMode(m)}
              className={`rounded-sm px-2 py-0.5 ${mode === m ? 'bg-bg text-fg shadow-popover' : 'text-muted hover:text-fg'}`}
            >
              {m === 'markdown' ? 'Markdown' : 'Rich'}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

const statusText = (s: SyncStatus): ReactNode =>
  typeof s === 'object' ? (
    <span className="text-danger">{s.failed}</span>
  ) : (
    { loading: 'Loading', saved: 'Saved', editing: '', saving: 'Saving' }[s]
  );

/** A toolbar button that leaves the editor's focus and selection where they are. */
function Tool({ label, icon, onClick }: { label: string; icon: ReactNode; onClick: () => void }) {
  return (
    <span onMouseDown={(e) => e.preventDefault()}>
      <IconButton label={label} onClick={onClick}>
        {icon}
      </IconButton>
    </span>
  );
}
const Gap = () => <span className="w-2" />;
