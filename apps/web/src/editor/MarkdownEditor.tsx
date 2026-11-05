// A Markdown file that a person and agents edit at once. One CodeMirror editor holds the text; edits go out as they are made (saved
// a moment after typing stops) and edits made elsewhere (an agent writing the file) come in and are laid over the person's own,
// unsent ones without losing them. The ordering is @codemirror/collab's: the far side orders, this side rebases. The far side is the
// daemon's shared document (doc.*: it orders every editor's updates and the file's own changes); a file not made yet, or a daemon
// without doc.*, falls back to the file itself, read again while the tab shows and written with its etag.
// Undo and redo walk only the person's own edits: what came in is not undone.
import { collab, getClientID, getSyncedVersion, receiveUpdates, sendableUpdates, type Update } from '@codemirror/collab';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { ChangeSet, EditorState, StateEffect, StateField, type Extension } from '@codemirror/state';
import { Decoration, EditorView, keymap, placeholder, type DecorationSet } from '@codemirror/view';
import { useEffect, useImperativeHandle, useRef, useState, type Ref } from 'react';
import { toggleWrap } from '#src/editor/commands.ts';
import { diffChanges } from '#src/editor/diff.ts';
import { livePreview, setRich } from '#src/editor/livePreview.ts';
import { keptScroll } from '#src/ui/keptScroll.ts';

/** Where the text lives: read it with its etag, write it against one. A stale etag rejects the write (`stale` true). */
export type FileSource = {
  kind: 'file';
  read: () => Promise<{ text: string; etag: string; readOnly?: boolean }>;
  write: (text: string, etag: string) => Promise<{ etag: string }>;
  isStale: (e: unknown) => boolean;
};
/** The daemon's shared document for the file: open it, push updates made on its version, pull what came after one, hear what others
 * pushed or wrote to the file. `refusal` names a push or pull turned back: behind (pull, then push again) or reopen (open again). */
export type SharedSource = {
  kind: 'shared';
  open: () => Promise<{ doc: string; version: number; text: string }>;
  push: (doc: string, version: number, updates: SharedUpdate[]) => Promise<number>;
  pull: (doc: string, version: number) => Promise<{ version: number; updates: SharedUpdate[] }>;
  close: () => void;
  listen: (
    f: (
      n: { kind: 'changed'; doc: string; version: number; updates: SharedUpdate[] } | { kind: 'closed'; doc: string; reason: string },
    ) => void,
  ) => () => void;
  refusal: (e: unknown) => 'behind' | 'reopen' | null;
};
export type SharedUpdate = { client: string; changes: unknown; by?: string };
export type DocSource = FileSource | SharedSource;
export type SyncStatus = 'loading' | 'saved' | 'editing' | 'saving' | { failed: string };

const FAR = 'far';
/** Lit updates fade after this long. */
const FADE_MS = 1600;
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
    const make = (doc: string, version: number, me: string, onEdit: () => void, onSave: () => void, readOnly = false) => {
      const v = new EditorView({
        parent: host.current!,
        state: EditorState.create({
          doc,
          extensions: [...editorExtensions(hint, version, me, onEdit, onSave), EditorState.readOnly.of(readOnly)],
        }),
      });
      view.current = v;
      setReady(true);
      // Found where it was left: the scroll offset, kept as it moves.
      const scroller = v.scrollDOM;
      const at = keep ? keptScroll.get(keep) : undefined;
      if (typeof at === 'number') requestAnimationFrame(() => (scroller.scrollTop = at));
      if (keep) scroller.addEventListener('scroll', () => keptScroll.set(keep, scroller.scrollTop), { passive: true });
      v.focus();
      return v;
    };
    const lightUp = (v: EditorView, tr: Parameters<EditorView['dispatch']>[0] & { changes: ChangeSet }) => {
      const landed: { from: number; to: number }[] = [];
      tr.changes.iterChangedRanges((_a, _b, from, to) => landed.push({ from, to }));
      v.dispatch(tr, { effects: flash.of(landed) });
    };
    const stop =
      source.kind === 'shared'
        ? shared(
            source,
            make,
            lightUp,
            (s) => status.current(s),
            () => view.current,
          )
        : fileSync(source);
    return () => {
      stop();
      view.current?.destroy();
      view.current = null;
    };

    function fileSync(source: FileSource) {
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
        // Lit where the far edit landed after it was laid over the unsent local ones.
        lightUp(v, receiveUpdates(v.state, [update]));
        clearTimeout(fade);
        fade = setTimeout(() => view.current?.dispatch({ effects: unflash.of(null) }), FADE_MS);
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

      say('loading');
      void source.read().then(
        (r) => {
          if (gone || !host.current) return;
          synced = r.text;
          etag = r.etag;
          make(
            r.text,
            0,
            ME,
            () => (say('editing'), schedule()),
            () => void send(),
            !!r.readOnly,
          );
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
      };
    }
    // The source names one file; a new file is a new editor (keyed by its path).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // The rich look follows the prop, once the editor is there.
  useEffect(() => {
    view.current?.dispatch({ effects: setRich.of(rich) });
  }, [rich, ready]);

  return <div ref={host} className="md-editor min-h-0 flex-1 overflow-hidden" />;
}

