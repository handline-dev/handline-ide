import { useAgentDetail, useAgentStrip, useServes, useWrite } from '@handline/app-data';
import { FoldVertical } from 'lucide-react';
import { useState } from 'react';
import { CardAction, MenuFact } from '#src/chat/agent/menuParts.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { useNow } from '#src/format/time.ts';
import { COMPACTING_WORDS, useCompactNow, useCompacting } from '#src/chat/agent/compaction.ts';

const COMPACT_K = [100, 150, 200, 300];
const tokens = (n: number) => (n >= 1_000_000 ? `${n / 1_000_000}M` : n >= 1000 ? `${Math.round(n / 1000)}k` : String(n));

/** The agent's context in the message box: a ring of how full it is toward the point where it compacts (its context_k; the
 * model's window when it has none). The ring turns colour only past that point, when this turn's end compacts it. Resting on it
 * shows the tokens against that point and the prompt cache's time left, with Compact now and the point to choose; a click keeps
 * the card open. */
export function ContextRing({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  const strip = useAgentStrip(agent).data;
  const compactNow = useCompactNow(agent);
  const now = useNow(30_000);
  const compacting = useCompacting(agent);
  if (!detail) return null;
  const used = strip?.ctx_used ?? null;
  const limit = strip?.ctx_limit ?? null;
  const point = (Number(detail.settings?.context_k) || 0) * 1000 || null;
  // Full toward the compaction point when both are known; else the share of the window the daemon reports.
  const pct = used != null && (point ?? limit) ? (used / (point ?? limit)!) * 100 : (detail.ctx_pct ?? strip?.ctx_pct ?? null);
  if (pct == null) return null;
  const over = point != null && used != null && used > point;
  const cache = detail.cache_until ? Math.round((Date.parse(detail.cache_until) - now) / 60_000) : null;
  const r = 6;
  const round = 2 * Math.PI * r;
  return (
    <HoverCard
      pin
      align="end"
      trigger={
        <button
          type="button"
          className="flex h-7 items-center gap-1 rounded-md px-1.5 text-xs text-muted outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
        >
          <svg viewBox="0 0 16 16" className="size-4 -rotate-90" aria-hidden>
            <circle cx="8" cy="8" r={r} fill="none" strokeWidth="2" className="stroke-border" />
            <circle
              cx="8"
              cy="8"
              r={r}
              fill="none"
              strokeWidth="2"
              strokeLinecap="round"
              strokeDasharray={round}
              strokeDashoffset={round * (1 - Math.min(100, pct) / 100)}
              className={over ? 'stroke-warning' : 'stroke-muted'}
            />
          </svg>
          <span className={`tabular-nums ${over ? 'text-warning' : ''}`}>{`${Math.round(pct)}%`}</span>
        </button>
      }
    >
      <div className="flex min-w-64 flex-col gap-2 px-2 pt-1 pb-2">
        {over && <p className="text-xs text-warning">Compacts when this turn ends</p>}
        {used != null && <ContextLine used={used} point={point} limit={limit} over={over} />}
      </div>
      {cache != null && cache > 0 && <MenuFact name="Prompt cache">{`${cache}m left`}</MenuFact>}
      <div className="my-1 h-px bg-border" />
      <CompactAt agent={agent} point={point} />
      {compacting ? (
        <p className="flex h-7 items-center gap-2 px-2 text-sm text-muted">
          <FoldVertical size={14} className="animate-pulse" />
          {COMPACTING_WORDS[compacting]}
        </p>
      ) : (
        compactNow && (
          <CardAction icon={<FoldVertical size={14} />} onClick={compactNow}>
            Compact now
          </CardAction>
        )
      )}
    </HoverCard>
  );
}

/** Tokens used of the compaction point as a bar that is full at that point, and the words: used of the point, and as a share of it. */
function ContextLine({ used, point, limit, over }: { used: number; point: number | null; limit: number | null; over: boolean }) {
  const scale = point ?? limit ?? used;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex justify-between gap-6 text-xs">
        <span className="text-muted">Context</span>
        <span className="text-fg tabular-nums">
          {point ? `${tokens(used)} / ${tokens(point)}` : tokens(used)}
          {point && <span className="text-muted">{` · ${Math.round((used / point) * 100)}%`}</span>}
        </span>
      </div>
      <div className="h-1.5 rounded-full bg-border">
        <div
          className={`h-full rounded-full ${over ? 'bg-warning' : 'bg-accent'}`}
          style={{ width: `${scale ? Math.min(100, (used / scale) * 100) : 0}%` }}
        />
      </div>
    </div>
  );
}

/** Where it compacts, as a setting: the usual points side by side, the one in force selected (another point in force gets its own
 * segment); a click sets it. Other… is always last: a click turns it into a field, set on Enter or on leaving it, Esc leaves. */
function CompactAt({ agent, point }: { agent: string; point: number | null }) {
  const set = useWrite('agent.set', agent);
  const served = useServes('agent.set').served;
  const chosen = point ? point / 1000 : null;
  const custom = chosen != null && !COMPACT_K.includes(chosen);
  const [typed, setTyped] = useState<string | null>(null);
  if (!served) return null;
  const apply = (k: number) => k !== chosen && set.mutate({ agent, values: { context_k: String(k) } });
  const take = () => {
    const k = Math.round(Number(typed));
    if (k >= 20 && k <= 2000) apply(k);
    setTyped(null);
  };
  const seg =
    'h-6 rounded-sm px-2 text-xs text-muted tabular-nums outline-none hover:text-fg focus-visible:ring-2 focus-visible:ring-accent aria-checked:bg-bg aria-checked:text-fg aria-checked:shadow-popover';
  return (
    // The name above its choices: side by side the last one (another point) was cut off at the card's edge.
    <div className="flex flex-col gap-1.5 px-2 py-1">
      <span className="text-xs text-muted">Compact at</span>
      <div role="radiogroup" aria-label="Compact at" className="flex gap-0.5 rounded-md bg-surface p-0.5">
        {COMPACT_K.map((k) => (
          <button key={k} type="button" role="radio" aria-checked={k === chosen} onClick={() => apply(k)} className={seg}>
            {tokens(k * 1000)}
          </button>
        ))}
        {/* A point in force that is not one of the usual: its own segment, selected. */}
        {custom && (
          <button type="button" role="radio" aria-checked onClick={() => undefined} className={seg}>
            {tokens(chosen! * 1000)}
          </button>
        )}
        {/* Any other point, always there: a click turns it into a field. */}
        {typed == null ? (
          <button type="button" aria-label="Other point" onClick={() => setTyped(custom ? String(chosen) : '')} className={seg}>
            Other…
          </button>
        ) : (
          <label className={`flex items-center bg-bg text-fg shadow-popover ${seg}`}>
            <input
              autoFocus
              inputMode="numeric"
              aria-label="Compact at, in thousands of tokens"
              value={typed}
              onFocus={(e) => e.target.select()}
              onChange={(e) => setTyped(e.target.value.replace(/\D/g, ''))}
              onKeyDown={(e) => {
                if (e.key === 'Enter') take();
                if (e.key === 'Escape') {
                  e.stopPropagation();
                  setTyped(null);
                }
              }}
              onBlur={take}
              className="w-8 bg-transparent text-right outline-none"
            />
            k
          </label>
        )}
      </div>
    </div>
  );
}
