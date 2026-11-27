import { count } from '#src/format/count.ts';
// Settings > Devices: every device in one flat list, the master first; a row opens its page. A row says only what its Run · Pause · Off
// cannot. Under it, where agents run; at the foot, the
// battery switch, Auto or Manual for a lent Mac taking the master's build, and the reclaim time.
import { useNameOf } from '@handline/app-data';
import { ListLoading } from '#src/ui/Loading.tsx';
import {
  ArchiveX,
  ArrowUpCircle,
  BatteryCharging,
  ChevronRight,
  Link2,
  Loader,
  Plus,
  RefreshCw,
  Timer,
  TriangleAlert,
  Zap,
} from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { pickRange } from '@handline/app-core';
import { AgentPlacement } from '#src/settings/AgentPlacement.tsx';
import { DevicePage } from '#src/settings/DevicePage.tsx';
import {
  KIND_ICON,
  KIND_WORD,
  MODE_WORD,
  ModeControl,
  OFF_TEXT,
  StateWord,
  confirmRevoke,
  eachOf,
  refused,
} from '#src/settings/DeviceControls.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { openPairSheet } from '#src/settings/PairSheet.tsx';
import { joinWithLink } from '#src/connection/join.tsx';
import { shell } from '#src/shell.ts';
import {
  MASTER,
  MODES,
  canUpdate,
  markSetAsideSeen,
  useDevices,
  useSetAside,
  type DeviceRow,
  type Mode,
} from '#src/settings/data/devices.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Expert } from '#src/ui/Expert.tsx';
import { Button } from '#src/ui/Button.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { confirm } from '#src/ui/modal.tsx';
import { selectionRoom, WithSelectionBar } from '#src/ui/SelectionBar.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';

const BUILD_MODES = ['auto', 'manual'] as const;

type Devices = ReturnType<typeof useDevices>;

/** Sets each row's mode, past any that fail; Off asks once for all of them. */
function setAll(rows: DeviceRow[], mode: Mode, q: Devices, done?: () => void) {
  const go = async () => {
    await eachOf(rows, (r) => q.setMode(r, mode), `set to ${MODE_WORD[mode]}`);
    done?.();
  };
  if (mode !== 'off') return void go().catch(refused);
  void confirm({
    title: rows.length === 1 ? `Turn ${rows[0]!.name} off?` : `Turn ${rows.length} devices off?`,
    message: OFF_TEXT,
    confirmText: 'Turn off',
    run: go,
  });
}

/** What the row says under its name: a Mac's or a server's agents, with a warning mark whose hover says the whole reason it cannot run one,
 * or that it is on another build; a phone's or a browser's Controls only. Its kind is the row's icon. */
function Detail({ r, q }: { r: DeviceRow; q: Devices }) {
  const nameOf = useNameOf();
  if (!r.runner) return <div className="truncate text-xs text-muted">Controls only</div>;
  const why = r.lacks.map((l) => `Cannot run ${nameOf(l.agent)}: ${l.why}.`);
  return (
    <div className="flex items-center gap-1 truncate text-xs text-muted">
      {count(r.agents.length, 'agent')}
      {why.length > 0 && (
        <HoverCard
          trigger={
            <span role="img" aria-label={why.join(' ')} data-lacks className="inline-flex text-warning">
              <TriangleAlert size={12} />
            </span>
          }
        >
          <div className="flex max-w-80 flex-col gap-1 px-2 py-1.5 text-xs text-fg">
            {why.map((w) => (
              <span key={w}>{w}</span>
            ))}
          </div>
        </HoverCard>
      )}
      {r.behind && !r.updating ? (
        <span>
          {' · '}
          <span className="text-warning">
            <TriangleAlert size={12} className="mb-0.5 inline" /> other build
          </span>
          {canUpdate(r) && (
            <>
              {' · '}
              <UpdateLink r={r} q={q}>
                Update
              </UpdateLink>
            </>
          )}
        </span>
      ) : (
        !r.behind &&
        r.appReady && (
          <span>
            {' · '}
            {canUpdate(r) ? (
              <UpdateLink r={r} q={q}>
                <ArrowUpCircle size={12} className="mb-0.5 inline" /> Update to {r.appReady}
              </UpdateLink>
            ) : (
              <span className="text-fg">
                <ArrowUpCircle size={12} className="mb-0.5 inline" /> Update ready {r.appReady}
              </span>
            )}
          </span>
        )
      )}
    </div>
  );
}

/** Beside the buttons only what they cannot say: offline since when, starting, updating, or a phone's last contact. */
function RowState({ r }: { r: DeviceRow }) {
  if (r.updating)
    return (
      <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-muted">
        <Loader size={12} /> Updating…
      </span>
    );
  if (r.state === 'running' || r.state === 'paused' || r.state === 'off') return null;
  return <StateWord row={r} />;
}

