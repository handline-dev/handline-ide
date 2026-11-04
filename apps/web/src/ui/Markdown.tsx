import type { ReactNode } from 'react';

/** A small Markdown reader: headings, bullet and numbered lists, paragraphs, fenced code, **bold** and `code`.
 * No HTML is read: every piece is a React text node or element. */

type Block =
  | { kind: 'code'; text: string }
  | { kind: 'heading'; level: number; text: string }
  | { kind: 'list'; ordered: boolean; items: string[] }
  | { kind: 'para'; text: string };

const FENCE = /^\s*```/;
const HEADING = /^(#{1,4})\s+(.+)$/;
const BULLET = /^\s*[-*]\s+(.*)$/;
const NUMBER = /^\s*\d+[.)]\s+(.*)$/;

function parse(text: string): Block[] {
  const blocks: Block[] = [];
  const lines = text.split('\n');
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ kind: 'para', text: para.join('\n') });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    if (FENCE.test(line)) {
      flush();
      const code: string[] = [];
      for (i++; i < lines.length && !FENCE.test(lines[i]!); i++) code.push(lines[i]!);
      blocks.push({ kind: 'code', text: code.join('\n') });
      continue;
    }
    const heading = HEADING.exec(line);
    const item = BULLET.exec(line) ?? NUMBER.exec(line);
    if (heading) {
      flush();
      blocks.push({ kind: 'heading', level: heading[1]!.length, text: heading[2]! });
    } else if (item) {
      flush();
      const ordered = !BULLET.test(line);
      const last = blocks.at(-1);
      if (last?.kind === 'list' && last.ordered === ordered) last.items.push(item[1]!);
      else blocks.push({ kind: 'list', ordered, items: [item[1]!] });
    } else if (!line.trim()) flush();
    else para.push(line);
  }
  flush();
  return blocks;
}

const INLINE = /(`[^`\n]+`|\*\*[^*\n]+\*\*)/;
const code = 'rounded-sm bg-surface px-1 font-mono';

function Inline({ text }: { text: string }): ReactNode {
  return text.split(INLINE).map((s, i) =>
    i % 2 === 0 ? (
      s
    ) : s.startsWith('`') ? (
      <code key={i} className={code}>
        {s.slice(1, -1)}
      </code>
    ) : (
      <strong key={i} className="font-semibold">
        <Inline text={s.slice(2, -2)} />
      </strong>
    ),
  );
}

const headings = ['text-lg', 'text-base', 'text-sm', 'text-sm'];

export function Markdown({ text }: { text: string }) {
  return (
    <div className="flex flex-col gap-2 wrap-break-word">
      {parse(text).map((b, i) =>
        b.kind === 'code' ? (
          <pre key={i} className={`${code} overflow-x-auto p-2 whitespace-pre`}>
            <code>{b.text}</code>
          </pre>
        ) : b.kind === 'heading' ? (
          <div key={i} role="heading" aria-level={b.level} className={`${headings[b.level - 1]} pt-1 font-semibold`}>
            <Inline text={b.text} />
          </div>
        ) : b.kind === 'list' ? (
          (() => {
            const List = b.ordered ? 'ol' : 'ul';
            return (
              <List key={i} className={`${b.ordered ? 'list-decimal' : 'list-disc'} pl-5`}>
                {b.items.map((t, j) => (
                  <li key={j}>
                    <Inline text={t} />
                  </li>
                ))}
              </List>
            );
          })()
        ) : (
          <p key={i} className="whitespace-pre-wrap">
            <Inline text={b.text} />
          </p>
        ),
      )}
    </div>
  );
}
