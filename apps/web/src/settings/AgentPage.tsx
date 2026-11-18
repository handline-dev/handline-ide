// Settings › Agents › one agent: what it learned (its lessons, each removable) and how its finished work is reviewed (its level, its
// record, and the level the record suggests, which only the founder sets).
import { classify, hooks, sentence, useAgentReview, useLessons, useNameOf, useWrite } from '@handline/app-data';
import type { Methods } from '@handline/client';
import { ArrowLeft, Check, ChevronDown, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Block } from '#src/settings/Meter.tsx';
import { openSettingsFor } from '#src/settings/open.ts';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';

export type Level = Methods['agent.review']['result']['level'];
export const LEVEL_WORD: Record<Level, string> = { every: 'Every task', one_in_three: '1 in 3', auto: 'Auto-merge' };
const LEVELS: Level[] = ['every', 'one_in_three', 'auto'];

/** What each level does, said under the levels for the one picked. */
const MEANS: Record<Level, string[]> = {
  every: ['Every finished task waits for your OK or Redo'],
  one_in_three: [
    'One in three finished tasks waits for your OK or Redo',
    'The other two merge once the check and tests pass',
    'Any Redo moves it back to Every task',
  ],
  auto: [
    'Finished tasks merge once the check and tests pass',
    'You can still Redo a task after it merged',
    'Any Redo moves it back to 1 in 3',
  ],
};

/** Lessons shown before the rest fold into "N more". */
const SHOWN = 8;

const day = (iso: string) => new Date(iso).toLocaleDateString([], { month: 'short', day: 'numeric' });

export function AgentPage({ agent, back }: { agent: string; back: () => void }) {
  const name = useNameOf()(agent);
  return (
    <SettingsPage
      keep={`settings-agent-${agent}`}
      title={name}
      note="What it learned, and how much of its finished work you check."
      action={
        <Button variant="ghost" onClick={back}>
          <ArrowLeft size={14} />
          Agents
        </Button>
      }
    >
      <Lessons agent={agent} />
      <Review agent={agent} />
    </SettingsPage>
  );
}

function Lessons({ agent }: { agent: string }) {
  const lessons = useLessons(agent);
  const remove = useWrite('lesson.remove');
  const [all, setAll] = useState(false);
  const items = lessons.data?.items ?? [];
  const shown = all ? items : items.slice(0, SHOWN);
  return (
    <Block title={`What it learned · ${items.length} ${items.length === 1 ? 'lesson' : 'lessons'}`}>
      {lessons.data && !items.length && (
        <p className="text-sm text-muted">Nothing yet. When you Redo its work, it keeps one lesson from your note.</p>
      )}
      {!!shown.length && (
        <ul className="-mx-2 flex flex-col">
          {shown.map((l) => (
            <li key={l.id} className="group flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0 hover:bg-hover/30">
              <span className="min-w-0 flex-1 text-sm text-fg">{l.text}</span>
              <span className="shrink-0 text-xs text-muted">
                {[l.task != null && `Redo on T${l.task}`, day(l.created_at)].filter(Boolean).join(' · ')}
              </span>
              <IconButton
                label="Remove this lesson"
                className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                disabled={remove.isPending}
                onClick={() =>
                  remove.mutate(
                    { id: l.id },
                    { onError: (e) => toast(`Did not remove: ${sentence(classify(e)) || 'try again'}`, { tone: 'warning' }) },
                  )
                }
              >
                <Trash2 size={14} />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
      {items.length > SHOWN && (
        <button type="button" onClick={() => setAll(!all)} className="flex items-center gap-1 self-start text-xs text-muted hover:text-fg">
          <ChevronDown size={14} className={all ? 'rotate-180' : ''} />
          {all ? 'Fewer' : `${items.length - SHOWN} more`}
        </button>
      )}
    </Block>
  );
}

function Review({ agent }: { agent: string }) {
  const review = useAgentReview(agent);
  const pending = hooks.useRequestsPending();
  const set = useWrite('agent.review_set');
  const dismiss = useWrite('warning.dismiss');
  const r = review.data;
  const [picked, pick] = useState<Level | null>(null);
  if (!r) return null;
  const chosen = picked ?? r.suggested ?? r.level;
  const ok = r.record.filter((x) => x.ok_first).length;
  // The notice in Requests, while it stands: Not now dismisses it.
  const notice = pending.data?.answer.find((p) => p.warning?.kind === 'review' && p.warning.agent === agent);
  const fail = (what: string) => (e: unknown) => toast(`Did not ${what}: ${sentence(classify(e)) || 'try again'}`, { tone: 'warning' });
  return (
    <Block title="Review">
      <div className="flex items-center gap-3">
        <span className="flex gap-1" aria-hidden>
          {Array.from({ length: 10 }, (_, i) => {
            const x = r.record[r.record.length - 10 + i];
            return <span key={i} className={`size-2 rounded-full ${!x ? 'bg-surface' : x.ok_first ? 'bg-success' : 'bg-warning'}`} />;
          })}
        </span>
        <span className="text-sm text-fg">
          {r.record.length
            ? `${ok} of its last ${r.record.length} ${r.record.length === 1 ? 'task was' : 'tasks were'} OK at first look`
            : 'No finished tasks at this level yet'}
        </span>
      </div>
      <div role="radiogroup" aria-label="Review level" className="flex gap-0.5 self-start rounded-md bg-surface p-0.5">
        {LEVELS.map((l) => (
          <button
            key={l}
            type="button"
            role="radio"
            aria-checked={chosen === l}
            onClick={() => pick(l)}
            className={`flex items-center gap-2 rounded px-3 py-1 text-sm ${chosen === l ? 'bg-bg text-fg shadow-sm' : 'text-muted hover:text-fg'}`}
          >
            {LEVEL_WORD[l]}
            {l === r.level && <span className="rounded-full bg-surface px-1.5 text-xs text-muted">Now</span>}
            {l === r.suggested && <span className="rounded-full bg-accent/15 px-1.5 text-xs text-accent">Suggested</span>}
          </button>
        ))}
      </div>
      <ul className="flex flex-col gap-1.5">
        {MEANS[chosen].map((m) => (
          <li key={m} className="flex items-center gap-2 text-sm text-fg">
            <Check size={14} className="text-success" />
            {m}
          </li>
        ))}
      </ul>
      {(notice || chosen !== r.level) && (
        <div className="flex justify-end gap-2 border-t border-border pt-3">
          {notice?.id && (
            <Button disabled={dismiss.isPending} onClick={() => dismiss.mutate({ id: notice.id! }, { onError: fail('put it off') })}>
              Not now
            </Button>
          )}
          <Button
            variant="primary"
            disabled={chosen === r.level || set.isPending}
            onClick={() => set.mutate({ agent, level: chosen }, { onSuccess: () => pick(null), onError: fail('change it') })}
          >
            Change to {LEVEL_WORD[chosen]}
          </Button>
        </div>
      )}
    </Block>
  );
}

/** A review notice's one button in Requests: its agent's page, where the level is changed or the notice put off. */
export function OpenReview({ warning }: { warning: { kind: string; agent?: string } }) {
  return (
    <Button variant="primary" onClick={() => openSettingsFor(warning)}>
      Open
    </Button>
  );
}
