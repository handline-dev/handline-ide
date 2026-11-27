// The command palette: one box that goes to a place or finds what was agreed. Cmd-K on a Mac opens and closes it (elsewhere Ctrl-K is the
// terminal's kill-line, so it opens from the sidebar's Search row only). Arrows move, Enter opens the row the app's one way: here,
// Cmd-Enter in a new tab, Option-Enter beside (layout/openTab howOf). It is a dialog, so the tab keys behind it rest.
import { isPaletteChord, stepRow } from '@handline/app-core';
import { Dialog } from '@base-ui/react/dialog';
import { ArrowLeft, Search } from 'lucide-react';
import { useEffect, useRef, useState, type KeyboardEvent as InputKey } from 'react';
import { useOpenable } from '#src/layout/openable.tsx';
import { useWindowLayer } from '#src/layout/windowLayer.tsx';
import type { Entry } from '#src/palette/places.tsx';
import { useSections } from '#src/palette/sections.tsx';
import { closePalette, togglePalette, usePaletteOpen } from '#src/palette/store.ts';
import { useSettingsOpen } from '#src/settings/open.ts';
import { useTownOpen } from '#src/town/history.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { MAC } from '#src/ui/keys.ts';
import { Spinner } from '#src/ui/Loading.tsx';
import { useNarrow } from '#src/ui/narrow.ts';

/** The shortcut as the Search row and its title show it; none off a Mac. */
export const PALETTE_KEYS = MAC ? '⌘K' : '';
const PALETTE = 'data-palette';
const ROW = 'data-palette-row';

/** While it is open it is a full-screen layer: nothing under it moves the window. It takes no history entry, as a modal does for
 * Back to close it: the palette's whole use is to go somewhere, and each going would leave that entry behind, in Forward's way. */
function Layer() {
  useWindowLayer(true);
  return null;
}

/** Mounted once at the app's root: the key, and the palette while it is open. */
export function PaletteHost() {
  const open = usePaletteOpen();
  const narrow = useNarrow();
  // Settings and the town cover the workspace the palette goes about in (and its Search row): the key rests under them.
  const town = useTownOpen();
  const settings = useSettingsOpen();
  const covered = town || settings;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!isPaletteChord(e, MAC) || covered) return;
      // Not behind another dialog, as the tab keys.
      if (document.querySelector(`[role="dialog"]:not([${PALETTE}]), [role="alertdialog"]`)) return;
      e.preventDefault();
      e.stopPropagation();
      togglePalette();
    };
    // Captured, so a terminal or an editor with the focus does not take the key first.
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [covered]);
  return (
    <Dialog.Root open={open} onOpenChange={(next) => !next && closePalette()}>
      <Dialog.Portal>
        <Layer />
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-fg/40 transition-opacity duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className={`fixed inset-0 z-50 flex items-start justify-center ${narrow ? '' : 'px-4 pt-24 pb-8'}`}>
          <Dialog.Popup
            {...{ [PALETTE]: '' }}
            aria-label="Search"
            className={`flex w-full flex-col bg-bg text-fg outline-none transition duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0 ${
              narrow ? 'h-full' : 'max-h-full max-w-2xl rounded-lg border border-border shadow-popover'
            }`}
          >
            <Panel narrow={narrow} />
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Panel({ narrow }: { narrow: boolean }) {
  const [query, setQuery] = useState('');
  const [at, setAt] = useState(0);
  const { sections, searching } = useSections(query);
  const list = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLInputElement>(null);
  // The box takes the keys as the palette opens.
  useEffect(() => box.current?.focus(), []);
  const count = sections.reduce((n, s) => n + s.entries.length, 0);
  const sel = Math.min(at, Math.max(0, count - 1));
  // The picked row stays in view as the arrows move it.
  useEffect(() => {
    list.current?.querySelector(`[${ROW}="${sel}"]`)?.scrollIntoView({ block: 'nearest' });
  }, [sel]);
  const onKeyDown = (e: InputKey) => {
    if (e.nativeEvent.isComposing) return; // a Korean syllable still being made keeps its Enter and arrows
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || ((e.key === 'Home' || e.key === 'End') && !query)) {
      e.preventDefault();
      setAt(stepRow(sel, count, e.key));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      // The row's own click opens it, so the keys held open it as they would with a click (howOf).
      list.current
        ?.querySelector(`[${ROW}="${sel}"]`)
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true, metaKey: e.metaKey, ctrlKey: e.ctrlKey, altKey: e.altKey }));
    }
  };
  // Each section's first row, counted through the sections before it.
  const firstOf = sections.map((_, k) => sections.slice(0, k).reduce((m, s) => m + s.entries.length, 0));
  return (
    <>
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-3">
        {narrow ? (
          <IconButton label="Close" onClick={closePalette}>
            <ArrowLeft size={16} />
          </IconButton>
        ) : (
          <span className="grid size-7 shrink-0 place-items-center text-muted">{searching ? <Spinner /> : <Search size={16} />}</span>
        )}
        <input
          ref={box}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setAt(0);
          }}
          onKeyDown={onKeyDown}
          role="combobox"
          aria-expanded
          aria-controls="palette-list"
          aria-label="Go to a place, a task number, or search"
          placeholder="Go to, a task number, or search"
          maxLength={200}
          spellCheck={false}
          autoComplete="off"
          className="h-full min-w-0 flex-1 bg-transparent text-base text-fg outline-none placeholder:text-muted"
        />
        {narrow && searching && <Spinner />}
      </div>
      <div ref={list} id="palette-list" role="listbox" aria-label="Results" className="min-h-0 flex-1 overflow-y-auto py-1">
        {sections.map((s, k) => (
          <div key={s.title} role="group" aria-label={s.title} className="py-1">
            <div className="px-4 py-1 text-xs font-medium text-muted">{s.title}</div>
            {s.entries.map((entry, j) => {
              const i = firstOf[k] + j;
              return <Row key={entry.key} entry={entry} index={i} selected={i === sel} onPoint={() => setAt(i)} />;
            })}
            {s.note && <p className="px-4 py-1 text-sm text-muted">{s.note}</p>}
          </div>
        ))}
      </div>
      {!narrow && (
        <div className="flex shrink-0 gap-4 border-t border-border px-4 py-2 text-xs text-muted">
          <span>↵ Open</span>
          <span>{MAC ? '⌘' : 'Ctrl'} ↵ New tab</span>
          <span>{MAC ? '⌥' : 'Alt'} ↵ Beside</span>
          <span className="ml-auto">Esc Close</span>
        </div>
      )}
    </>
  );
}

/** One row. Its click, middle click, drag and right-click are every openable's (layout/openable); the palette closes as it opens. */
function Row({ entry, index, selected, onPoint }: { entry: Entry; index: number; selected: boolean; onPoint: () => void }) {
  const open = useOpenable(entry.opens, {
    here: entry.here,
    onOpen: () => {
      closePalette();
      entry.then?.();
    },
  });
  return (
    <div
      {...open}
      {...{ [ROW]: index }}
      role="option"
      aria-selected={selected}
      onMouseMove={onPoint}
      className={`mx-1 flex cursor-default flex-col rounded-md px-3 py-1.5 text-sm select-none ${selected ? 'bg-hover' : ''}`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span className="grid size-4 shrink-0 place-items-center text-muted">{entry.icon}</span>
        <span className="min-w-0 truncate text-fg">{entry.label}</span>
        {entry.hint && <span className="ml-auto shrink-0 pl-2 text-xs text-muted">{entry.hint}</span>}
      </span>
      {entry.note && <span className="truncate pl-6 text-xs text-muted">{entry.note}</span>}
    </div>
  );
}
