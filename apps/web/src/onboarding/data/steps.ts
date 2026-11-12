// Onboarding's five steps, each done by what is true on the machine, not by having walked through it: Settings ticks them too.
import type { Account, Methods, VendorStatus } from '@handline/client';

export const STEPS = ['ai', 'tools', 'notify', 'project', 'agent'] as const;
export type StepId = (typeof STEPS)[number];

/** The tools every agent needs, whatever AI tool it runs; an AI tool's own command is installed from its row on the AI step. */
export const BASE_TOOLS: readonly string[] = ['git', 'tmux'];

export type Facts = {
  accounts: Pick<Account, 'login_state'>[];
  missingTools: { tool: string }[];
  /** The browser's notification permission, or 'unsupported'. */
  notify: NotificationPermission | 'unsupported';
  /** The person chose Skip on the notifications step; notifications are optional, so that ticks it. */
  notifySkipped: boolean;
  repos: number;
  agents: number;
};

export function stepsDone(f: Facts): Record<StepId, boolean> {
  return {
    ai: f.accounts.some((a) => a.login_state === 'ok'),
    tools: !f.missingTools.some((m) => BASE_TOOLS.includes(m.tool)),
    notify: f.notify === 'granted' || f.notify === 'unsupported' || f.notifySkipped,
    project: f.repos > 0,
    agent: f.agents > 0,
  };
}

/** Where Continue opens: the step the person left at when it is still not done, else the first step not done after it, else the first not done. */
export function resumeAt(done: Record<StepId, boolean>, left: number): number {
  const at = Math.min(Math.max(left, 0), STEPS.length - 1);
  if (!done[STEPS[at]!]) return at;
  const order = [...STEPS.keys()].slice(at + 1).concat([...STEPS.keys()].slice(0, at));
  return order.find((i) => !done[STEPS[i]!]) ?? at;
}

/** ready: logged in; logging_in: a browser tab or device code is waiting; found: a login outside Handline on this Mac; none: nothing yet. */
export type AiState = 'ready' | 'logging_in' | 'found' | 'none';

export type AiRow = {
  vendor: string;
  state: AiState;
  /** The account that is ready or logging in, else one made earlier that is not logged in: a login goes to it. */
  account: string | null;
  /** The logged-in (or logging-in) account's email. */
  email: string | null;
  /** The found login's folder, as account.adopt takes it. */
  source: string | null;
  /** codex: the found login is used as it is; claude: it logs in once. */
  adoptable: boolean;
  installed: boolean;
  /** The command that installs the vendor's binary. */
  fix: string;
};

type Found = Methods['accounts.discover']['result']['found'][number];
type AccountFacts = Pick<Account, 'name' | 'vendor' | 'login_state' | 'email' | 'login'>;

/** The vendors this screen offers, in order. */
export const AI_VENDORS: readonly string[] = ['claude', 'codex'];

/** One row per vendor the daemon can run and this screen offers: its account if one exists, else a login found on this Mac. */
export function aiRows(vendors: VendorStatus[], found: Found[], accounts: AccountFacts[]): AiRow[] {
  return AI_VENDORS.flatMap((vendor) => {
    const v = vendors.find((x) => x.vendor === vendor);
    if (!v) return [];
    const mine = accounts.filter((a) => a.vendor === vendor);
    const ready = mine.find((a) => a.login_state === 'ok');
    const pending = mine.find((a) => a.login?.state === 'waiting' || a.login?.state === 'code_sent');
    const f = found.find((x) => x.vendor === vendor && x.logged_in !== false);
    const state: AiState = ready ? 'ready' : pending ? 'logging_in' : f ? 'found' : 'none';
    const account = ready ?? pending ?? mine[0];
    return [
      {
        vendor,
        state,
        account: account?.name ?? null,
        email: (ready ?? pending)?.email ?? null,
        source: f?.source ?? null,
        adoptable: f?.adoptable ?? false,
        installed: v.installed,
        fix: v.fix,
      },
    ];
  });
}
