import { useEffect } from 'react';

/** Open full-screen layers, top last. The shell's drag area ignores z-index, so body[data-layer] makes the page's strips no-drag under a layer;
 * a layer that drags draws its own `<LayerDragStrip />`. */
type Layer = { drag: boolean };
let layers: Layer[] = [];
const set = (next: Layer[]) => {
  layers = next;
  const top = layers[layers.length - 1];
  if (top) document.body.dataset.layer = top.drag ? 'drag' : 'still';
  else delete document.body.dataset.layer;
};

/** While `open`, this component is a full-screen layer. */
export function useWindowLayer(open: boolean, { drag = false }: { drag?: boolean } = {}) {
  useEffect(() => {
    if (!open) return;
    const me = { drag };
    set([...layers, me]);
    return () => set(layers.filter((l) => l !== me));
  }, [open, drag]);
}

/** The top of a layer that moves the window: where the page's own strip is. Put it first in the layer. */
export function LayerDragStrip() {
  return <div className="app-drag layer-drag shrink-0" style={{ height: 'max(40px, calc(var(--controls-top, 6px) + 34px))' }} />;
}