/** The row's Update: the machine takes the master's build, and its Mac app installs the version it has ready. */
function UpdateLink({ r, q, children }: { r: DeviceRow; q: Devices; children: ReactNode }) {
  return (
    <button
      type="button"
      className="text-fg underline"
      disabled={q.pending !== null}
      onClick={(e) => {
        e.stopPropagation();
        void q.update(r).catch(refused);
      }}
    >
      {children}
    </button>
  );
}

function RowMenu({ r, q, open }: { r: DeviceRow; q: Devices; open: () => void }) {
  return (
    <>
      <MenuItem onClick={open}>Open</MenuItem>
      {r.runner && r.mine && (
        <>
          <MenuSeparator />
          {MODES.map((m) => (
            <MenuItem key={m} checked={r.mode === m} disabled={r.master && m === 'off'} onClick={() => setAll([r], m, q)}>
              {MODE_WORD[m]}
            </MenuItem>
          ))}
        </>
      )}
      {!r.master && (
        <>
          <MenuSeparator />
          <MenuItem danger onClick={() => confirmRevoke([r], q.revoke)}>
            Revoke…
          </MenuItem>
        </>
      )}
    </>
  );
}

function Row({
  r,
  q,
  picked,
  pick,
  open,
}: {
  r: DeviceRow;
  q: Devices;
  picked: boolean;
  pick: (on: boolean, shift: boolean) => void;
  open: () => void;
}) {
  const Icon = KIND_ICON[r.kind ?? 'desktop'];
  const busy = [`mode:${r.id}`, `revoke:${r.id}`, `update:${r.id}`].includes(q.pending ?? '');
  return (
    <ContextMenu items={<RowMenu r={r} q={q} open={open} />}>
      <div
        data-device={r.id}
        role="button"
        tabIndex={0}
        onClick={open}
        onKeyDown={(e) => e.key === 'Enter' && e.target === e.currentTarget && open()}
        className={`flex cursor-default items-center gap-3 px-4 py-2.5 text-sm hover:bg-hover/30 ${picked ? 'bg-selected' : ''} ${busy ? 'opacity-50' : ''}`}
      >
        <input
          type="checkbox"
          aria-label={`Select ${r.name}`}
          checked={picked}
          disabled={r.master}
          className={r.master ? 'invisible' : ''}
          onChange={() => undefined}
          onClick={(e) => {
            e.stopPropagation();
            pick(!picked, e.shiftKey);
          }}
        />
        <Icon size={16} role="img" aria-label={KIND_WORD[r.kind ?? 'desktop']} className="shrink-0 text-muted" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate">{r.name}</span>
            {r.here && <span className="shrink-0 rounded-full border border-border px-2 text-xs text-muted">This device</span>}
            {r.master && <span className="shrink-0 rounded-full border border-border px-2 text-xs text-muted">Master</span>}
          </div>
          <Detail r={r} q={q} />
        </div>
        <span className={`w-40 shrink-0 text-xs ${r.stale ? 'text-warning' : ''}`}>
          <RowState r={r} />
        </span>
        {/* Phones and browsers keep the control's room, so the states line up. */}
        <span className="flex w-52 shrink-0 justify-end">
          {r.runner && r.mode && <ModeControl row={r} busy={q.pending !== null} setMode={(m) => q.setMode(r, m)} />}
        </span>
        <ChevronRight size={14} className="shrink-0 text-muted" />
      </div>
    </ContextMenu>
  );
}

/** What holds for every lent device: the battery switch, Auto or Manual for the master's build, and under Expert the reclaim time. */
function AllDevices({ q }: { q: Devices }) {
  return (
    <Block title="Every lent device" icon={<Zap size={16} />}>
      <Setting title="Turn off on battery" icon={<BatteryCharging size={14} />} note="Back to Run when plugged in again.">
        <Toggle
          label="Turn off on battery"
          on={q.battery}
          disabled={q.pending !== null}
          onChange={(on) => void q.setBattery(on).catch(refused)}
        />
      </Setting>
      <Setting title="Take the master's build" icon={<RefreshCw size={14} />} note="A lent Mac on another build restarts on the master's.">
        <Segmented
          label="Take the master's build"
          value={q.buildAuto ? 'auto' : 'manual'}
          options={BUILD_MODES}
          word={(m) => (m === 'auto' ? 'Auto' : 'Manual')}
          onChange={(m) => q.pending === null && (m === 'auto') !== q.buildAuto && void q.setBuildAuto(m === 'auto').catch(refused)}
        />
      </Setting>
      <Expert>
        <Setting title="Hand work over after offline for" icon={<Timer size={14} />}>
          <span className="flex items-center gap-2 text-sm">
            <NumberStepper
              value={Math.round(q.reclaimS / 60)}
              min={1}
              max={1440}
              label="Hand work over after offline for"
              disabled={q.pending !== null}
              onCommit={(m) => q.setReclaimS(m * 60)}
            />
            min
          </span>
        </Setting>
      </Expert>
    </Block>
  );
}

