// A unified diff (`git diff`, `git show -p`) as files of numbered rows. A line's numbers come from following each hunk header's counts
// (`@@ -a,b +c,d @@`), never from how the line starts: inside a hunk a deleted line whose text is `-- x` reads `--- x` and an added
// `++ x` reads `+++ x`, and both are content. Where the numbers cannot be trusted the file says so and no number is made up. Pure.

export type DiffRowKind = 'add' | 'del' | 'ctx' | 'hunk' | 'meta';

/** One line of a file's diff. `text` is the line without its +, - or space; a hunk row is its whole header; a meta row is a line that is
 *  not the file's content (`\ No newline at end of file`, `Binary files … differ`, a note in place of a diff). */
export interface DiffRow {
  kind: DiffRowKind;
  /** Its line number before the change: a deleted or unchanged line. */
  oldNo?: number;
  /** Its line number after the change: an added or unchanged line. */
  newNo?: number;
  text: string;
}

export interface DiffFile {
  /** The path after the change; of a deleted file, the path it had. */
  path: string;
  /** The path before a rename. */
  oldPath?: string;
  status: 'added' | 'deleted' | 'renamed' | 'modified';
  binary: boolean;
  rows: DiffRow[];
  /** How many `diff` sections of the text are this path's: more than one when it is several commits' patches joined. */
  sections: number;
  /** The text ends inside one of its hunks, or was cut somewhere in this file. */
  cut: boolean;
  /** A merge's combined diff (`diff --cc`): its lines carry one sign per parent and are not numbered here. */
  combined: boolean;
}

/** Whether the file's rows carry line numbers that can be pointed at: one section, whole, not a merge's. */
export const lineNumbersHold = (f: DiffFile): boolean => f.sections === 1 && !f.cut && !f.combined;

const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;
const SIMPLE: Record<string, number> = { a: 7, b: 8, t: 9, n: 10, v: 11, f: 12, r: 13, '"': 34, '\\': 92 };
const pct = (byte: number) => `%${byte.toString(16).padStart(2, '0')}`;

/** A name git wrote in double quotes, as it is: `\303\257` and the like are the bytes of its UTF-8. */
export function unquotePath(quoted: string): string {
  if (!quoted.startsWith('"') || !quoted.endsWith('"') || quoted.length < 2) return quoted;
  const body = quoted.slice(1, -1);
  let out = '';
  for (let i = 0; i < body.length; i++) {
    if (body[i] !== '\\') {
      out += body[i] === '%' ? '%25' : body[i];
      continue;
    }
    const octal = /^[0-3][0-7]{2}/.exec(body.slice(i + 1));
    if (octal) {
      out += pct(parseInt(octal[0], 8));
      i += 3;
    } else if (body[i + 1] in SIMPLE) out += pct(SIMPLE[body[++i]]);
  }
  try {
    return decodeURIComponent(out);
  } catch {
    return body;
  }
}

/** Where the quoted name starting at `from` ends. */
function closing(s: string, from: number): number {
  for (let i = from + 1; i < s.length; i++) {
    if (s[i] === '\\') i++;
    else if (s[i] === '"') return i;
  }
  return -1;
}

/** The two names of a `diff --git` line, their a/ and b/ still on. Two plain names are told apart where both halves are the same name;
 *  a rename with a space in a name is settled by its `rename` lines. */
function twoNames(rest: string): [string, string] | null {
  if (rest.startsWith('"')) {
    const end = closing(rest, 0);
    return end < 0 ? null : [unquotePath(rest.slice(0, end + 1)), unquotePath(rest.slice(end + 2))];
  }
  const quoted = rest.endsWith('"') ? rest.indexOf(' "') : -1;
  if (quoted > 0) return [rest.slice(0, quoted), unquotePath(rest.slice(quoted + 1))];
  const half = (rest.length - 1) / 2;
  if (Number.isInteger(half) && rest[half] === ' ' && rest.slice(2, half) === rest.slice(half + 3))
    return [rest.slice(0, half), rest.slice(half + 1)];
  const m = /^(.+?) (b\/.+)$/.exec(rest);
  return m ? [m[1], m[2]] : null;
}

