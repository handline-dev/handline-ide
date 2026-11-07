import { checkedKeys } from '@handline/app-core';
import { useAgentDetail, usePromptAnswer } from '@handline/app-data';
import type { Prompt } from '@handline/client';
import { MessageCircleQuestion, ShieldAlert } from 'lucide-react';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';

/** At the end of a chat whose agent waits on a prompt: answer it here, Allow or Deny, or a question's choices. */
export function PromptCard({ agent }: { agent: string }) {
  const prompt = useAgentDetail(agent).data?.prompt;
  if (!prompt) return null;
  return <Card agent={agent} p={prompt} />;
}

/** A question's own words: its panel's lines that are not one of its choices. */
const questionText = (p: Prompt) => {
  const labels = new Set(p.options.map((o) => o.label.trim()));
  return p.lines.map((l) => l.trim()).find((l) => l && !labels.has(l) && !/^\d+\.\s/.test(l)) ?? null;
};

function Card({ agent, p }: { agent: string; p: Prompt }) {
  const a = usePromptAnswer(agent, p, undefined);
  const permission = p.kind === 'permission';
  const title = permission ? `Allow ${p.tool ?? 'this'}?` : a.question ? (questionText(p) ?? 'Question') : 'The agent asks';
  const label = (o: (typeof a.opts)[number]) =>
    permission ? (o.answer === 'yes' ? 'Allow' : o.answer === 'no' ? 'Deny' : o.label) : o.label;
  return (
    <div
      className={`mt-2 flex max-w-150 flex-col gap-3 rounded-lg border border-border p-3 text-sm ${a.answering ? 'pointer-events-none opacity-60' : ''}`}
    >
      <div className="flex gap-2.5">
        {permission ? (
          <ShieldAlert size={16} className="mt-0.5 shrink-0 text-warning" />
        ) : (
          <MessageCircleQuestion size={16} className="mt-0.5 shrink-0 text-warning" />
        )}
        <div className="flex min-w-0 flex-col gap-1">
          <p className="text-fg">{title}</p>
          {p.command && <code className="block truncate rounded bg-surface px-1.5 py-0.5 font-mono text-xs text-fg">{p.command}</code>}
          {p.reason && <p className="text-muted">{p.reason}</p>}
          {a.changed && <p className="text-xs text-danger">The prompt changed, so nothing was sent.</p>}
        </div>
      </div>
      {a.opts.length === 0 ? (
        // A prompt read from the screen before its choices are parsed: its panel, to answer in the terminal.
        <pre className="ml-6 overflow-x-auto text-xs whitespace-pre text-muted">{p.lines.join('\n')}</pre>
      ) : a.multi ? (
        <div className="flex flex-col gap-1.5 pl-6">
          {a.opts.map((o) => (
            <label key={o.key} className="flex items-center gap-2">
              <input type="checkbox" checked={a.picked.includes(o.key)} onChange={() => a.toggle(o.key)} />
              {o.label}
            </label>
          ))}
          <div className="flex gap-2 pt-1">
            <Button variant="primary" disabled={!a.picked.length} onClick={() => a.send(checkedKeys(a.opts, a.picked), 'submit')}>
              {a.mark('submit', 'Submit')}
            </Button>
            {a.cancel && <Button onClick={() => a.send(a.cancel!.key, a.cancel!.key)}>{a.mark(a.cancel.key, 'Cancel')}</Button>}
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2 pl-6">
          {a.opts.map((o) => (
            <Button
              key={o.key}
              variant={o.answer === 'yes' || (!permission && o.highlighted) ? 'primary' : 'secondary'}
              onClick={() => a.send(o.key, o.key)}
            >
              {a.mark(o.key, label(o))}
            </Button>
          ))}
          {a.question && !a.typing && (
            <Button variant="ghost" onClick={() => a.setTyping(true)}>
              Other…
            </Button>
          )}
          {a.cancel && (
            <Button variant="ghost" onClick={() => a.send(a.cancel!.key, a.cancel!.key)}>
              {a.mark(a.cancel.key, 'Cancel')}
            </Button>
          )}
        </div>
      )}
      {a.typing && (
        <form
          className="flex gap-2 pl-6"
          onSubmit={(e) => {
            e.preventDefault();
            if (a.text.trim()) a.send({ text: a.text.trim() }, 'text');
          }}
        >
          <Input autoFocus value={a.text} onChange={(e) => a.setText(e.target.value)} placeholder="Your answer" />
          <Button type="submit" variant="primary" disabled={!a.text.trim()}>
            {a.mark('text', 'Send')}
          </Button>
        </form>
      )}
    </div>
  );
}
