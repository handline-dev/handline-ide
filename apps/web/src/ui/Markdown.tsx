import { CopyButton } from '#src/chat/MessageActions.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { attachmentOf, BANG_FROM, bangOf, commandOf } from '@handline/app-core';
import { CommandRunBar, CommandsOf, type CommandsOfValue } from '#src/chat/CommandBlock.tsx';
import { marked, type Token, type Tokens } from 'marked';
import { createContext, Fragment, useContext, type ReactNode } from 'react';
import { useOpenable } from '#src/layout/openable.tsx';
import { fileOf, taskOf, webOf } from '#src/layout/kinds.tsx';

/** Markdown as the chat and notes show it: marked reads it (GFM), and each token is drawn as a React element, so no HTML in the
 * text is ever run. Lists keep the numbers as written (1. 2. or 1) 2)), not renumbered. In text, a task (T1234), a web address
 * and a repo path (apps/web/src/main.tsx:12) open in a tab: a click here, Cmd+click beside. */
export function Markdown({ text }: { text: string }) {
  const tokens = marked.lexer(text, { gfm: true, breaks: true });
  // In a chat line, every code fence is numbered in order (app-core commandOf), and each inline `! ` span from BANG_FROM: a command's
  // run is named by that number.
  const of = useContext(CommandsOf);
  const fences = new Map<Token, { n: number; command: string | null }>();
  if (of) {
    // Link text is drawn as plain words, so a span in it is no command and takes no number.
    const inLinks = new Set<Token>();
    void marked.walkTokens(tokens, (t) => {
      if (t.type === 'link') void marked.walkTokens((t as Tokens.Link).tokens, (x) => void inLinks.add(x));
    });
    let code = 0;
    let bangs = 0;
    void marked.walkTokens(tokens, (t) => {
      if (t.type === 'code') fences.set(t, { n: code++, command: commandOf(t.lang ?? '', t.text) });
      else if (t.type === 'codespan' && !inLinks.has(t)) {
        const command = bangOf(t.text);
        if (command) fences.set(t, { n: BANG_FROM + bangs++, command });
      }
    });
  }
  return (
    <Fences.Provider value={of && { of, n: fences }}>
      <div className="flex flex-col gap-2 wrap-break-word">{blocks(tokens)}</div>
    </Fences.Provider>
  );
}

const Fences = createContext<{ of: NonNullable<CommandsOfValue>; n: ReadonlyMap<Token, { n: number; command: string | null }> } | null>(
  null,
);

/** A code block copies whole: its button at the top right, on hover. A command fence in a chat line also runs (chat/CommandBlock). */
function Code({ t }: { t: Tokens.Code }) {
  const fences = useContext(Fences);
  const f = fences?.n.get(t);
  return (
    <div className="group/code relative rounded-md bg-surface">
      <pre className="p-2 pr-9 font-mono text-sm break-all whitespace-pre-wrap">
        <code>{t.text}</code>
      </pre>
      <span className="absolute top-1 right-1 opacity-0 transition-opacity group-hover/code:opacity-100 focus-within:opacity-100">
        <CopyButton text={t.text} />
      </span>
      {fences && f?.command && <CommandRunBar of={fences.of} command={f.command} n={f.n} />}
    </div>
  );
}

const code = 'rounded-sm bg-surface px-1 font-mono text-sm';

/** Inline code: a path or a task written as code still opens; an agent's `! <command>` in a chat line runs (chat/CommandBlock). */
function CodeSpan({ t }: { t: Tokens.Codespan }) {
  const fences = useContext(Fences);
  const f = fences?.n.get(t);
  const target = linkOf(t.text);
  const c = <code className={code}>{t.text}</code>;
  if (fences && f?.command)
    return (
      <>
        {c}
        <CommandRunBar inline of={fences.of} command={f.command} n={f.n} />
      </>
    );
  return target ? <Opens opens={target}>{c}</Opens> : c;
}
const headings = ['text-lg', 'text-base', 'text-base', 'text-sm', 'text-sm', 'text-sm'];

