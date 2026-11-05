// The toolbar's formatting as edits of the Markdown text: a mark around the selection (**, *, ~~, `), toggled, or a prefix on each
// selected line (#, -, 1., >), toggled. They change only the characters of the mark, so they merge with others' edits like typing.
import { EditorSelection, type EditorState } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';

export function toggleWrap(view: EditorView, mark: string) {
  const { state } = view;
  const n = mark.length;
  view.dispatch(
    state.changeByRange((r) => {
      const before = state.sliceDoc(r.from - n, r.from);
      const after = state.sliceDoc(r.to, r.to + n);
      if (before === mark && after === mark)
        return {
          changes: [
            { from: r.from - n, to: r.from },
            { from: r.to, to: r.to + n },
          ],
          range: EditorSelection.range(r.from - n, r.to - n),
        };
      return {
        changes: [
          { from: r.from, insert: mark },
          { from: r.to, insert: mark },
        ],
        range: EditorSelection.range(r.from + n, r.to + n),
      };
    }),
    { scrollIntoView: true, userEvent: 'input.format' },
  );
  view.focus();
}

const PREFIX = /^(#{1,6} |[-*+] |\d+[.)] |> )/;

/** Sets each selected line's prefix to `prefix`, or clears it when every line has it already. */
export function toggleLinePrefix(view: EditorView, prefix: string) {
  const { state } = view;
  const ls = selectedLines(state);
  const all = ls.every((l) => (prefix === '1. ' ? /^\d+[.)] /.test(l.text) : l.text.startsWith(prefix)));
  let k = 0;
  const changes = ls.map((l) => {
    const had = PREFIX.exec(l.text)?.[0] ?? '';
    const next = all ? '' : prefix === '1. ' ? `${++k}. ` : prefix;
    return { from: l.from, to: l.from + had.length, insert: next };
  });
  view.dispatch({ changes, scrollIntoView: true, userEvent: 'input.format' });
  view.focus();
}

function selectedLines(state: EditorState) {
  const seen = new Map<number, { from: number; text: string }>();
  for (const r of state.selection.ranges)
    for (let p = r.from; p <= r.to;) {
      const l = state.doc.lineAt(p);
      seen.set(l.number, { from: l.from, text: l.text });
      p = l.to + 1;
    }
  return [...seen.entries()].sort((a, b) => a[0] - b[0]).map(([, l]) => l);
}

/** A fenced block around the selected lines, or an empty one at the cursor. */
export function insertCodeBlock(view: EditorView) {
  const { state } = view;
  const r = state.selection.main;
  const a = state.doc.lineAt(r.from);
  const b = state.doc.lineAt(r.to);
  view.dispatch({
    changes: [
      { from: a.from, insert: '```\n' },
      { from: b.to, insert: '\n```' },
    ],
    selection: EditorSelection.cursor(a.from + 4),
    userEvent: 'input.format',
  });
  view.focus();
}
