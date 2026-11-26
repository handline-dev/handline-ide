// A marked picture's marks drawn into its pixels, as the message goes: numbered from `from`, so they run on across the message.
import { exportAnnotated, marksToShapes, type Mark } from '@handline/app-core';

/** The picture with the kept marks drawn in, as a PNG; with no marks, the picture as a PNG. */
export async function bake(source: Blob | HTMLImageElement, marks: readonly Mark[], from: number): Promise<Blob | null> {
  const img = source instanceof Blob ? await createImageBitmap(source) : source;
  const w = img instanceof HTMLImageElement ? img.naturalWidth : img.width;
  const h = img instanceof HTMLImageElement ? img.naturalHeight : img.height;
  const c = document.createElement('canvas');
  c.width = w;
  c.height = h;
  const g = c.getContext('2d');
  if (!g) {
    if (!(img instanceof HTMLImageElement)) img.close();
    return null;
  }
  g.drawImage(img, 0, 0);
  if (marks.length) {
    const src = g.getImageData(0, 0, w, h);
    const out = exportAnnotated(
      { width: w, height: h, data: src.data },
      marksToShapes(marks, Math.max(3, Math.round(w / 250)), false, from),
    );
    g.putImageData(new ImageData(out.data as Uint8ClampedArray<ArrayBuffer>, out.width, out.height), 0, 0);
  }
  if (!(img instanceof HTMLImageElement)) img.close();
  return new Promise((ok) => c.toBlob(ok, 'image/png'));
}
