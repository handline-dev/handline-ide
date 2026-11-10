import { useAgents } from '@handline/app-data';
import { activityOf, holdLine } from '#src/layout/agentTree.ts';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { Page } from '#src/ui/Page.tsx';

/** At the chat's top while the agent stands still: the sidebar's mark and the same one line of why. */
export function HoldBar({ agent }: { agent: string }) {
  const a = useAgents().data?.agents.find((x) => x.name === agent);
  const line = a && holdLine(a);
  if (!a || !line) return null;
  const act = activityOf(a);
  const tone = act === 'error' ? 'bg-danger/10' : act === 'you' ? 'bg-warning/10' : 'bg-hover';
  return (
    <Page>
      <div role="status" className={`mt-2 flex items-center gap-2 rounded-lg px-3 py-2 text-sm ${tone}`}>
        <span className="grid size-4 shrink-0 place-items-center">
          <ActivityIcon act={act} />
        </span>
        <span className="min-w-0 flex-1 truncate">{line}</span>
      </div>
    </Page>
  );
}
