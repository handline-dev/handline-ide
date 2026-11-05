import { CopyButton } from '#src/chat/MessageActions.tsx';
import { marked, type Token, type Tokens } from 'marked';
import { Fragment, type ReactNode } from 'react';
import { useOpenTab, howOf } from '#src/layout/openTab.tsx';
import { fileOf, taskOf, webOf } from '#src/layout/kinds.tsx';

/** Markdown as the chat and notes show it: marked reads it (GFM), and each token is drawn as a React element, so no HTML in the
 * text is ever run. Lists keep the numbers as written (1. 2. or 1) 2)), not renumbered. In text, a task (T1234), a web address
 * and a repo path (apps/web/src/main.tsx:12) open in a tab: a click here, Cmd+click beside. */
export function Markdown({ text }: { text: string }) {
  const tokens = marked.lexer(text, { gfm: true, breaks: true });
  return <div className="flex flex-col gap-2 wrap-break-word">{blocks(tokens)}</div>;
}

const code = 'rounded-sm bg-surface px-1 font-mono text-sm';
const headings = ['text-lg', 'text-base', 'text-base', 'text-sm', 'text-sm', 'text-sm'];

function blocks(tokens: Token[]): ReactNode {
  return tokens.map((t, i) => <Fragment key={i}>{block(t)}</Fragment>);
}

function block(t: Token): ReactNode {
  switch (t.type) {
    case 'heading':
      return (
        <div role="heading" aria-level={t.depth} className={`${headings[t.depth - 1]} pt-1 font-semibold`}>
          {inline(t.tokens)}
        </div>
      );
    case 'paragraph':
      return <p className="whitespace-pre-wrap">{inline(t.tokens)}</p>;
    case 'list':
      return <List list={t as Tokens.List} />;
    case 'code':
      return (
        // A code block copies whole: its button at the top right, on hover.
        <div className="group/code relative">
          <pre className="rounded-md bg-surface p-2 pr-9 font-mono text-sm break-all whitespace-pre-wrap">
            <code>{t.text}</code>
          </pre>
          <span className="absolute top-1 right-1 opacity-0 transition-opacity group-hover/code:opacity-100 focus-within:opacity-100">
            <CopyButton text={t.text} />
          </span>
        </div>
      );
    case 'blockquote':
      return <blockquote className="flex flex-col gap-2 border-l-2 border-border pl-3 text-muted">{blocks(t.tokens ?? [])}</blockquote>;
    case 'hr':
      return <hr className="border-border" />;
    case 'table':
      return <Table table={t as Tokens.Table} />;
    case 'space':
      return null;
    default:
      return 'tokens' in t && t.tokens ? <p className="whitespace-pre-wrap">{inline(t.tokens)}</p> : <p>{t.raw}</p>;
  }
}

/** A list with each item's own marker as written: `2)` stays `2)`, a list starting at 3 starts at 3. */
function List({ list }: { list: Tokens.List }) {
  return (
    <div className="flex flex-col gap-0.5">
      {list.items.map((item, i) => {
        const marker = /^\s*(\d+[.)]|[-*+])\s/.exec(item.raw)?.[1] ?? (list.ordered ? `${i + 1}.` : '-');
        const bullet = /^[-*+]$/.test(marker);
        return (
          <div key={i} className="flex gap-2">
            <span className={`shrink-0 text-right tabular-nums ${bullet ? 'w-3 text-muted' : 'min-w-5 text-muted'}`}>
              {item.task ? (item.checked ? '☑' : '☐') : bullet ? '•' : marker}
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              {item.tokens.map((c, j) => (
                <Fragment key={j}>{c.type === 'text' && 'tokens' in c && c.tokens ? <span>{inline(c.tokens)}</span> : block(c)}</Fragment>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function Table({ table }: { table: Tokens.Table }) {
  return (
    <div className="overflow-x-auto">
      <table className="text-sm">
        <thead>
          <tr>
            {table.header.map((h, i) => (
              <th key={i} className="border-b border-border px-2 py-1 text-left font-semibold">
                {inline(h.tokens)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {table.rows.map((r, i) => (
            <tr key={i}>
              {r.map((c, j) => (
                <td key={j} className="border-b border-border px-2 py-1">
                  {inline(c.tokens)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function inline(tokens: Token[] | undefined): ReactNode {
  return (tokens ?? []).map((t, i) => <Fragment key={i}>{span(t)}</Fragment>);
}

function span(t: Token): ReactNode {
  switch (t.type) {
    case 'strong':
      return <strong className="font-semibold">{inline(t.tokens)}</strong>;
    case 'em':
      return <em>{inline(t.tokens)}</em>;
    case 'del':
      return <del className="text-muted">{inline(t.tokens)}</del>;
    case 'codespan': {
      // A path or a task written as code still opens.
      const target = linkOf(t.text);
      const c = <code className={code}>{t.text}</code>;
      return target ? <Opens opens={target}>{c}</Opens> : c;
    }
    case 'br':
      return <br />;
    case 'link':
      // Its words as plain text: a link inside a link would open twice.
      return /^https?:\/\//i.test(t.href) ? <Opens opens={webOf(t.href)}>{t.text}</Opens> : t.text;
    case 'text':
      return 'tokens' in t && t.tokens ? inline(t.tokens) : <Linked text={t.text} />;
    case 'escape':
      return t.text;
    default:
      // HTML and anything unknown is shown as the text it is, never run.
      return <Linked text={t.raw} />;
  }
}

// A task (T1234), a web address, or a repo path with a / and an extension (a :line after it is kept on the link).
const LINKS = /(\bT\d{2,}\b|https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"]|(?<![\w/.-])(?:[\w.-]+\/)+[\w.-]+\.[A-Za-z][\w]{0,7}(?::\d+)?)/g;

function linkOf(s: string): string | null {
  if (/^T\d{2,}$/.test(s)) return taskOf(Number(s.slice(1)));
  if (/^https?:\/\//i.test(s)) return webOf(s);
  if (/^(?:[\w.-]+\/)+[\w.-]+\.[A-Za-z]\w{0,7}(?::\d+)?$/.test(s)) return fileOf(s.replace(/:\d+$/, ''));
  return null;
}

function Linked({ text }: { text: string }) {
  const parts = text.split(LINKS);
  return parts.map((p, i) => {
    const target = i % 2 ? linkOf(p) : null;
    return target ? (
      <Opens key={i} opens={target}>
        {p}
      </Opens>
    ) : (
      p
    );
  });
}

/** Text that opens a tab the app's way: a click here, Cmd+click (or a middle click) beside. */
function Opens({ opens, children }: { opens: string; children: ReactNode }) {
  const open = useOpenTab();
  return (
    <a
      href={opens.startsWith('web:') ? opens.slice(4) : undefined}
      role="link"
      tabIndex={0}
      onClick={(e) => {
        e.preventDefault();
        open(opens, howOf(e));
      }}
      onAuxClick={(e) => {
        if (e.button !== 1) return;
        e.preventDefault();
        open(opens, 'side');
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') open(opens, howOf(e));
      }}
      className="cursor-pointer text-accent hover:underline"
    >
      {children}
    </a>
  );
}
