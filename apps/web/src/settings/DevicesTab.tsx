// Settings > Devices: every device in one flat list, the master first; a row opens its page. At the foot, the battery switch, Auto or
// Manual for a lent Mac taking the master's build, and the reclaim time.
import { useNameOf } from '@handline/app-data';
import { ListLoading } from '#src/ui/Loading.tsx';
import {
  ArchiveX,
  ArrowUpCircle,
  BatteryCharging,
  ChevronRight,
  Plus,
  RefreshCw,
  SlidersHorizontal,
  Timer,
  TriangleAlert,
  Zap,
} from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { DevicePage } from '#src/settings/DevicePage.tsx';
import {
  Confirm,
  KIND_ICON,
  KIND_WORD,
  MODE_WORD,
  ModeControl,
  OFF_TEXT,
  StateWord,
  confirmRevoke,
  refused,
} from '#src/settings/DeviceControls.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { openPairSheet } from '#src/settings/PairSheet.tsx';
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
import { Button } from '#src/ui/Button.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { openModal } from '#src/ui/modal.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';

const BUILD_MODES = ['auto', 'manual'] as const;

type Devices = ReturnType<typeof useDevices>;

/** Sets each row's mode; Off asks once for all of them. */
function setAll(rows: DeviceRow[], mode: Mode, q: Devices) {
  const go = async () => {
    for (const r of rows) await q.setMode(r, mode);
  };
  if (mode !== 'off') return void go().catch(refused);
  void openModal<void>((close) => (
    <Confirm
      title={rows.length === 1 ? `Turn ${rows[0]!.name} off?` : `Turn ${rows.length} devices off?`}
      text={OFF_TEXT}
      act="Turn off"
      busyWord="Turning off…"
      go={go}
      close={() => close()}
    />
  ));
}

/** What the row says under its name: its kind, its agents, its memory cap, and what it lacks for an agent, or that it is on another build. */
function Detail({ r, q }: { r: DeviceRow; q: Devices }) {
  const memoryMax = q.memoryMax;
  const nameOf = useNameOf();
  const kind = r.kind ? KIND_WORD[r.kind] : null;
  const cap = r.node?.memory_max_gb ?? memoryMax;
  const parts = r.runner
    ? [kind, `${r.agents.length} ${r.agents.length === 1 ? 'agent' : 'agents'}`, cap ? `up to ${cap} GB` : null]
    : [kind, 'runs no agents'];
  const lack = r.lacks[0];
  const why = r.lacks.map((l) => `${nameOf(l.agent)}: ${l.why}`).join('\n');
  return (
    <div className="truncate text-xs text-muted">
      {parts.filter(Boolean).join(' · ')}
      {r.behind ? (
        <span title={why}>
          {' · '}
          {r.updating ? (
            'updating…'
          ) : (
            <>
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
            </>
          )}
        </span>
      ) : r.appReady ? (
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
      ) : (
        lack && (
          <span className="text-warning" title={why}>
            {' · '}cannot run {nameOf(lack.agent)}: {lack.why}
            {r.lacks.length > 1 && ` (+${r.lacks.length - 1})`}
          </span>
        )
      )}
    </div>
  );
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

function Row({ r, q, picked, pick, open }: { r: DeviceRow; q: Devices; picked: boolean; pick: (on: boolean) => void; open: () => void }) {
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
          onClick={(e) => e.stopPropagation()}
          onChange={(e) => pick(e.target.checked)}
        />
        <Icon size={16} className="shrink-0 text-muted" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate">{r.name}</span>
            {r.here && <span className="shrink-0 rounded-full border border-border px-2 text-xs text-muted">This device</span>}
            {r.master && <span className="shrink-0 rounded-full border border-border px-2 text-xs text-muted">Master</span>}
          </div>
          <Detail r={r} q={q} />
        </div>
        <span className={`w-40 shrink-0 text-xs ${r.stale ? 'text-warning' : ''}`}>
          <StateWord row={r} />
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
  const [expert, setExpert] = useState(false);
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
        <Switch
          label="Take the master's build"
          value={q.buildAuto ? 'auto' : 'manual'}
          options={BUILD_MODES}
          word={(m) => (m === 'auto' ? 'Auto' : 'Manual')}
          onChange={(m) => q.pending === null && (m === 'auto') !== q.buildAuto && void q.setBuildAuto(m === 'auto').catch(refused)}
        />
      </Setting>
      <button
        type="button"
        aria-expanded={expert}
        onClick={() => setExpert((v) => !v)}
        className="flex w-full items-center gap-2 border-t border-border pt-3 text-sm text-muted hover:text-fg"
      >
        <ChevronRight size={14} className={expert ? 'rotate-90' : ''} />
        <SlidersHorizontal size={14} />
        Expert
      </button>
      {expert && (
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
      )}
    </Block>
  );
}

/** The list, or one device's page: `open` is the device Settings was opened on. */
export function DevicesTab({ open: start }: { open?: string }) {
  const q = useDevices();
  const [open, setOpen] = useState<string | null>(start ?? null);
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [showOld, setShowOld] = useState(false);
  const page = open ? q.rows.find((r) => r.id === open || (open === MASTER && r.master)) : undefined;
  if (page) return <DevicePage row={page} q={q} back={() => setOpen(null)} />;
  const recent = q.rows.filter((r) => !r.stale);
  const old = q.rows.filter((r) => r.stale);
  const chosen = q.rows.filter((r) => picked.has(r.id));
  const runners = chosen.filter((r) => r.runner && r.mine);
  const pick = (id: string, on: boolean) =>
    setPicked((s) => {
      const n = new Set(s);
      if (on) n.add(id);
      else n.delete(id);
      return n;
    });
  const list = (items: DeviceRow[]) => (
    <div className="divide-y divide-border rounded-lg border border-border">
      {items.map((r) => (
        <Row key={r.id} r={r} q={q} picked={picked.has(r.id)} pick={(on) => pick(r.id, on)} open={() => setOpen(r.id)} />
      ))}
    </div>
  );
  const bulk = (
    <div className="flex items-center gap-2" role="region" aria-label="Selection">
      <span className="text-sm text-fg">{chosen.length} selected</span>
      {MODES.map((m) => (
        <Button key={m} disabled={runners.length === 0 || q.pending !== null} onClick={() => setAll(runners, m, q)}>
          {MODE_WORD[m]}
        </Button>
      ))}
      <Button variant="danger" disabled={q.pending !== null} onClick={() => confirmRevoke(chosen, q.revoke)}>
        Revoke
      </Button>
      <Button onClick={() => setPicked(new Set())}>Cancel</Button>
    </div>
  );
  return (
    <SettingsPage
      keep="settings-devices"
      title="Devices"
      note="The Macs and servers that run agents, and the phones and browsers that connect."
      action={
        chosen.length > 0 ? (
          bulk
        ) : (
          <Button variant="primary" onClick={() => void openPairSheet()}>
            <Plus size={14} />
            Connect a device
          </Button>
        )
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
      <AllDevices q={q} />
    </SettingsPage>
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
