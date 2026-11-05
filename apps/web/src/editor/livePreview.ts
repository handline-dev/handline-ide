// Markdown drawn as a document while it stays Markdown: headings sized, emphasis set, and in the rich look the marks (#, **, `, >)
// hidden on every line the cursor is not on, as Obsidian's live preview does. The text itself is never changed, so a file edited
// here differs from before only by what was typed.
import { syntaxTree } from '@codemirror/language';
import { RangeSetBuilder, StateEffect, StateField, type EditorState, type Extension } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';
import { Decoration, ViewPlugin, type DecorationSet, type ViewUpdate } from '@codemirror/view';

/** Switches the rich look on or off: marks hidden off the cursor's lines, or always shown. */
export const setRich = StateEffect.define<boolean>();
export const richField = StateField.define<boolean>({
  create: () => false,
  update: (v, tr) => tr.effects.reduce((r, e) => (e.is(setRich) ? e.value : r), v),
});

const HIDDEN = new Set(['HeaderMark', 'EmphasisMark', 'CodeMark', 'StrikethroughMark', 'QuoteMark', 'LinkMark', 'URL']);
const marks: Record<string, Decoration> = {
  StrongEmphasis: Decoration.mark({ class: 'cm-md-strong' }),
  Emphasis: Decoration.mark({ class: 'cm-md-em' }),
  Strikethrough: Decoration.mark({ class: 'cm-md-strike' }),
  InlineCode: Decoration.mark({ class: 'cm-md-code' }),
  Link: Decoration.mark({ class: 'cm-md-link' }),
  ListMark: Decoration.mark({ class: 'cm-md-listmark' }),
};
const lines: Record<string, Decoration> = {
  ATXHeading1: Decoration.line({ class: 'cm-md-h1' }),
  ATXHeading2: Decoration.line({ class: 'cm-md-h2' }),
  ATXHeading3: Decoration.line({ class: 'cm-md-h3' }),
  ATXHeading4: Decoration.line({ class: 'cm-md-h4' }),
  Blockquote: Decoration.line({ class: 'cm-md-quote' }),
  FencedCode: Decoration.line({ class: 'cm-md-fence' }),
};
const hide = Decoration.replace({});

/** The lines any selection range touches: their marks stay visible, so what is being edited reads as typed. */
function activeLines(state: EditorState): Set<number> {
  const s = new Set<number>();
  for (const r of state.selection.ranges) {
    const a = state.doc.lineAt(r.from).number;
    const b = state.doc.lineAt(r.to).number;
    for (let n = a; n <= b; n++) s.add(n);
  }
  return s;
}

function build(view: EditorView): DecorationSet {
  const { state } = view;
  const rich = state.field(richField);
  const active = rich ? activeLines(state) : new Set<number>();
  const found: { from: number; to: number; deco: Decoration; line?: true }[] = [];
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(state).iterate({
      from,
      to,
      enter: (n) => {
        const ln = lines[n.name];
        if (ln) {
          // A block's style goes on each of its lines.
          for (let p = n.from; p <= n.to;) {
            const l = state.doc.lineAt(p);
            found.push({ from: l.from, to: l.from, deco: ln, line: true });
            p = l.to + 1;
          }
        }
        const mk = marks[n.name];
        if (mk && n.to > n.from) found.push({ from: n.from, to: n.to, deco: mk });
        if (rich && HIDDEN.has(n.name) && !active.has(state.doc.lineAt(n.from).number)) {
          // A heading's # takes the space after it along; a link keeps its text and hides its (url).
          let end = n.to;
          if ((n.name === 'HeaderMark' || n.name === 'QuoteMark') && state.doc.sliceString(end, end + 1) === ' ') end++;
          if (end > n.from) found.push({ from: n.from, to: end, deco: hide });
        }
      },
    });
  }
  // Line decorations first at a position, then marks, each in document order, as RangeSetBuilder wants.
  found.sort((a, b) => a.from - b.from || (a.line ? -1 : 0) - (b.line ? -1 : 0) || a.to - b.to);
  const out = new RangeSetBuilder<Decoration>();
  let last = -1;
  for (const f of found) {
    if (f.deco === hide && f.from < last) continue; // overlapping hides (a URL inside a hidden link)
    out.add(f.from, f.to, f.deco);
    if (f.deco === hide) last = f.to;
  }
  return out.finish();
}

const plugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = build(view);
    }
    update(u: ViewUpdate) {
      const switched = u.transactions.some((t) => t.effects.some((e) => e.is(setRich)));
      if (u.docChanged || u.viewportChanged || u.selectionSet || switched || syntaxTree(u.startState) !== syntaxTree(u.state))
        this.decorations = build(u.view);
    }
  },
  { decorations: (p) => p.decorations },
);

export const livePreview = (): Extension => [richField, plugin];
