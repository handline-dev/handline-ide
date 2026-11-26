// Settings › Agents › one agent: its sandbox switch and what the sandbox lets it do, one removable row per sandbox_allow entry.
import { useAgentDetail, useWrite, why } from '@handline/app-data';
import { X } from 'lucide-react';
import { day } from '#src/format/time.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

/** One line of sandbox_allow, `<day> <kind> <value>`, as the daemon keeps it. */
export type Allowance = { line: string; day: string; kind: string; value: string };

const KIND: Record<string, string> = { host: 'Reach', command: 'Run outside', write: 'Write', cert: 'Certificate check' };

/** The list's entries in order; a line that does not read as one is left out, as the daemon leaves it out. */
export function allowancesOf(text: string): Allowance[] {
  return text.split('\n').flatMap((l) => {
    const m = /^(\d{4}-\d{2}-\d{2}) (host|command|write|cert)(?: (.+))?$/.exec(l.trim());
    return m ? [{ line: l.trim(), day: m[1]!, kind: m[2]!, value: m[3] ?? '' }] : [];
  });
}

/** The list with `line` taken out, one entry per line. */
export const without = (text: string, line: string) =>
  allowancesOf(text)
    .filter((a) => a.line !== line)
    .map((a) => a.line)
    .join('\n');

const words = (a: Allowance) => (a.kind === 'cert' ? 'The certificate check Go tools need' : a.value);

/** Its sandbox switch; then, once it has any, what the sandbox allows it, each row's x removing it at once. */
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
    <Block title="Sandbox">
      <Setting title="Run in a sandbox">
        <Toggle
          label="Run in a sandbox"
          on={on}
          disabled={set.isPending}
          onChange={(v) => set.mutate({ agent, values: { sandbox: v ? 'on' : 'off' } })}
        />
      </Setting>
      {allowed.length > 0 && (
        <div className={`flex flex-col gap-1 border-t border-border pt-3 ${on ? '' : 'opacity-60'}`}>
          <p className="text-sm text-fg">Allowed</p>
          <ul className="-mx-2 flex flex-col">
            {allowed.map((a) => (
              <li key={a.line} className="flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0">
                <span className="w-32 shrink-0 text-xs text-muted">{KIND[a.kind]}</span>
                {a.kind === 'cert' ? (
                  <span className="min-w-0 flex-1 truncate text-sm text-fg">{words(a)}</span>
                ) : (
                  <code className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{words(a)}</code>
                )}
                <span className="shrink-0 text-xs text-muted">{day(`${a.day}T00:00`)}</span>
                <IconButton label={`Stop allowing ${words(a)}`} disabled={set.isPending} onClick={() => remove(a)}>
                  <X size={14} />
                </IconButton>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Block>
  );
}
