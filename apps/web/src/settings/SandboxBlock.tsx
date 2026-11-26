// Settings › Agents › one agent: its sandbox switch and what the sandbox lets it do, one removable row per sandbox_allow entry.
import { useAgentDetail, useWrite, why } from '@handline/app-data';
import { FolderPen, Globe, ShieldCheck, SquareTerminal, X, type LucideIcon } from 'lucide-react';
import { day } from '#src/format/time.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

/** One line of sandbox_allow, `<day> <kind> <value>`, as the daemon keeps it. */
type Kind = 'host' | 'command' | 'write' | 'cert';
export type Allowance = { line: string; day: string; kind: Kind; value: string };

/** Each kind's icon and word; the certificate entry reads as a sentence and needs no word. */
const KIND: Record<Kind, { icon: LucideIcon; word: string }> = {
  host: { icon: Globe, word: 'May reach' },
  command: { icon: SquareTerminal, word: 'Runs outside the sandbox' },
  write: { icon: FolderPen, word: 'May write' },
  cert: { icon: ShieldCheck, word: '' },
};

/** The list's entries in order; a line that does not read as one is left out, as the daemon leaves it out. */
export function allowancesOf(text: string): Allowance[] {
  return text.split('\n').flatMap((l) => {
    const m = /^(\d{4}-\d{2}-\d{2}) (host|command|write|cert)(?: (.+))?$/.exec(l.trim());
    return m ? [{ line: l.trim(), day: m[1]!, kind: m[2] as Kind, value: m[3] ?? '' }] : [];
  });
}

/** The list with `line` taken out, one entry per line. */
export const without = (text: string, line: string) =>
  allowancesOf(text)
    .filter((a) => a.line !== line)
    .map((a) => a.line)
    .join('\n');

const words = (a: Allowance) => (a.kind === 'cert' ? 'Go tools may use the macOS certificate check' : a.value);

/** Its sandbox switch, then what the sandbox lets it do beyond package registries, each row's x removing it. */
export function Sandbox({ agent }: { agent: string }) {
  const settings = useAgentDetail(agent).data?.settings;
  const set = useWrite('agent.set', agent);
  if (!settings) return null;
  const on = settings.sandbox === 'on';
  const text = settings.sandbox_allow ?? '';
  const allowed = allowancesOf(text);
  const remove = (a: Allowance) =>
    set.mutate(
      { agent, values: { sandbox_allow: without(text, a.line) } },
      { onError: (e) => toast(`Did not remove: ${why(e) || 'try again'}`, { tone: 'warning' }) },
    );
  return (
    <Block
      title="Sandbox"
      note="What it may do beyond package registries, added when it asks and you allow. A removal holds from its next fresh session."
    >
      <Setting title="Run in a sandbox">
        <Toggle
          label="Run in a sandbox"
          on={on}
          disabled={set.isPending}
          onChange={(v) => set.mutate({ agent, values: { sandbox: v ? 'on' : 'off' } })}
        />
      </Setting>
      {on && !allowed.length && (
        <p className="text-sm text-muted">
          Nothing beyond package registries. When its work needs more, it asks you, and what you allow shows here.
        </p>
      )}
      {allowed.length > 0 && (
        <ul className={`-mx-2 flex flex-col border-t border-border ${on ? '' : 'opacity-60'}`}>
          {allowed.map((a) => {
            const { icon: Icon, word } = KIND[a.kind];
            return (
              <li key={a.line} className="flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0">
                <Icon size={14} className="shrink-0 text-muted" aria-hidden />
                {a.kind === 'cert' ? (
                  <span className="min-w-0 flex-1 truncate text-sm text-fg">{words(a)}</span>
                ) : (
                  <code className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{words(a)}</code>
                )}
                {word && <span className="shrink-0 text-xs text-muted">{word}</span>}
                <span className="shrink-0 text-xs text-muted">allowed {day(`${a.day}T00:00`)}</span>
                <IconButton label={`Stop allowing ${words(a)}`} disabled={set.isPending} onClick={() => remove(a)}>
                  <X size={14} />
                </IconButton>
              </li>
            );
          })}
        </ul>
      )}
    </Block>
  );
}
