// The palette's place in the chrome: the sidebar's first row, with its shortcut, so search is found without knowing the key.
import { Search } from 'lucide-react';
import { PALETTE_KEYS } from '#src/palette/Palette.tsx';
import { openPalette } from '#src/palette/store.ts';

/** Shaped as the sidebar's page rows (sidebar/Row); it opens no tab, so it is a plain button. */
export function SearchRow() {
  return (
    <button
      type="button"
      title={PALETTE_KEYS ? `Search (${PALETTE_KEYS})` : 'Search'}
      onClick={openPalette}
      className="flex h-7 cursor-default items-center gap-1.5 rounded-md px-2 text-left text-base text-fg outline-none select-none hover:bg-hover focus-visible:bg-hover"
    >
      <span className="grid size-4 shrink-0 place-items-center text-muted">
        <Search size={14} />
      </span>
      <span className="flex-1 truncate">Search</span>
      {PALETTE_KEYS && <kbd className="shrink-0 font-sans text-xs text-muted">{PALETTE_KEYS}</kbd>}
    </button>
  );
}
