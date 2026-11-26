// What a Web tab hands the agent's chat, in the desktop shell only: its page as it is on screen, to mark up; or one element of
// it, picked with a click in the page, which comes as the same picture with a box drawn on the element and the element's details
// (app-core element-pick) as the picture's note. An iframe in a browser can give neither.
import { elementBlock, elementMark, type Mark } from '@handline/app-core';
import { useEffect, useState } from 'react';
import { shell } from '#src/shell.ts';
import { toast } from '#src/ui/toast.tsx';

/** A picture of the page for the viewer: with a picked element, the box on it and the text that goes with it. */
export type PageShot = { url: string; seed?: Mark[]; note?: string };

const pngUrl = (png: string) => URL.createObjectURL(new Blob([Uint8Array.from(atob(png), (c) => c.charCodeAt(0))], { type: 'image/png' }));

/** The page's picture and picks. `picking` is true while the next click in the page picks an element; `pick` then stops it, as Esc does. */
export function usePageShot(page: { id(): number | null }) {
  const [shot, setShot] = useState<PageShot | null>(null);
  /** The id of the page being picked from. */
  const [picking, setPicking] = useState<number | null>(null);
  useEffect(() => () => void (shot && URL.revokeObjectURL(shot.url)), [shot]);
  // Esc in the app window does not reach the page, so the shell is told; leaving the tab stops the pick too.
  useEffect(() => {
    if (picking === null) return;
    const key = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.preventDefault();
      e.stopPropagation();
      void shell()?.cancelPick?.(picking);
    };
    addEventListener('keydown', key, true);
    return () => {
      removeEventListener('keydown', key, true);
      void shell()?.cancelPick?.(picking);
    };
  }, [picking]);

  const capture = async () => {
    const id = page.id();
    const r = id === null ? null : await shell()?.capturePreview?.(id);
    if (!r?.ok) return toast(`No screenshot: ${r?.message ?? 'the page is not showing'}`, { tone: 'warning' });
    setShot({ url: pngUrl(r.png) });
  };
  const pick = async () => {
    if (picking !== null) return void shell()?.cancelPick?.(picking);
    const id = page.id();
    if (id === null) return toast('No element: the page is not showing', { tone: 'warning' });
    setPicking(id);
    const r = await shell()?.pickElement?.(id);
    setPicking(null);
    if (!r || (!r.ok && r.code === 'cancelled')) return;
    if (!r.ok) return toast(`No element: ${r.message}`, { tone: 'warning' });
    const { ok: _ok, png, ...element } = r;
    const mark = elementMark(element);
    setShot({ url: pngUrl(png), seed: mark ? [mark] : undefined, note: elementBlock(element) });
  };
  return { shot, picking: picking !== null, capture, pick, close: () => setShot(null) };
}
