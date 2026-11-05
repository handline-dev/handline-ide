import { Check, Copy, Quote } from 'lucide-react';
import { useState } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';

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
function CopyButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <IconButton
      className="size-6!"
      label={done ? 'Copied' : 'Copy'}
      onClick={() => {
        navigator.clipboard.writeText(text).then(
          () => {
            setDone(true);
            setTimeout(() => setDone(false), 1500);
          },
          () => toast('Copy did not work.', { tone: 'warning' }),
        );
      }}
    >
      {done ? <Check size={14} className="text-success" /> : <Copy size={14} />}
    </IconButton>
  );
}
