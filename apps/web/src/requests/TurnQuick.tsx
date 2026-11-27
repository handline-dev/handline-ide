// A Requests row's answers: a proposal's Allow or Deny, a step-down offer's Yes or Not now, a warning's dismiss, or Later.
import { classify, sentence, useProposals, useUnstall, useWrite, why } from '@handline/app-data';
import type { PendingRequest } from '@handline/client';
import { laterKey, tomorrowAt9, turnKey } from '@handline/app-core';
import { Play, RotateCw, X } from 'lucide-react';
import { useState } from 'react';
import { useRowActions } from '#src/requests/RowMenu.tsx';
import { RunButton } from '#src/requests/RunRow.tsx';
import { Button, useAction } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';

/** A proposal's Allow (Run for a command) or Deny; a step-down offer's Yes or Not now; a daemon warning's dismiss, kept by the daemon while its cause
 *  lasts; else a dismiss until tomorrow 09:00 (Later). An Allow names the subject proposal.list showed; one the daemon refuses as
 *  changed since stays off on this row, so a changed proposal is opened and read before it is allowed. */
export function TurnQuick({ p }: { p: PendingRequest }) {
  const act = useRowActions();
  const dismiss = useWrite('warning.dismiss');
  if (p.source === 'proposal' && p.id) return <ProposalAnswers id={Number(p.id)} />;
  if (p.warning?.kind === 'review' && p.id) return <ReviewOffer id={p.id} warning={p.warning} />;
  if (p.source === 'machine' && p.id) {
    const id = p.id;
    return (
      <>
        {p.warning?.stall && p.warning.agent && <TryAgain agent={p.warning.agent} />}
        <IconButton
          label="Dismiss"
          disabled={dismiss.isPending}
          onClick={() =>
            dismiss.mutate(
              { id },
              {
                onError: (e) => {
                  const s = why(e);
                  if (s) toast(`Did not dismiss: ${s}`, { tone: 'warning' });
                },
              },
            )
          }
        >
          <X size={14} />
        </IconButton>
      </>
    );
  }
  const later = laterKey(p);
  // A note is not put off: opening its task reads it.
  if (!later || p.kind === 'note') return null;
  return (
    <>
      {p.kind === 'run' && p.task !== null && <RunButton task={p.task} />}
      <IconButton
        label="Dismiss until tomorrow 09:00"
        onClick={() => void act.later([{ keys: [turnKey(p)], later }], tomorrowAt9(Date.now()))}
      >
        <X size={14} />
      </IconButton>
    </>
  );
}

/** A step-down offer's answer in place: Yes sets the offered level for its kind of work, Not now puts it off until 10 more looks. */
function ReviewOffer({ id, warning: w }: { id: string; warning: NonNullable<PendingRequest['warning']> }) {
  const set = useWrite('agent.review_set');
  const dismiss = useWrite('warning.dismiss');
  const busy = set.isPending || dismiss.isPending;
  const fail = (what: string) => (e: unknown) => toast(`Did not ${what}: ${why(e) || 'try again'}`, { tone: 'warning' });
  if (!w.agent || !w.work || !w.level) return null;
  const answer = { agent: w.agent, work: w.work, level: w.level };
  return (
    <>
      <Button disabled={busy} onClick={() => dismiss.mutate({ id }, { onError: fail('put it off') })}>
        Not now
      </Button>
      <Button variant="primary" disabled={busy} onClick={() => set.mutate(answer, { onError: fail('change it') })}>
        Yes
      </Button>
    </>
  );
}

/** A stalled agent's one action: clear the stall so the daemon starts it again; a refusal is a toast, since the row may be gone by then. */
function TryAgain({ agent }: { agent: string }) {
  const unstall = useUnstall(agent);
  const run = useAction();
  return (
    <Button busy={run.busy} onClick={() => void run.go(() => unstall().catch((e: unknown) => toast(why(e), { tone: 'warning' })))}>
      <RotateCw size={14} />
      Try again
    </Button>
  );
}

/** A proposal's Deny and Allow (Run, with its play mark, for a command); `runFirst` puts Run before Deny, as under a command. */
export function ProposalAnswers({ id, runFirst }: { id: number; runFirst?: boolean }) {
  const decide = useWrite('proposal.decide');
  const listed = useProposals().data?.proposals.find((x) => x.id === id);
  const [changed, setChanged] = useState(false);
  const answer = (decision: 'approve' | 'reject') =>
    decide.mutate(
      { proposal: id, decision, ...(decision === 'approve' && listed ? { subject: listed.subject } : {}) },
      {
        onError: (e) => {
          const f = classify(e);
          if (f.kind === 'refused' && f.gate === 'proposal_changed') {
            setChanged(true);
            toast('The proposal changed since it was listed: open it and read it before you allow it.', { tone: 'warning' });
          } else if (sentence(f)) toast(`Did not answer: ${sentence(f)}`, { tone: 'warning' });
        },
      },
    );
  const deny = (
    <Button disabled={decide.isPending} onClick={() => answer('reject')}>
      Deny
    </Button>
  );
  const run = (
    <Button
      variant="primary"
      disabled={decide.isPending || !listed || changed}
      title={changed ? 'Changed since it was listed: open it to read it' : listed ? undefined : 'Reading the proposal'}
      onClick={() => answer('approve')}
    >
      {listed?.kind === 'run' ? (
        <>
          <Play size={14} />
          Run
        </>
      ) : (
        'Allow'
      )}
    </Button>
  );
  return runFirst ? (
    <>
      {run}
      {deny}
    </>
  ) : (
    <>
      {deny}
      {run}
    </>
  );
}
