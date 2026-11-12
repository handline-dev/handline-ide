// Onboarding's progress and whether its modal is open. Done-ness comes from the daemon (steps.ts); this browser keeps only that the modal
// has opened once, the step it was closed on, a skipped notifications step and a hidden sidebar card.
import { useAccounts, useAgents, useFoundLogins, useRepos, useStatus, useWrite } from '@handline/app-data';
import { useEffect, useRef, useSyncExternalStore } from 'react';
import { read, write } from '#src/persist/kv.ts';
import { aiRows, resumeAt, STEPS, stepsDone } from '#src/onboarding/data/steps.ts';

type Kept = { opened: boolean; step: number; notifySkipped: boolean; cardHidden: boolean };
const KEY = 'onboarding';
const FRESH: Kept = { opened: false, step: 0, notifySkipped: false, cardHidden: false };
const clamp = (step: number) => Math.min(Math.max(step, 0), STEPS.length - 1);
const permission = (): NotificationPermission | 'unsupported' =>
  typeof Notification === 'undefined' ? 'unsupported' : Notification.permission;

let kept: Kept = { ...FRESH, ...read<Partial<Kept>>(KEY, {}) };
let open = false;
let notify = permission();
const listeners = new Set<() => void>();
const emit = () => {
  for (const l of listeners) l();
};
const keep = (patch: Partial<Kept>) => {
  kept = { ...kept, ...patch };
  write(KEY, kept);
  emit();
};
const subscribe = (l: () => void) => (listeners.add(l), () => listeners.delete(l));

// The permission can change outside the page (the system settings); where the browser reports it, every screen follows.
void navigator.permissions
  ?.query({ name: 'notifications' })
  .then((p) => {
    p.onchange = () => {
      notify = permission();
      emit();
    };
  })
  .catch(() => {});

/** Opens the modal at `step`, or where the person left it. */
export const openOnboarding = (step?: number) => {
  open = true;
  keep({ opened: true, ...(step !== undefined && { step: clamp(step) }) });
};
/** Closes the modal; its step is kept for Continue. */
export const closeOnboarding = () => {
  open = false;
  emit();
};
export const useOnboardingOpen = () => useSyncExternalStore(subscribe, () => open);
const useKept = () => useSyncExternalStore(subscribe, () => kept);
const useNotify = () => useSyncExternalStore(subscribe, () => notify);

export const resetOnboardingForTest = () => {
  kept = { ...FRESH };
  open = false;
  notify = permission();
  write(KEY, undefined);
  emit();
};

export function useOnboarding() {
  const k = useKept();
  const perm = useNotify();
  const status = useStatus();
  const accounts = useAccounts();
  const repos = useRepos();
  const agents = useAgents();
  const loaded = status.isSuccess && accounts.isSuccess && repos.isSuccess && agents.isSuccess;
  const done = stepsDone({
    accounts: accounts.data?.accounts ?? [],
    missingTools: status.data?.missing_tools ?? [],
    notify: perm,
    notifySkipped: k.notifySkipped,
    repos: repos.data?.repos.length ?? 0,
    agents: agents.data?.agents.length ?? 0,
  });
  const doneCount = STEPS.filter((s) => done[s]).length;
  const allDone = doneCount === STEPS.length;
  const resumeStep = resumeAt(done, k.step);
  // Opens by itself once; an install that already has agents is not walked through it.
  const fresh = loaded && !k.opened && !allDone && !agents.data?.agents.length;
  useEffect(() => {
    if (fresh) openOnboarding(resumeStep);
  }, [fresh, resumeStep]);
  return {
    loaded,
    steps: STEPS.map((id) => ({ id, done: done[id] })),
    done,
    doneCount,
    total: STEPS.length,
    allDone,
    /** The step the modal shows. */
    step: k.step,
    /** The step Continue opens. */
    resumeStep,
    goTo: (step: number) => keep({ step: clamp(step) }),
    cardVisible: loaded && k.opened && !allDone && !k.cardHidden,
    hideCard: () => keep({ cardHidden: true }),
    notify: perm,
    askNotify: async () => {
      if (typeof Notification === 'undefined') return;
      try {
        await Notification.requestPermission();
      } finally {
        notify = permission();
        emit();
      }
    },
    skipNotify: () => keep({ notifySkipped: true }),
  };
}

/** The AI step: a row per vendor and the calls its button makes. */
export function useAiTools() {
  const status = useStatus();
  const accounts = useAccounts();
  const found = useFoundLogins();
  const adopt = useWrite('account.adopt');
  const add = useWrite('account.add');
  const login = useWrite('account.login');
  const rows = aiRows(status.data?.vendors ?? [], found.data ?? [], accounts.data?.accounts ?? []);
  // An account made here before its login failed, until the list shows it: a retry logs it in instead of making another.
  const made = useRef<Record<string, string>>({});
  const busy = adopt.isPending || add.isPending || login.isPending;
  return {
    loading: !status.isSuccess || !accounts.isSuccess || found.isLoading,
    rows,
    /** True while a call below is in flight; the buttons wait for it. */
    busy,
    /** Uses a codex login found on this Mac as it is; the new account is logged in when this resolves. The daemon names it. */
    adopt: (vendor: string, source: string) => adopt.mutateAsync({ vendor, source }).then((r) => r.account),
    /** Starts `vendor`'s login on its account, made first when it has none: the URL to open and, for codex, the code to type there. */
    logIn: async (vendor: string) => {
      const row = rows.find((r) => r.vendor === vendor);
      if (busy || row?.state === 'logging_in' || row?.state === 'ready') return null;
      const account = row?.account ?? made.current[vendor] ?? (made.current[vendor] = (await add.mutateAsync({ vendor })).account);
      const started = await login.mutateAsync({ account });
      return { account, url: started.url, deviceCode: started.device_code ?? null };
    },
  };
}
