// The pull request numbers (`#42`) in a text: a #N is a link only when the chat agent's repo has a pull request of that number.

// Not part of a word, an entity (`&#38;`) or a path: the char before must be a boundary.
const REF = /(?<![\w&/#.-])#(\d{1,9})\b/g;

/** The `#N` numbers of a text, once each and at most `max` (what pr.exists takes). */
export function hashNumbers(text: string, max = 20): number[] {
  const ids = [...text.matchAll(REF)].map((m) => Number(m[1]));
  return [...new Set(ids)].filter((n) => n > 0).slice(0, max);
}
