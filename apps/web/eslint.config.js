import { react, RELATIVE_CALL, test } from '@handline/config/eslint';

// Colors come only from the theme roles in src/styles.css. Tailwind no longer has a palette, so a
// palette class silently draws nothing; these rules turn that and every other way in into an error.
const colorLiteral = String.raw`(#[0-9a-fA-F]{3,8}\b|\b(rgba?|hsla?|oklch|oklab|lab|lch|color)\()`;
const paletteClass = String.raw`\b(bg|text|border|ring|outline|fill|stroke|from|via|to|divide|shadow|accent|caret|decoration|placeholder)-(\[|(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)\b)`;
// Any arbitrary value (w-[13px], text-[15px], rounded-[3px]) steps outside the tokens. State variants
// (data-[open]:, aria-[...]:) select, they set no value, so they stay allowed.
const arbitraryValue = String.raw`(^|\s|:)-?(?!(group-|peer-)?(data|aria|has|supports|not|in)-\[)[a-z][\w-]*-\[`;
const message = 'Use a design token from src/styles.css (color role, text-*, rounded-*) or a component in src/ui.';
// An element that opens a tab takes its click, drag and right-click from useOpenable (src/layout/openable.tsx), so every one of them
// opens the same ways. This catches the usual slip, openTab or startTabDrag called in an element's own handler.
const openable = 'Spread useOpenable(opens) from src/layout/openable.tsx on it instead.';
const onHandler = 'JSXAttribute[name.name=/^on(Click|AuxClick|KeyDown|DragStart|ContextMenu)$/]';
const openOutsideOpenable = [
  { selector: `${onHandler} CallExpression[callee.name='openTab']`, message: openable },
  { selector: `${onHandler} CallExpression[callee.name='startTabDrag']`, message: openable },
];
const noHardcodedColor = {
  files: ['src/**/*.{ts,tsx}'],
  rules: {
    'no-restricted-syntax': [
      'error',
      RELATIVE_CALL,
      ...openOutsideOpenable,
      ...[colorLiteral, paletteClass, arbitraryValue].flatMap((re) => [
        { selector: `Literal[value=/${re}/]`, message },
        { selector: `TemplateElement[value.raw=/${re}/]`, message },
      ]),
    ],
  },
};

// legacy/ is the old web, frozen until it is removed: it is not linted.
export default [{ ignores: ['legacy/**'] }, ...react(import.meta.dirname), noHardcodedColor, ...test];