function blocks(tokens: Token[]): ReactNode {
  return joinPictures(tokens).map((t, i) => <Fragment key={i}>{block(t)}</Fragment>);
}

const isPicture = (t: Token): boolean => t.type === 'image' && attachmentOf(t.href) !== null;
const isGap = (t: Token): boolean => t.type === 'br' || !t.raw.trim();
/** A paragraph of nothing but pasted pictures. */
const picturesOnly = (t: Token): boolean =>
  t.type === 'paragraph' && (t.tokens ?? []).some(isPicture) && (t.tokens ?? []).every((x) => isPicture(x) || isGap(x));

/** A paragraph whose last thing is a pasted picture: more pictures after it join its row. */
const endsInPictures = (t: Token): boolean => {
  if (t.type !== 'paragraph') return false;
  const shown = (t.tokens ?? []).filter((x) => !isGap(x));
  return shown.length > 0 && isPicture(shown.at(-1)!);
};

/** Pictures pasted on lines of their own after other pictures join the paragraph before: one row that wraps, not one row each. */
function joinPictures(tokens: Token[]): Token[] {
  const out: Token[] = [];
  for (const t of tokens) {
    let at = out.length - 1;
    while (at >= 0 && out[at]!.type === 'space') at--;
    const last = out[at];
    if (picturesOnly(t) && last && endsInPictures(last)) {
      out.splice(at, out.length - at, {
        ...last,
        tokens: [...(last as Tokens.Paragraph).tokens, ...(t as Tokens.Paragraph).tokens],
      } as Token);
    } else out.push(t);
  }
  return out;
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
      return <Paragraph tokens={t.tokens ?? []} />;
    case 'list':
      return <List list={t as Tokens.List} />;
    case 'code':
      return <Code t={t as Tokens.Code} />;
    case 'blockquote':
      return <blockquote className="flex flex-col gap-2 border-l-2 border-border pl-3 text-muted">{blocks(t.tokens ?? [])}</blockquote>;
    case 'hr':
      return <hr className="m-0 w-full border-border" />;
    case 'table':
      return <Table table={t as Tokens.Table} />;
    case 'space':
      return null;
    default:
      return 'tokens' in t && t.tokens ? <p className="whitespace-pre-wrap">{inline(t.tokens)}</p> : <p>{t.raw}</p>;
  }
}

/** A paragraph's words, then the pictures pasted into it (`![caption](attachment:id)`) as one row of thumbnails sharing a viewer. */
function Paragraph({ tokens }: { tokens: Token[] }) {
  const pictures = tokens
    .filter(isPicture)
    .map((t) => ({ id: attachmentOf((t as Tokens.Image).href)!, name: (t as Tokens.Image).text || 'picture' }));
  const words = tokens.filter((t) => !isPicture(t));
  // The line breaks the pictures sat on go with them, or they draw as blank lines under the words.
  while (words.length && isGap(words.at(-1)!)) words.pop();
  const said = words.length > 0;
  return (
    <>
      {said && <p className="whitespace-pre-wrap">{inline(words)}</p>}
      <MessageImages images={pictures} />
    </>
  );
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
    case 'codespan':
      return <CodeSpan t={t as Tokens.Codespan} />;
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

export function Linked({ text }: { text: string }) {
  // One span, so it is one item in a flex row.
  return (
    <span>
      {text.split(LINKS).map((p, i) => {
        const target = i % 2 ? linkOf(p) : null;
        return target ? (
          <Opens key={i} opens={target}>
            {p}
          </Opens>
        ) : (
          p
        );
      })}
    </span>
  );
}

/** Text that opens a tab the app's way (openable.tsx). */
function Opens({ opens, children }: { opens: string; children: ReactNode }) {
  const open = useOpenable(opens);
  return (
    <a
      href={opens.startsWith('web:') ? opens.slice(4) : undefined}
      role="link"
      tabIndex={0}
      {...open}
      onClick={(e) => {
        // The href is for the browser's own link look and Copy; the app opens it.
        e.preventDefault();
        open.onClick(e);
      }}
      className="cursor-pointer text-accent hover:underline"
    >
      {children}
    </a>
  );
}
