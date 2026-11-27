// Settings > Agents > an agent: the exact commands the founder always allows it, each with Remove (from its next fresh session).
import { useRead, useWrite, why } from '@handline/app-data';
import { X } from 'lucide-react';
import { day } from '#src/format/time.ts';
import { Block } from '#src/settings/Meter.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';

/** A rule as the founder reads it: Claude Code's Bash(npm test) is the command, npm test. */
export const ruleText = (rule: string) => /^Bash\((.*)\)$/s.exec(rule)?.[1] ?? rule;

/** Nothing until the agent has one; then the list, each row's x removing it. */
export function AllowRules({ agent }: { agent: string }) {
  const rules = useRead('access.list', { agent }).data?.rules ?? [];
  const remove = useWrite('access.rule_remove', agent);
  if (!rules.length) return null;
  return (
    <Block title="Always allowed" note="Commands it runs without asking. A removal holds from its next fresh session.">
      <ul className="-mx-2 flex flex-col">
        {rules.map((r) => (
          <li key={r.rule} className="flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0">
            <code className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{ruleText(r.rule)}</code>
            <span className="shrink-0 text-xs text-muted">{day(r.at)}</span>
            <IconButton
              label={`Stop always allowing ${ruleText(r.rule)}`}
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(
                  { agent, rule: r.rule, request_id: uuid() },
                  { onError: (e) => toast(`Did not remove: ${why(e) || 'try again'}`, { tone: 'warning' }) },
                )
              }
            >
              <X size={14} />
            </IconButton>
          </li>
        ))}
      </ul>
    </Block>
  );
}
