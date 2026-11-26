// Whether an agent stays on the account it is on: its own account_split (manual pins it, auto lets the daemon move it), else the
// shared one. Unpinning back to what the shared value says clears the agent's own, so it follows that again.
import { useSettings, useWrite } from '@handline/app-data';

export function useAccountPin(agent: string) {
  const settings = useSettings(agent);
  const save = useWrite('settings.set');
  const own = (settings.data?.overrides as Record<string, unknown> | undefined)?.account_split;
  const shared = (settings.data?.values as Record<string, unknown> | undefined)?.account_split ?? 'auto';
  const pinned = (own ?? shared) === 'manual';
  return {
    ready: settings.isSuccess,
    pinned,
    saving: save.isPending,
    error: save.error,
    set: (pin: boolean) => {
      const value = pin ? 'manual' : 'auto';
      save.mutate({ agent, changes: { account_split: value === shared ? null : value } });
    },
  };
}
