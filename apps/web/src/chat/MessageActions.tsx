import { Check, Copy, Quote } from 'lucide-react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useCopy } from '#src/ui/copy.ts';

/** A line's hover buttons: copy it, or quote it into the draft. */
export function Actions({ text, onQuote }: { text: string; onQuote: (text: string) => void }) {
  return (
    <span className="flex gap-0.5">
      <CopyButton text={text} />
      <IconButton className="size-6!" label="Reference" onClick={() => onQuote(text)}>
        <Quote size={14} />
      </IconButton>
    </span>
  );
}

/** Copies `text`; a check stands in for the icon for a moment to show it took. */
export function CopyButton({ text }: { text: string }) {
  const { copied: done, copy } = useCopy();
  return (
    <IconButton className="size-6!" label={done ? 'Copied' : 'Copy'} onClick={() => copy(text, 'the message')}>
      {done ? <Check size={14} className="text-success" /> : <Copy size={14} />}
    </IconButton>
  );
}