/** The list, or one device's page: `open` is the device Settings was opened on. */
export function DevicesTab({ open: start }: { open?: string }) {
  const q = useDevices();
  const [open, setOpen] = useState<string | null>(start ?? null);
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [showOld, setShowOld] = useState(false);
  // The row clicked last, where a Shift-click's range starts.
  const [anchor, setAnchor] = useState<string | null>(null);
  const page = open ? q.rows.find((r) => r.id === open || (open === MASTER && r.master)) : undefined;
  if (page) return <DevicePage row={page} q={q} back={() => setOpen(null)} />;
  const recent = q.rows.filter((r) => !r.stale);
  const old = q.rows.filter((r) => r.stale);
  const chosen = q.rows.filter((r) => picked.has(r.id));
  const runners = chosen.filter((r) => r.runner && r.mine);
  // The master's box is hidden: a range passing it leaves it out.
  const shown = [...recent, ...(showOld ? old : [])].filter((r) => !r.master).map((r) => r.id);
  // A Shift-click takes the rows from the last one clicked, as every list's boxes do.
  const pick = (id: string, on: boolean, shift: boolean) => {
    const span = shift ? pickRange(shown, anchor, id, (k) => picked.has(k)) : { keys: [id], on };
    setAnchor(id);
    setPicked((s) => {
      const n = new Set(s);
      for (const k of span.keys) {
        if (span.on) n.add(k);
        else n.delete(k);
      }
      return n;
    });
  };
  const list = (items: DeviceRow[]) => (
    <div className="divide-y divide-border rounded-lg border border-border">
      {items.map((r) => (
        <Row
          key={r.id}
          r={r}
          q={q}
          picked={picked.has(r.id)}
          pick={(on, shift) => pick(r.id, on, shift)}
          open={() => (setPicked(new Set()), setOpen(r.id))}
        />
      ))}
    </div>
  );
  const clear = () => setPicked(new Set());
  // A mode is offered only when a picked device runs agents and is yours to set; it then applies to those, counted when not all.
  const part = runners.length < chosen.length ? ` ${runners.length}` : '';
  const bulk = (
    <>
      {runners.length > 0 &&
        MODES.map((m) => (
          <Button key={m} variant="ghost" disabled={q.pending !== null} onClick={() => setAll(runners, m, q, clear)}>
            {MODE_WORD[m]}
            {part}
          </Button>
        ))}
      <Button variant="ghost" disabled={q.pending !== null} onClick={() => confirmRevoke(chosen, q.revoke, clear)}>
        Revoke
      </Button>
    </>
  );
  return (
    <WithSelectionBar count={chosen.length} onClear={clear} actions={bulk} className="flex-1">
      <SettingsPage
        keep="settings-devices"
        room={selectionRoom(chosen.length)}
        title="Devices"
        note="The computers that run agents, and the phones and browsers that connect."
        action={
          <div className="flex shrink-0 items-center gap-2">
            {shell()?.joinCluster && (
              <Button onClick={() => void joinWithLink()}>
                <Link2 size={14} />
                Join with a link
              </Button>
            )}
            <Button variant="primary" onClick={() => void openPairSheet()}>
              <Plus size={14} />
              Connect a device
            </Button>
          </div>
        }
      >
        <SetAsideNote />
        {q.loading && <ListLoading rows={3} label="Loading devices" />}
        {q.error && <p className="text-sm text-danger">Devices did not load.</p>}
        {recent.length > 0 && list(recent)}
        {old.length > 0 && (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-3 text-sm">
              <button
                type="button"
                aria-expanded={showOld}
                onClick={() => setShowOld((v) => !v)}
                className="flex flex-1 items-center gap-2 text-left text-muted hover:text-fg"
              >
                <ChevronRight size={14} className={showOld ? 'rotate-90' : ''} />
                {old.length} not seen for over 30 days
              </button>
              <Button disabled={q.pending !== null} onClick={() => confirmRevoke(old, q.revoke)}>
                Revoke all {old.length}
              </Button>
            </div>
            {showOld && list(old)}
          </div>
        )}
        <AgentPlacement rows={q.rows} />
        <AllDevices q={q} />
      </SettingsPage>
    </WithSelectionBar>
  );
}

/** A board set aside: which machine was the master until when, and where the board is now. */
function SetAsideNote() {
  const a = useSetAside();
  const key = a?.key;
  useEffect(() => {
    if (key) markSetAsideSeen(key);
  }, [key]);
  if (!a) return null;
  const until = new Date(a.since).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  return (
    <p role="note" className="flex items-start gap-2 rounded-md border border-border px-3 py-2 text-sm text-fg">
      <ArchiveX size={16} className="mt-0.5 shrink-0 text-warning" />
      <span>
        {a.name} was the master until {until}. Its board was set aside{a.master ? `; the board is now on ${a.master}` : ''}.
      </span>
    </p>
  );
}