/** The editor's parts: collab from `version` as `me`, its own undo, Markdown with the live preview, and the keys. */
function editorExtensions(hint: string, version: number, me: string, onEdit: () => void, onSave: () => void): Extension[] {
  return [
    collab({ startVersion: version, clientID: me }),
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
      { key: 'Mod-s', run: () => (onSave(), true), preventDefault: true },
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
      )
        onEdit();
    }),
  ];
}

/** Keeps the editor on the daemon's shared doc: opens it, pushes local updates a moment after typing rests, takes in what others
 * pushed or wrote (lit where it lands), and opens again when the doc was let go. Returns what stops it. */
function shared(
  source: SharedSource,
  make: (doc: string, version: number, me: string, onEdit: () => void, onSave: () => void) => EditorView,
  lightUp: (v: EditorView, tr: ReturnType<typeof receiveUpdates>) => void,
  status: (s: SyncStatus) => void,
  current: () => EditorView | null,
): () => void {
  let gone = false;
  let doc = '';
  let busy = false;
  let settle: ReturnType<typeof setTimeout> | undefined;
  let fade: ReturnType<typeof setTimeout> | undefined;
  const say = (s: SyncStatus) => !gone && status(s);
  const toUpdates = (us: SharedUpdate[]): Update[] => us.map((u) => ({ clientID: u.client, changes: ChangeSet.fromJSON(u.changes) }));

  // Updates after the editor's version, in order: its own confirm what it sent, the rest are laid over its unsent edits and lit.
  const take = (us: SharedUpdate[]) => {
    const v = current();
    if (!v || !us.length) return;
    const me = getClientID(v.state);
    const tr = receiveUpdates(v.state, toUpdates(us));
    if (us.some((u) => u.client !== me)) {
      lightUp(v, tr);
      clearTimeout(fade);
      fade = setTimeout(() => current()?.dispatch({ effects: unflash.of(null) }), FADE_MS);
    } else v.dispatch(tr);
  };
  const pull = async () => {
    const v = current();
    if (!v) return;
    const r = await source.pull(doc, getSyncedVersion(v.state));
    // A notice may have brought some of these while the pull was out.
    const have = getSyncedVersion(current()!.state);
    take(r.updates.slice(have - (r.version - r.updates.length)));
  };
  const reopen = async () => {
    say('loading');
    current()?.destroy();
    await start();
  };
  const send = async () => {
    const v = current();
    if (!v || busy) return;
    const out = sendableUpdates(v.state);
    if (!out.length) return say('saved');
    // It also runs once after the editor closes: the push below starts before the view goes.
    busy = true;
    say('saving');
    const me = getClientID(v.state);
    try {
      await source.push(
        doc,
        getSyncedVersion(v.state),
        out.map((u) => ({ client: me, changes: u.changes.toJSON() })),
      );
      // What was taken comes back in order (a pull, or the notice): pulling now settles it.
      await pull();
    } catch (e) {
      const why = source.refusal(e);
      busy = false;
      if (why === 'behind') return void pull().then(send, () => schedule());
      if (why === 'reopen') return void reopen();
      say({ failed: e instanceof Error ? e.message : String(e) });
      return;
    }
    busy = false;
    const now = current();
    if (now && sendableUpdates(now.state).length) schedule();
    else say('saved');
  };
  const schedule = () => {
    clearTimeout(settle);
    settle = setTimeout(() => void send(), SETTLE_MS);
  };
  const start = async () => {
    try {
      const r = await source.open();
      if (gone) return;
      doc = r.doc;
      make(
        r.text,
        r.version,
        crypto.randomUUID(),
        () => (say('editing'), schedule()),
        () => void send(),
      );
      say('saved');
    } catch (e) {
      say({ failed: e instanceof Error ? e.message : String(e) });
    }
  };

  const unlisten = source.listen((n) => {
    if (gone || n.doc !== doc) return;
    if (n.kind === 'closed') return void reopen();
    const v = current();
    if (!v) return;
    const have = getSyncedVersion(v.state);
    // Already had (a pull or a behind came first): dropped. One that skips ahead asks for what is between.
    if (n.version <= have) return;
    const from = n.version - n.updates.length;
    if (from > have) return void pull().catch(() => {});
    take(n.updates.slice(have - from));
  });
  say('loading');
  void start();
  return () => {
    gone = true;
    unlisten();
    clearTimeout(fade);
    clearTimeout(settle);
    // Whatever is unsent goes out as the editor closes, then the doc is let go.
    void send().finally(() => source.close());
  };
}
