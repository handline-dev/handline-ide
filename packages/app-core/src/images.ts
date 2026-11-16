// What an app does with a picture before it is uploaded and after it is chosen, with no platform in it: the size to shrink to, and the line a
// text carries for an image already uploaded.

/** The Markdown line that names an uploaded image in a task body or a memo. */
export const imageRef = (id: string, alt = '') => `![${alt.replace(/[[\]\n]/g, ' ').trim()}](attachment:${id})`;

/** A note, then one reference per image, each on its own line. */
export const withImages = (note: string, ids: readonly string[], alt = 'capture'): string =>
  [note.trim(), ...ids.map((id) => imageRef(id, alt))].filter(Boolean).join('\n');

/** An image reference in a text: where it sits, the caption and the attachment id. */
export interface ImageSpan {
  start: number;
  end: number;
  caption: string;
  id: string;
}

/** What an attachment id is made of. */
const ID = '[A-Za-z0-9_-]+';

/** An image link's attachment id (`attachment:ID`), or null for any other link. */
export const attachmentOf = (href: string): string | null => new RegExp(`^attachment:(${ID})$`).exec(href)?.[1] ?? null;

/** The `![caption](attachment:ID)` references in a text, in order. */
export function findImageRefs(text: string): ImageSpan[] {
  const out: ImageSpan[] = [];
  for (const m of text.matchAll(new RegExp(`!\\[([^\\]\\n]*)\\]\\(attachment:(${ID})\\)`, 'g'))) {
    out.push({ start: m.index, end: m.index + m[0].length, caption: m[1]!, id: m[2]! });
  }
  return out;
}
