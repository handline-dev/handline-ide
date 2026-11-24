/** A number with its word, plural unless it is one: 1 file, 3 files. */
export const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;

/** `part` as a whole percent of `whole`, 0 when there is no whole. */
export const pct = (part: number, whole: number) => (whole > 0 ? Math.round((part / whole) * 100) : 0);