/** The name on a `---` or `+++` line, without its a/ or b/; null for /dev/null. A name with a space is followed by a tab. */
function sideName(raw: string): string | null {
  const name = unquotePath(raw.replace(/\t$/, ''));
  return name === '/dev/null' ? null : name.replace(/^[ab]\//, '');
}

const blank = (path: string, combined: boolean): DiffFile => ({
  path,
  status: 'modified',
  binary: false,
  rows: [],
  sections: 1,
  cut: false,
  combined,
});

/** The file a `diff` line starts; undefined for any other line. */
function opened(line: string): DiffFile | undefined {
  const merge = /^diff --(?:cc|combined) (.+)$/.exec(line);
  if (merge) return blank(unquotePath(merge[1]), true);
  if (!line.startsWith('diff --git ')) return undefined;
  const names = twoNames(line.slice('diff --git '.length));
  const file = blank(names ? names[1].replace(/^b\//, '') : '', false);
  const before = names?.[0].replace(/^a\//, '');
  if (before !== undefined && before !== file.path) file.oldPath = before;
  return file;
}

const QUIET = /^(index |old mode |new mode |mode |similarity index |dissimilarity index |copy from |copy to )/;

/** Reads a line of the part before a file's first hunk into `f`; a line that says something the reader should see becomes a meta row. */
function header(f: DiffFile, line: string) {
  if (line === '' || QUIET.test(line)) return;
  if (line.startsWith('new file mode ')) f.status = 'added';
  else if (line.startsWith('deleted file mode ')) f.status = 'deleted';
  else if (line.startsWith('rename from ')) {
    f.oldPath = unquotePath(line.slice(12));
    f.status = 'renamed';
  } else if (line.startsWith('rename to ')) f.path = unquotePath(line.slice(10));
  else if (line.startsWith('--- ')) {
    const name = sideName(line.slice(4));
    if (name === null) f.status = 'added';
    else if (f.status !== 'renamed') f.path = name;
  } else if (line.startsWith('+++ ')) {
    const name = sideName(line.slice(4));
    if (name === null) f.status = 'deleted';
    else f.path = name;
  } else {
    f.binary ||= /^(Binary files |GIT binary patch)/.test(line);
    f.rows.push({ kind: 'meta', text: line });
  }
}

/** Where a hunk stands: the next line's numbers and how many lines of each side it still owes. */
interface Hunk {
  oldNo: number;
  newNo: number;
  oldLeft: number;
  newLeft: number;
}
const open = (h: Hunk) => h.oldLeft > 0 || h.newLeft > 0;

/** One line inside a hunk as its row, the hunk moved on; null when the line cannot be the hunk's (the text was cut or is not a diff). */
function content(h: Hunk, line: string): DiffRow | null {
  const sign = line[0];
  const text = line.slice(1);
  if (sign === '\\') return { kind: 'meta', text: line };
  if (sign === '+' && h.newLeft > 0) {
    h.newLeft--;
    return { kind: 'add', newNo: h.newNo++, text };
  }
  if (sign === '-' && h.oldLeft > 0) {
    h.oldLeft--;
    return { kind: 'del', oldNo: h.oldNo++, text };
  }
  // an unchanged empty line may have lost its one space on the way
  if ((sign === ' ' || line === '') && h.oldLeft > 0 && h.newLeft > 0) {
    h.oldLeft--;
    h.newLeft--;
    return { kind: 'ctx', oldNo: h.oldNo++, newNo: h.newNo++, text };
  }
  return null;
}

/** Each `diff` section of the text as a file, in the text's order. `truncated`: the text was cut at a size cap, so its last file is
 *  marked cut even where the cut fell between two lines. */
export function diffSections(diff: string, truncated = false): DiffFile[] {
  const files: DiffFile[] = [];
  let f: DiffFile | undefined;
  let h: Hunk = { oldNo: 0, newNo: 0, oldLeft: 0, newLeft: 0 };
  let headed = false; // past the file's header lines: a hunk was seen
  for (const line of (diff.endsWith('\n') ? diff.slice(0, -1) : diff).split('\n')) {
    if (f && open(h)) {
      const row = content(h, line);
      if (row) {
        f.rows.push(row);
        continue;
      }
      f.cut = true;
      h = { ...h, oldLeft: 0, newLeft: 0 };
    }
    const next = opened(line);
    const at = !next && f && !f.combined ? HUNK.exec(line) : null;
    if (next) {
      files.push((f = next));
      headed = false;
    } else if (at) {
      h = { oldNo: Number(at[1]), newNo: Number(at[3]), oldLeft: Number(at[2] ?? 1), newLeft: Number(at[4] ?? 1) };
      f!.rows.push({ kind: 'hunk', text: line });
      headed = true;
    } else if (f?.combined && line.startsWith('@@@')) {
      f.rows.push({ kind: 'hunk', text: line });
      headed = true;
    } else if (f && !headed) header(f, line);
    else if (f && line !== '') f.rows.push({ kind: 'meta', text: line });
  }
  if (f && (open(h) || truncated)) f.cut = true;
  return files;
}

const bare = (rows: DiffRow[]): DiffRow[] => rows.map(({ kind, text }) => ({ kind, text }));

/** The text's files, one per path in the order they first appear. A path with several sections keeps every section's rows, one after
 *  another and without their numbers: each section counted its lines from a different state of the file. */
export function diffFiles(diff: string, truncated = false): DiffFile[] {
  const by = new Map<string, DiffFile>();
  for (const s of diffSections(diff, truncated)) {
    const have = by.get(s.path);
    if (!have) {
      by.set(s.path, s);
      continue;
    }
    by.set(s.path, {
      ...have,
      rows: [...bare(have.rows), ...bare(s.rows)],
      sections: have.sections + 1,
      binary: have.binary || s.binary,
      cut: have.cut || s.cut,
      combined: have.combined || s.combined,
    });
  }
  return [...by.values()];
}
