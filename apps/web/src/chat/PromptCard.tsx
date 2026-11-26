import { CommandText } from '#src/ui/CommandText.tsx';
import { checkedKeys } from '@handline/app-core';
import { useAgentDetail, useNameOf, usePromptAnswer } from '@handline/app-data';
import { ruleText } from '#src/settings/AllowRules.tsx';
import type { Prompt } from '@handline/client';
import { AnimatePresence, motion } from 'framer-motion';
import { MessageCircleQuestion, ShieldAlert } from 'lucide-react';
import { useEffect, useState } from 'react';
import { spring } from '#src/motion.ts';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';
import { Refs } from '#src/ui/refs.tsx';

/** At the end of a chat whose agent waits on a prompt: answer it here, Allow or Deny, or a question's choices. */
export function PromptCard({ agent }: { agent: string }) {
  const prompt = useAgentDetail(agent).data?.prompt;
  // Answered, the card folds away by its height rather than vanishing.
  return (
    <AnimatePresence initial={false}>
      {prompt && (
        <motion.div
          key={prompt.id}
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: 'auto', opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={spring}
          className="overflow-hidden"
        >
          <Card agent={agent} p={prompt} />
        </motion.div>
      )}
    </AnimatePresence>
  );
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
  const nameOf = useNameOf();
  const [whole, setWhole] = useState(false);
  // The clicked button spins while the write runs and a few seconds after, the time the prompt takes to leave; a prompt still there
  // after that is answerable again.
  const [sent, setSent] = useState<string | null>(null);
  useEffect(() => {
    if (!sent) return;
    const t = setTimeout(() => setSent(null), 4000);
    return () => clearTimeout(t);
  }, [sent]);
  const busy = (key: string) => (a.answering && a.clicked === key) || sent === key;
  const locked = a.answering || !!sent;
  const send: typeof a.send = (choice, mark) => {
    setSent(mark);
    a.send(choice, mark);
  };
  // A repeat of a command asked before word for word may be allowed for good: the button names the agent and the exact command.
  const always = permission && a.opts.some((o) => !!o.rule);
  const label = (o: (typeof a.opts)[number]) => {
    if (!permission) return o.label;
    if (o.rule) return `Always allow ${nameOf(agent)} ${ruleText(o.rule)}`;
    if (o.answer === 'yes') return always ? 'Allow once' : 'Allow';
    if (o.answer === 'no') return 'Deny';
    return o.label;
  };
  return (
    <div className={`mt-2 flex max-w-150 flex-col gap-3 rounded-lg border border-border p-3 text-sm `}>
      <div className="flex gap-2.5">
        {permission ? (
          <ShieldAlert size={16} className="mt-0.5 shrink-0 text-warning" />
        ) : (
          <MessageCircleQuestion size={16} className="mt-0.5 shrink-0 text-warning" />
        )}
        <div className="flex min-w-0 flex-col gap-1">
          <p className="text-fg">{title}</p>
          {p.command && (
            // Cut to one line; a click shows it whole, wrapped, and folds it again.
            <button
              type="button"
              aria-expanded={whole}
              title={whole ? undefined : 'Show the whole command'}
              onClick={() => setWhole(!whole)}
              className={`block rounded bg-surface px-1.5 py-0.5 text-left font-mono text-xs text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent ${whole ? 'break-all whitespace-pre-wrap' : 'truncate'}`}
            >
              {p.command}
            </button>
          )}
          {p.reason && (
            <p className="text-muted">
              <Refs text={p.reason} />
            </p>
          )}
          {a.changed && <p className="text-xs text-danger">The prompt changed, so nothing was sent.</p>}
        </div>
      </div>
      {a.opts.length === 0 ? (
        // A prompt read from the screen before its choices are parsed: its panel, to answer in the terminal.
        <div className="ml-6">
          <CommandText text={p.lines.join('\n')} />
        </div>
      ) : a.multi ? (
        <div className="flex flex-col gap-1.5 pl-6">
          {a.opts.map((o) => (
            <label key={o.key} className="flex items-center gap-2">
              <input type="checkbox" checked={a.picked.includes(o.key)} onChange={() => a.toggle(o.key)} />
              {o.label}
            </label>
          ))}
          <div className="flex gap-2 pt-1">
            <Button
              variant="primary"
              disabled={!a.picked.length || locked}
              busy={busy('submit')}
              onClick={() => send(checkedKeys(a.opts, a.picked), 'submit')}
            >
              Submit
            </Button>
            {a.cancel && (
              <Button disabled={locked} busy={busy(a.cancel.key)} onClick={() => send(a.cancel!.key, a.cancel!.key)}>
                Cancel
              </Button>
            )}
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2 pl-6">
          {a.opts.map((o) => (
            <Button
              key={o.key}
              variant={(o.answer === 'yes' && !o.rule) || (!permission && o.highlighted) ? 'primary' : 'secondary'}
              busy={busy(o.key)}
              disabled={locked}
              onClick={() => send(o.key, o.key)}
            >
              {label(o)}
            </Button>
          ))}
          {a.question && !a.typing && (
            <Button variant="ghost" onClick={() => a.setTyping(true)}>
              Other…
            </Button>
          )}
          {a.cancel && (
            <Button variant="ghost" disabled={locked} busy={busy(a.cancel.key)} onClick={() => send(a.cancel!.key, a.cancel!.key)}>
              Cancel
            </Button>
          )}
        </div>
      )}
      {a.typing && (
        <form
          className="flex gap-2 pl-6"
          onSubmit={(e) => {
            e.preventDefault();
            if (a.text.trim()) send({ text: a.text.trim() }, 'text');
          }}
        >
          <Input autoFocus value={a.text} onChange={(e) => a.setText(e.target.value)} placeholder="Your answer" />
          <Button type="submit" variant="primary" disabled={!a.text.trim() || locked} busy={busy('text')}>
            Send
          </Button>
        </form>
      )}
    </div>
  );
}
