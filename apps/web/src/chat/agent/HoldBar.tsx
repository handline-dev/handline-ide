import { classify, sentence, useAccounts, useAgents, useUnstall } from '@handline/app-data';
import { activityOf, holdLine, restarting } from '#src/layout/agentList.ts';
import { openSettings } from '#src/settings/open.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import { Button, useAction } from '#src/ui/Button.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { Page } from '#src/ui/Page.tsx';
import { toast } from '#src/ui/toast.tsx';

/** At the chat's top while the agent stands still: the sidebar's mark, why in full, and the one button that gets it going. */
export function HoldBar({ agent }: { agent: string }) {
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const accounts = useAccounts().data?.accounts;
  const unstall = useUnstall(agent);
  const terminal = useOpenable('shell');
  const wake = useAction();
  const line = a && !restarting(a) && holdLine(a); // a restart shows at the chat's foot, in the turn's row
  const need = a?.needs_you?.source === 'daemon' ? a.needs_you : null;
  if (!a || (!line && !need)) return null;
  const act = activityOf(a);
  const tone = act === 'error' ? 'bg-danger/10' : act === 'you' ? 'bg-warning/10' : 'bg-hover';
  const needsLogin = act === 'error' && accounts?.find((x) => x.name === a.account)?.login_state === 'needs_login';
  const button =
    need?.kind === 'start_failed' || need?.kind === 'no_session' ? (
      <Button
        busy={wake.busy}
        // A refused wake has cleared the stall all the same, so this bar may be gone by the answer: the reason is a toast.
        onClick={() => void wake.go(() => unstall().catch((e: unknown) => toast(sentence(classify(e)), { tone: 'warning' })))}
      >
        Try again
      </Button>
    ) : needsLogin ? (
      <Button onClick={() => openSettings({ section: 'accounts', account: a.account ?? undefined })}>Log in</Button>
    ) : need?.kind === 'trust' || need?.kind === 'dialog' ? (
      <Button {...terminal}>Open terminal</Button>
    ) : null;
  return (
    <Page>
      <div role="status" className={`mt-2 flex items-start gap-2 rounded-lg px-3 py-2 text-sm ${tone}`}>
        <span className="mt-0.5 grid size-4 shrink-0 place-items-center">
          <ActivityIcon act={act} />
        </span>
        <div className="min-w-0 flex-1 break-words">
          {need ? (
            <>
              <div className="font-medium">{need.title}</div>
              {need.text && <div className="text-muted">{need.text}</div>}
            </>
          ) : (
            line
          )}
          {button && <div className="mt-2 flex justify-end">{button}</div>}
        </div>
      </div>
    </Page>
  );
}
