// Each agent's own values for the per-agent settings keys (settings.set with `agent`); null takes one back to the shared value.
export class Overrides {
  private byAgent = new Map<string, Record<string, unknown>>();
  of(agent: string): Record<string, unknown> {
    return this.byAgent.get(agent) ?? {};
  }
  /** settings.get: with `agent`, that agent's own values beside the shared ones. */
  read<S extends object>(settings: S, agent: unknown): S {
    return typeof agent === 'string' ? { ...settings, overrides: this.of(agent) } : settings;
  }
  /** settings.set: with `agent`, keeps the changes as its own and says so; without, leaves them to the shared values. */
  set(agent: unknown, changes: unknown): boolean {
    if (typeof agent !== 'string') return false;
    const own: Record<string, unknown> = { ...this.of(agent), ...(changes as object) };
    for (const [k, v] of Object.entries(own)) if (v === null) delete own[k];
    this.byAgent.set(agent, own);
    return true;
  }
}
