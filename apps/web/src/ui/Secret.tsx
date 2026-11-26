// A secret shown in one rounded box, as a password field is: blurred until the eye at its right is pressed; resting on it says a
// click copies it, and a click does. The one way the app shows a pairing code, a token or a key.
import { Check, Copy, Eye, EyeOff } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useCopy } from '#src/ui/copy.ts';
import { IconButton } from '#src/ui/IconButton.tsx';

export function Secret({ value, name, children }: { value: string; name: string; children: ReactNode }) {
  const [shown, setShown] = useState(false);
  const { copied, copy } = useCopy();
  return (
    <div className="group flex items-stretch rounded-md border border-border bg-surface">
      <button
        type="button"
        aria-label={`Copy ${name}`}
        onClick={() => copy(value, name)}
        className="relative flex min-w-0 flex-1 flex-col items-start gap-1 px-3 py-2 text-left"
      >
        <span className={`flex min-w-0 flex-col gap-1 ${shown ? '' : 'blur-xs select-none'}`}>{children}</span>
        <span
          className={`absolute top-2 right-2 flex items-center gap-1 rounded-sm bg-bg px-1.5 py-0.5 text-xs text-muted ${copied ? '' : 'invisible group-hover:visible'}`}
        >
          {copied ? <Check size={12} /> : <Copy size={12} />}
          {copied ? 'Copied' : 'Copy'}
        </span>
      </button>
      <IconButton label={shown ? `Hide ${name}` : `Show ${name}`} className="m-1 self-center" onClick={() => setShown((v) => !v)}>
        {shown ? <EyeOff size={14} /> : <Eye size={14} />}
      </IconButton>
    </div>
  );
}
