// What a pane's + menu offers. Pure.
import { agentKind } from '#src/layout/kinds.tsx';

type Kind<I> = { kind: string; label: string; icon?: I };

/** With an agent on screen (`who`), its kinds open for it and Terminal names it muted, as the worktree its shell opens in; with none,
 * Terminal opens a plain shell and says nothing, and an agent's other kinds ask which agent (none while the list is loading). */
export function newTabKinds<I>(
  kinds: Kind<I>[],
  who: string | null | undefined,
  nameOf: (key: string) => string,
  agents: { key: string; label: string }[] | undefined,
): (Kind<I> & { hint?: string; agents?: { key: string; label: string }[] })[] {
  return kinds.flatMap((c) => {
    if (c.kind === 'newshell') return [{ ...c, hint: who ? nameOf(who) : undefined }];
    if (who || !agentKind(c.kind)) return [c];
    return agents ? [{ ...c, agents }] : [];
  });
}
