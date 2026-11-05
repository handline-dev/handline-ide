// A Markdown file that a person and agents edit at once. One CodeMirror editor holds the text; edits go out as they are made (saved
// a moment after typing stops) and edits made elsewhere (an agent writing the file) come in and are laid over the person's own,
// unsent ones without losing them. The ordering is @codemirror/collab's: the far side orders, this side rebases. Today the far side is
// the file itself (read again while the tab shows, written with its etag); the daemon's shared document (T8520) takes its place.
// Undo and redo walk only the person's own edits: what came in is not undone.
import { collab, getSyncedVersion, receiveUpdates, sendableUpdates, type Update } from '@codemirror/collab';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { EditorState, StateEffect, StateField, type Extension } from '@codemirror/state';
import { Decoration, EditorView, keymap, placeholder, type DecorationSet } from '@codemirror/view';
import { useEffect, useImperativeHandle, useRef, useState, type Ref } from 'react';
import { toggleWrap } from '#src/editor/commands.ts';
import { diffChanges } from '#src/editor/diff.ts';
import { livePreview, setRich } from '#src/editor/livePreview.ts';
import { keptScroll } from '#src/ui/keptScroll.ts';

/** Where the text lives: read it with its etag, write it against one. A stale etag rejects the write (`stale` true). */
export type DocSource = {
  read: () => Promise<{ text: string; etag: string; readOnly?: boolean }>;
  write: (text: string, etag: string) => Promise<{ etag: string }>;
  isStale: (e: unknown) => boolean;
};
export type SyncStatus = 'loading' | 'saved' | 'editing' | 'saving' | { failed: string };

const FAR = 'far';
/** How long typing rests before the edit is written, and how often the file is read again while the tab shows. */
const SETTLE_MS = 700;
const POLL_MS = 2000;

// What came in from elsewhere is lit for a moment where it landed.
const flash = StateEffect.define<{ from: number; to: number }[]>();
const unflash = StateEffect.define<null>();
const lit = Decoration.mark({ class: 'cm-md-incoming' });
const flashes = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(v, tr) {
    let d = v.map(tr.changes);
    for (const e of tr.effects) {
      if (e.is(flash)) d = d.update({ add: e.value.filter((r) => r.to > r.from).map((r) => lit.range(r.from, r.to)) });
      if (e.is(unflash)) d = Decoration.none;
    }
    return d;
  },
  provide: (f) => EditorView.decorations.from(f),
});

export type EditorHandle = { view: EditorView | null };

