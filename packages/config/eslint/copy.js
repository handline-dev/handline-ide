// Text the apps show is a word or a short label, never a sentence: layout, icons and the button itself carry the rest (docs/copy.md).
// A string that ends with a period or runs past MAX_WORDS fails. Strings that were already sentences are listed in copy-baseline.json,
// which may only shrink: a listed string that no longer appears fails too, so it is removed when its screen is fixed.
// A line that really needs a sentence carries `copy-ok` in a comment.
import { readFileSync } from 'node:fs';
import path from 'node:path';

export const MAX_WORDS = 6;
const TEXT_PROPS = new Set(['title', 'label', 'placeholder', 'aria-label', 'alt', 'aria-description', 'description']);
const repo = path.resolve(import.meta.dirname, '../../..');

export const isSentence = (s) => {
  const t = s.replace(/\s+/g, ' ').trim();
  if (!/[A-Za-z]/.test(t)) return false;
  return /[^.]\.$/.test(t) || t.split(' ').length > MAX_WORDS;
};

/** Every string the node shows: [node, text]. */
export function shownStrings(node) {
  if (node.type === 'JSXText') return [[node, node.value]];
  if (node.type !== 'JSXAttribute' || !TEXT_PROPS.has(node.name.name) || !node.value) return [];
  const v = node.value.type === 'JSXExpressionContainer' ? node.value.expression : node.value;
  if (v.type === 'Literal' && typeof v.value === 'string') return [[v, v.value]];
  if (v.type === 'TemplateLiteral' && v.expressions.length === 0) return [[v, v.quasis[0].value.cooked ?? '']];
  return [];
}

const norm = (s) => s.replace(/\s+/g, ' ').trim();

export const copyRule = {
  meta: {
    type: 'problem',
    schema: [{ type: 'object', properties: { baseline: { type: 'object' } }, additionalProperties: false }],
    messages: {
      sentence: 'Screen text is a label, not a sentence ({{text}}): say it with the layout, an icon or the button (docs/copy.md).',
      stale: 'Not a sentence any more, or gone: remove "{{text}}" from packages/config/eslint/copy-baseline.json.',
    },
  },
  create(context) {
    const rel = path.relative(repo, context.filename).split(path.sep).join('/');
    const baseline = context.options[0]?.baseline ?? JSON.parse(readFileSync(path.join(import.meta.dirname, 'copy-baseline.json'), 'utf8'));
    const left = new Map();
    for (const s of baseline[rel] ?? []) left.set(s, (left.get(s) ?? 0) + 1);
    const check = (node) => {
      for (const [at, raw] of shownStrings(node)) {
        const text = norm(raw);
        if (!isSentence(text)) continue;
        const line = context.sourceCode.lines[at.loc.start.line - 1];
        if (line.includes('copy-ok')) continue;
        if ((left.get(text) ?? 0) > 0) {
          left.set(text, left.get(text) - 1);
          continue;
        }
        context.report({ node: at, messageId: 'sentence', data: { text: text.slice(0, 60) } });
      }
    };
    return {
      JSXText: check,
      JSXAttribute: check,
      'Program:exit'(node) {
        for (const [text, n] of left) if (n > 0) context.report({ node, messageId: 'stale', data: { text: text.slice(0, 60) } });
      },
    };
  },
};
