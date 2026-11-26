// The schedules: the messages Handline sends an agent at set times, each with when it is sent next and when it was last sent, which
// opens that message in the agent's chat. Read-only: an agent asks for a schedule and the founder allows it. Settings › Schedules
// lists every agent's; an agent's settings tab lists its own.
import { Hooks, useNameOf } from '@handline/app-data';
import type { Methods } from '@handline/client';
import { ArrowUpRight } from 'lucide-react';
import { aimChat } from '#src/chat/aim.ts';
import { clockOrDay, useNow } from '#src/format/time.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { nextWord, zoneNote } from '#src/settings/data/schedules.ts';
import { Block } from '#src/settings/Meter.tsx';
import { closeSettings } from '#src/settings/open.ts';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Ago } from '#src/ui/Ago.tsx';
import { Mark } from '#src/ui/Face.tsx';

type Reply = Methods['schedules.list']['result'];
type Schedule = Reply['schedules'][number];

/** Read again this often while shown: no event says a schedule changed or was sent. */
const REFRESH_MS = 30_000;

/** Settings › Schedules: every agent's. */
export function SchedulesTab() {
  return (
    <SettingsPage keep="settings-schedules" title="Schedules" note="Messages Handline sends an agent at set times.">
      <Schedules />
    </SettingsPage>
  );
}

/** The schedules as one card of rows: every agent's, each row naming its agent, or with `agent` that agent's own, which shows nothing
 * until it has one. */
export function Schedules({ agent }: { agent?: string }) {
  const data = Hooks.useSchedulesList(agent ? { agent } : {}, { refetchMs: REFRESH_MS }).data;
  const nameOf = useNameOf();
  if (!data) return null;
  const { schedules, unreadable, zone } = data;
  if (agent && !schedules.length && !unreadable.length) return null;
  const count = schedules.length ? ` · ${schedules.length}` : '';
  return (
    <Block title={`${agent ? 'Schedules' : 'Scheduled messages'}${count}`} note={schedules.length ? zoneNote(zone) : undefined}>
      {!schedules.length && !unreadable.length && (
        <p className="text-sm text-muted">None yet. An agent asks for a schedule, and it starts when you allow it.</p>
      )}
      {unreadable.map((u) => (
        <p key={u.agent} role="alert" className="text-xs text-warning">
          {nameOf(u.agent)}&apos;s schedules cannot be read, so none of them is sent: {u.why}.
        </p>
      ))}
      {!!schedules.length && (
        <ul className="-mx-2 flex flex-col">
          {schedules.map((s) => (
            <Row key={`${s.agent}:${s.id}`} s={s} named={!agent} />
          ))}
        </ul>
      )}
    </Block>
  );
}

/** One schedule: when it runs, in words (its cron when no sentence says it exactly), and what is sent; at the right when it is sent
 * next and when it was last. The columns go under the words when the row is narrow. */
function Row({ s, named }: { s: Schedule; named: boolean }) {
  const name = useNameOf()(s.agent);
  const now = useNow();
  const last = s.fires[0];
  return (
    <li className="flex flex-wrap items-start gap-x-6 gap-y-2 border-t border-border px-2 py-2.5 first:border-t-0">
      {/* The face stays beside the words however narrow the row is. */}
      <div className="flex min-w-0 flex-1 basis-56 items-start gap-3">
        {named && <Mark who={s.agent} />}
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex flex-wrap items-baseline gap-x-2 text-sm text-fg">
            {named && <span className="font-medium">{name}</span>}
            <span title={s.sentence ? s.cron : undefined}>{s.sentence ?? <code className="font-mono">{s.cron}</code>}</span>
          </span>
          <p className="line-clamp-2 text-xs break-words text-muted">
            {s.message}
            {s.message_more && '…'}
          </p>
          {s.due && <p className="text-xs text-warning">It is sent when {name} stops working.</p>}
        </div>
      </div>
      {/* A fixed width, so Next and Last stand in one column down the list whatever the times are. */}
      <dl className="flex w-40 shrink-0 flex-col gap-0.5 text-xs">
        <div className="flex items-baseline gap-3">
          <dt className="w-8 text-muted">Next</dt>
          <dd className="text-fg tabular-nums">{s.due ? 'Due now' : s.next ? nextWord(s.next, now) : 'Never'}</dd>
        </div>
        <div className="flex items-baseline gap-3">
          <dt className="w-8 text-muted">Last</dt>
          <dd>{last ? <LastRun agent={s.agent} fire={last} /> : <span className="text-muted">Not yet</span>}</dd>
        </div>
      </dl>
    </li>
  );
}

/** When it was last sent: opens the agent's chat on that message, by the app's scheme, and Settings closes so the chat shows. */
function LastRun({ agent, fire }: { agent: string; fire: Schedule['fires'][number] }) {
  const open = useOpenable(chatOf(agent), { focus: () => aimChat(agent, fire.request, fire.at), onOpen: closeSettings });
  return (
    <button
      type="button"
      {...open}
      title={`Sent ${clockOrDay(fire.at)}. Open it in the chat.`}
      className="flex items-center gap-1 rounded text-fg underline-offset-2 hover:underline"
    >
      <Ago at={fire.at} />
      <ArrowUpRight size={12} className="text-muted" />
    </button>
  );
}
