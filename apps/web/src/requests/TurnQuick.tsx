// A Requests row's answers: a proposal's Allow or Deny, a step-down offer's Yes or Not now, a warning's dismiss, or Later.
import { classify, sentence, useProposals, useWrite } from '@handline/app-data';
import type { PendingRequest } from '@handline/client';
import { laterKey, tomorrowAt9, turnKey } from '@handline/app-core';
import { X } from 'lucide-react';
import { useState } from 'react';
import { useRowActions } from '#src/requests/RowMenu.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';

/** A proposal's Allow or Deny; a step-down offer's Yes or Not now; a daemon warning's dismiss, kept by the daemon while its cause
 *  lasts; else a dismiss until tomorrow 09:00 (Later). An Allow names the subject proposal.list showed; one the daemon refuses as
 *  changed since stays off on this row, so a changed proposal is opened and read before it is allowed. */
export function TurnQuick({ p }: { p: PendingRequest }) {
  const act = useRowActions();
  const decide = useWrite('proposal.decide');
  const dismiss = useWrite('warning.dismiss');
  const listed = useProposals().data?.proposals.find((x) => String(x.id) === p.id);
  const [changed, setChanged] = useState(false);
  if (p.source === 'proposal' && p.id) {
    const answer = (decision: 'approve' | 'reject') =>
      decide.mutate(
        { proposal: Number(p.id), decision, ...(decision === 'approve' && listed ? { subject: listed.subject } : {}) },
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
    return (
      <>
        <Button disabled={decide.isPending} onClick={() => answer('reject')}>
          Deny
        </Button>
        <Button
          variant="primary"
          disabled={decide.isPending || !listed || changed}
          title={changed ? 'Changed since it was listed: open it to read it' : listed ? undefined : 'Reading the proposal'}
          onClick={() => answer('approve')}
        >
          Allow
        </Button>
      </>
    );
  }
  if (p.warning?.kind === 'review' && p.id) return <ReviewOffer id={p.id} warning={p.warning} />;
  if (p.source === 'machine' && p.id) {
    const id = p.id;
    return (
      <IconButton
        label="Dismiss"
        disabled={dismiss.isPending}
        onClick={() =>
          dismiss.mutate(
            { id },
            {
              onError: (e) => {
                const s = sentence(classify(e));
                if (s) toast(`Did not dismiss: ${s}`, { tone: 'warning' });
              },
            },
          )
        }
      >
        <X size={14} />
      </IconButton>
    );
  }
  const later = laterKey(p);
  // A note is not put off: opening its task reads it.
  if (!later || p.kind === 'note') return null;
  return (
    <IconButton
      label="Dismiss until tomorrow 09:00"
      onClick={() => void act.later([{ keys: [turnKey(p)], later }], tomorrowAt9(Date.now()))}
    >
      <X size={14} />
    </IconButton>
  );
}

/** A step-down offer's answer in place: Yes sets the offered level for its kind of work, Not now puts it off until 10 more looks. */
function ReviewOffer({ id, warning: w }: { id: string; warning: NonNullable<PendingRequest['warning']> }) {
  const set = useWrite('agent.review_set');
  const dismiss = useWrite('warning.dismiss');
  const busy = set.isPending || dismiss.isPending;
  const fail = (what: string) => (e: unknown) => toast(`Did not ${what}: ${sentence(classify(e)) || 'try again'}`, { tone: 'warning' });
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
