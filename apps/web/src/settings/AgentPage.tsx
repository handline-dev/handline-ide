import { day } from '#src/format/time.ts';
import { count } from '#src/format/count.ts';
// Settings › Agents › one agent: what it learned (its lessons, each removable), what its sandbox allows, and how its finished work is
// reviewed, per kind of work.
import { useAgentDetail, useAgentReview, useLessons, useNameOf, useWrite, why } from '@handline/app-data';
import type { Methods } from '@handline/client';
import { AppWindow, Bot, ChevronDown, Database, FileText, Lock, Server, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { SettingsBar } from '#src/settings/SettingsBar.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { InfoTip } from '#src/ui/InfoTip.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { toast } from '#src/ui/toast.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';
import { DeviceLine } from '#src/settings/DeviceControls.tsx';
import { useAgentDevice } from '#src/settings/data/devices.ts';

type Rung = Methods['agent.review']['result']['ladder'][number];
export type Level = Rung['level'];
export type Work = Rung['work'];
export const LEVEL_WORD: Record<Level, string> = {
  every: 'Every',
  one_in_two: '1 in 2',
  one_in_three: '1 in 3',
  one_in_five: '1 in 5',
  never: 'Never',
};
const LEVELS: Level[] = ['every', 'one_in_two', 'one_in_three', 'one_in_five', 'never'];
export const WORK_WORD: Record<Work, string> = {
  contract: 'Contract and data',
  screen: 'Screen',
  daemon: 'Daemon',
  docs: 'Docs and cleanup',
};
const WORK_ICON: Record<Work, typeof Database> = { contract: Database, screen: AppWindow, daemon: Server, docs: FileText };
/** First-look OKs in a row that offer one step less. */
const RUN = 10;

/** Lessons shown before the rest fold into "N more". */
const SHOWN = 8;

export function AgentPage({ agent, back }: { agent: string; back?: () => void }) {
  const name = useNameOf()(agent);
  const device = useAgentDevice(agent);
  return (
    <SettingsPage
      bar={back && <SettingsBar title={name} icon={<Bot size={16} className="shrink-0 text-muted" />} back={back} backTo="Agents" />}
      keep={`settings-agent-${agent}`}
      note={
        <>
          What it learned, and what it may do.
          {device && <DeviceLine device={device} />}
        </>
      }
    >
      <Lessons agent={agent} />
      <Sandbox agent={agent} />
      <Review agent={agent} />
    </SettingsPage>
  );
}

/** An agent's lessons, or the founder's preferences (`founder`, kept by the agents and read by every one), each removable. */
export function Lessons({ agent, founder = false }: { agent: string; founder?: boolean }) {
  const lessons = useLessons(agent);
  const remove = useWrite('lesson.remove');
  const [all, setAll] = useState(false);
  const items = lessons.data?.items ?? [];
  const shown = all ? items : items.slice(0, SHOWN);
  return (
    <Block
      title={founder ? `Your preferences · ${items.length}` : `What it learned · ${count(items.length, 'lesson')}`}
      note={founder ? 'What you said you want. Every agent follows these.' : undefined}
    >
      {lessons.data && !items.length && (
        <p className="text-sm text-muted">
          {founder
            ? 'None yet. When you tell an agent how you want something done, it keeps it here.'
            : 'Nothing yet. When you Redo its work, it keeps one lesson from your note.'}
        </p>
      )}
      {!!shown.length && (
        <ul className="-mx-2 flex flex-col">
          {shown.map((l) => (
            <li key={l.id} className="group flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0 hover:bg-hover/30">
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="text-sm text-fg">{l.text}</span>
                <span className="text-xs text-muted">
                  {[l.task != null && (founder ? `T${l.task}` : `Redo on T${l.task}`), day(l.created_at)].filter(Boolean).join(' · ')}
                </span>
              </span>
              <IconButton
                label={founder ? 'Remove this preference' : 'Remove this lesson'}
                className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                disabled={remove.isPending}
                onClick={() =>
                  remove.mutate({ id: l.id }, { onError: (e) => toast(`Did not remove: ${why(e) || 'try again'}`, { tone: 'warning' }) })
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

/** sandbox_allow's words, each a switch. */
const ALLOW: { word: string; title: string }[] = [
  { word: 'weaker-network', title: 'Allow Go module downloads' },
  { word: 'pnpm-install', title: 'Allow pnpm install' },
];

/** Its sandbox and what the sandbox allows; each switch writes at once, the allowances greyed while the sandbox is off. */
export function Sandbox({ agent }: { agent: string }) {
  const settings = useAgentDetail(agent).data?.settings;
  const set = useWrite('agent.set', agent);
  if (!settings) return null;
  const on = settings.sandbox === 'on';
  const words = new Set((settings.sandbox_allow ?? '').split(' ').filter(Boolean));
  const allow = (word: string, yes: boolean) => {
    const next = new Set(words);
    if (yes) next.add(word);
    else next.delete(word);
    set.mutate({ agent, values: { sandbox_allow: [...next].join(' ') } });
  };
  return (
    <Block title="Sandbox">
      <Setting title="Run in a sandbox">
        <Toggle
          label="Run in a sandbox"
          on={on}

          onChange={(v) => set.mutate({ agent, values: { sandbox: v ? 'on' : 'off' } })}
        />
      </Setting>
      {ALLOW.map((a) => (
        <Setting key={a.word} title={<span className={on ? '' : 'text-muted'}>{a.title}</span>}>
          <Toggle label={a.title} on={words.has(a.word)} disabled={!on} onChange={(v) => allow(a.word, v)} />
        </Setting>
      ))}
    </Block>
  );
}

/** One row per kind of work the agent has done: its level, and its run of first-look OKs toward one step less. */
function Review({ agent }: { agent: string }) {
  const ladder = useAgentReview(agent).data?.ladder ?? [];
  if (!ladder.length) return null;
  return (
    <Block
      title="Review"
      action={
        <InfoTip label="What the levels mean">
          A level is how many of its finished tasks of that kind wait for your OK or Redo; the others merge once the check and tests pass.{' '}
          {RUN} OKs in a row at first look offer one step less, and any Redo puts it back to Every. Contract and data work is always looked
          at.
        </InfoTip>
      }
    >
      <ul className="-mx-2 flex flex-col">
        {ladder.map((rung) => (
          <ReviewRow key={rung.work} agent={agent} rung={rung} />
        ))}
      </ul>
    </Block>
  );
}

/** The OKs at first look since the last miss, newest last. */
const runOf = (record: Rung['record']) => {
  let n = 0;
  while (record.at(-1 - n)?.ok_first) n++;
  return n;
};

function ReviewRow({ agent, rung: r }: { agent: string; rung: Rung }) {
  const Icon = WORK_ICON[r.work];
  const floor = r.work === 'contract';
  const run = runOf(r.record);
  return (
    <li className="flex items-center gap-3 border-t border-border px-2 py-2 first:border-t-0">
      <Icon size={14} className="shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate text-sm text-fg">{WORK_WORD[r.work]}</span>
      <Dots record={r.record} />
      <span className="w-14 text-right text-xs text-muted tabular-nums">{floor || r.level === 'never' ? '' : `${run} of ${RUN}`}</span>
      {floor ? (
        <span className="flex w-24 items-center justify-end gap-1.5 text-sm text-muted">
          <Lock size={12} />
          {LEVEL_WORD.every}
        </span>
      ) : (
        <LevelMenu agent={agent} rung={r} />
      )}
    </li>
  );
}

/** The last ten first looks, oldest first: a filled dot an OK, a cross a Redo, a ring a look not had yet. */
function Dots({ record }: { record: Rung['record'] }) {
  const last = record.slice(-RUN);
  return (
    <span
      className="flex items-center gap-1"
      role="img"
      aria-label={`${last.filter((x) => x.ok_first).length} of the last ${last.length} OK at first look`}
    >
      {Array.from({ length: RUN }, (_, i) => {
        const x = last[i - (RUN - last.length)];
        if (!x) return <span key={i} className="size-2 rounded-full border border-border" />;
        if (!x.ok_first) return <X key={i} size={10} strokeWidth={3} className="text-warning" />;
        return <span key={i} className="size-2 rounded-full bg-success" />;
      })}
    </span>
  );
}

/** A kind's level, set by hand from a menu of the levels; the pick is saved at once. */
function LevelMenu({ agent, rung }: { agent: string; rung: Rung }) {
  const set = useWrite('agent.review_set');
  const pick = (level: Level) =>
    level !== rung.level &&
    set.mutate(
      { agent, work: rung.work, level },
      { onError: (e) => toast(`Did not change it: ${why(e) || 'try again'}`, { tone: 'warning' }) },
    );
  return (
    <Menu
      align="end"
      trigger={
        <Button className="w-24 justify-between!" busy={set.isPending} aria-label={`${WORK_WORD[rung.work]} level`}>
          {LEVEL_WORD[rung.level]}
          <ChevronDown size={14} className="text-muted" />
        </Button>
      }
    >
      {LEVELS.map((l) => (
        <MenuItem key={l} checked={l === rung.level} onClick={() => pick(l)}>
          {LEVEL_WORD[l]}
        </MenuItem>
      ))}
    </Menu>
  );
}