export function MarkdownEditor({
  source,
  rich,
  hint,
  onStatus,
  keep,
  ref,
}: {
  /** The name its scroll position is kept under (ui/keptScroll). */
  keep?: string;
  source: DocSource;
  rich: boolean;
  hint: string;
  onStatus: (s: SyncStatus) => void;
  ref?: Ref<EditorHandle>;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const [ready, setReady] = useState(false);
  const status = useRef(onStatus);
  useEffect(() => {
    status.current = onStatus;
  });
  useImperativeHandle(ref, () => ({
    get view() {
      return view.current;
    },
  }));

  useEffect(() => {
    if (!host.current) return;
    // Each editor its own id: two panes on one file are two writers.
    const ME = crypto.randomUUID();
    let gone = false;
    // The text both sides agree on, and the etag it was read or written with.
    let synced = '';
    let etag = '';
    let busy = false;
    let settle: ReturnType<typeof setTimeout> | undefined;
    let fade: ReturnType<typeof setTimeout> | undefined;
    const say = (s: SyncStatus) => !gone && status.current(s);

    const incoming = (text: string, tag: string) => {
      const v = view.current;
      if (!v || text === synced) return void (etag = tag);
      const changes = diffChanges(synced, text);
      const update: Update = { changes, clientID: FAR };
      const tr = receiveUpdates(v.state, [update]);
      // Where the far edit landed after it was laid over the unsent local ones.
      const landed: { from: number; to: number }[] = [];
      tr.changes.iterChangedRanges((_a, _b, from, to) => landed.push({ from, to }));
      v.dispatch(tr, { effects: flash.of(landed) });
      clearTimeout(fade);
      fade = setTimeout(() => view.current?.dispatch({ effects: unflash.of(null) }), 1600);
      synced = text;
      etag = tag;
    };

    const send = async () => {
      const v = view.current;
      if (!v || busy || gone) return;
      const out = sendableUpdates(v.state);
      if (!out.length) return;
      busy = true;
      say('saving');
      // The document now is the agreed text with every unsent edit on it, which is exactly what goes out.
      const text = v.state.doc.toString();
      const version = getSyncedVersion(v.state);
      try {
        const r = await source.write(text, etag);
        if (gone) return;
        const now = view.current!;
        // Our own updates coming back confirm them (collab knows them by client id).
        if (getSyncedVersion(now.state) === version)
          now.dispatch(
            receiveUpdates(
              now.state,
              out.map((u) => ({ ...u, clientID: ME })),
            ),
          );
        synced = text;
        etag = r.etag;
        say(sendableUpdates(now.state).length ? 'editing' : 'saved');
      } catch (e) {
        if (source.isStale(e)) {
          // Changed elsewhere since: take it in, then go again with ours laid over it.
          busy = false;
          await pull();
          return void send();
        }
        say({ failed: e instanceof Error ? e.message : String(e) });
      } finally {
        busy = false;
      }
      if (view.current && sendableUpdates(view.current.state).length) schedule();
    };
    const schedule = () => {
      clearTimeout(settle);
      settle = setTimeout(() => void send(), SETTLE_MS);
    };
    const pull = async () => {
      if (busy || gone) return;
      try {
        const r = await source.read();
        if (!gone && !busy && r.etag !== etag) incoming(r.text, r.etag);
      } catch {
        /* a missed read is retried on the next round */
      }
    };

    const extensions: Extension[] = [
      collab({ startVersion: 0, clientID: ME }),
      history(),
      markdown({ base: markdownLanguage }),
      livePreview(),
      flashes,
      placeholder(hint),
      EditorView.lineWrapping,
      keymap.of([
        { key: 'Mod-b', run: (v) => (toggleWrap(v, '**'), true) },
        { key: 'Mod-i', run: (v) => (toggleWrap(v, '*'), true) },
        // Saving is automatic; ⌘S sends at once instead of waiting.
        { key: 'Mod-s', run: () => (void send(), true), preventDefault: true },
        ...historyKeymap,
        ...defaultKeymap,
        indentWithTab,
      ]),
      EditorView.updateListener.of((u) => {
        if (
          u.docChanged &&
          u.transactions.some(
            (t) =>
              t.isUserEvent('input') || t.isUserEvent('delete') || t.isUserEvent('undo') || t.isUserEvent('redo') || t.isUserEvent('move'),
          )
        ) {
          say('editing');
          schedule();
        }
      }),
    ];

    say('loading');
    void source.read().then(
      (r) => {
        if (gone || !host.current) return;
        synced = r.text;
        etag = r.etag;
        view.current = new EditorView({
          parent: host.current,
          state: EditorState.create({ doc: r.text, extensions: [...extensions, EditorState.readOnly.of(!!r.readOnly)] }),
        });
        setReady(true);
        // Found where it was left: the scroll offset, kept as it moves.
        const scroller = view.current.scrollDOM;
        const at = keep ? keptScroll.get(keep) : undefined;
        if (typeof at === 'number') requestAnimationFrame(() => (scroller.scrollTop = at));
        if (keep) scroller.addEventListener('scroll', () => keptScroll.set(keep, scroller.scrollTop), { passive: true });
        view.current.focus();
        say('saved');
      },
      (e) => say({ failed: e instanceof Error ? e.message : String(e) }),
    );
    const poll = setInterval(() => {
      if (document.visibilityState === 'visible') void pull();
    }, POLL_MS);
    return () => {
      gone = true;
      clearInterval(poll);
      clearTimeout(fade);
      // Whatever is unsent goes out as the editor closes.
      if (settle) {
        clearTimeout(settle);
        const v = view.current;
        if (v && sendableUpdates(v.state).length) void source.write(v.state.doc.toString(), etag).catch(() => {});
      }
      view.current?.destroy();
      view.current = null;
    };
    // The source names one file; a new file is a new editor (keyed by its path).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // The rich look follows the prop, once the editor is there.
  useEffect(() => {
    view.current?.dispatch({ effects: setRich.of(rich) });
  }, [rich, ready]);

  return <div ref={host} className="md-editor min-h-0 flex-1 overflow-hidden" />;
}
