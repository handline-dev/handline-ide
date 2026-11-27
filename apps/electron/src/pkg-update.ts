// The installed app's own update: it reads latest-mac.yml (latest-x64-mac.yml for an Intel build) at the address the build was made for (the rolling release of .github/workflows/mac-release.yml),
// downloads the newer zip, checks it, prepares the new Handline.app beside the old one and offers it as the one update notice (update-flow.ts).
// "Restart now" swaps the two (swap.ts) and starts the new one. This is the whole of what an updater does here, behind PkgUpdater, so that
// electron-updater can take its place once a Developer ID is there (Squirrel.Mac refuses an ad-hoc signed update).
import type { UpdateFlow } from '#src/update-flow.ts';

export interface PkgUpdater {
  check(): Promise<void>;
}

export interface Latest {
  version: string;
  /** The zip, relative to the address the yml is at. */
  path: string;
  /** Base64, as electron-builder writes it. */
  sha512: string;
}

/** The three top-level keys of latest-mac.yml that matter (the file is flat enough that a YAML parser would only be weight). Null when one is missing. */
export function parseLatest(yml: string): Latest | null {
  const top = (key: string) => new RegExp(`^${key}:\\s*(.+?)\\s*$`, 'm').exec(yml)?.[1]?.replace(/^['"]|['"]$/g, '');
  const version = top('version');
  const path = top('path');
  const sha512 = top('sha512');
  return version && path && sha512 ? { version, path, sha512 } : null;
}

/** The file an app of this architecture reads: Apple silicon's keeps the name every build had, Intel's has its own. */
export const latestFile = (arch: 'arm64' | 'x64'): string => (arch === 'x64' ? 'latest-x64-mac.yml' : 'latest-mac.yml');

const nums = (v: string) => (v.split('-')[0] ?? '').split('.').map((n) => Number(n) || 0);

/** Whether `candidate` is a later version than `current`; a pre-release suffix ("0.0.0-dev") is earlier than the same numbers without one. */
export function isNewer(candidate: string, current: string): boolean {
  const a = nums(candidate);
  const b = nums(current);
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const d = (a[i] ?? 0) - (b[i] ?? 0);
    if (d !== 0) return d > 0;
  }
  return current.includes('-') && !candidate.includes('-');
}

/** The release the app reads, and whether the person moved it off the channel the build was made for. */
export interface Where {
  base: string;
  switched: boolean;
}

export interface PkgPorts {
  /** The version of the app that runs. */
  current(): string;
  /** The architecture of the app that runs: the file it reads and the build it takes are its own. */
  arch(): 'arm64' | 'x64';
  text(url: string): Promise<string>;
  /** Saves the file; `progress` is called with 0 to 1. */
  download(url: string, progress: (fraction: number) => void): Promise<string>;
  sha512(file: string): Promise<string>;
  /** Unpacks the zip beside the installed app and returns the new Handline.app's path. */
  stage(zip: string): Promise<string>;
  /** True when nothing on screen would be lost by asking. */
  idle(): Promise<boolean>;
  /** Puts the staged app in place of the installed one, moves the daemon onto it and starts it. Rejects with the reason, and the old app is still in place then. */
  swapAndRestart(staged: string): Promise<void>;
}

/** Offers a newer build once, when the app is idle: downloaded, checked and unpacked before the person is asked. */
export class SelfUpdater implements PkgUpdater {
  private busy = false;
  /** Restart now was pressed and has not failed: a second press must not swap again, which would remove the only copy of the old app. */
  private going = false;
  private ports: PkgPorts;
  private flow: UpdateFlow;
  /** The channel the person is on, asked at each check: it can change between two. `switched` is true when it is not the build's own. */
  private where: () => Where;
  constructor(ports: PkgPorts, flow: UpdateFlow, where: () => Where) {
    this.ports = ports;
    this.flow = flow;
    this.where = where;
  }
  private base = () => this.where().base.replace(/\/+$/, '');

  async check(): Promise<void> {
    this.flow.remind();
    const phase = this.flow.view.phase;
    if (this.busy || (phase !== 'idle' && phase !== 'failed')) return; // told already, or being prepared
    this.busy = true;
    try {
      const latest = await this.read();
      if (!latest || !(await this.ports.idle())) return; // owed: the next check offers again
      await this.prepare(latest, true);
    } catch {
      /* offline, or the release is being replaced: the next check tries again */
    } finally {
      this.busy = false;
    }
  }

  /** The release as it is now, or null when it offers nothing newer. */
  private async read(): Promise<Latest | null> {
    const latest = parseLatest(await this.ports.text(`${this.base()}/${latestFile(this.ports.arch())}`));
    // Another channel's builds are not ordered with this one's (Canary's numbers run ahead of Stable's): any other version is the update.
    const current = this.ports.current();
    const next = this.where().switched ? latest?.version !== current : latest && isNewer(latest.version, current);
    return latest && next ? latest : null;
  }

  /** `reread`: a zip gone from the release (its channel moved on) reads the release once more and takes what it names. */
  private async prepare(latest: Latest, reread: boolean): Promise<void> {
    try {
      this.flow.preparing('Downloading', 0);
      let zip: string;
      try {
        zip = await this.ports.download(`${this.base()}/${latest.path}`, (f) => this.flow.preparing('Downloading', f));
      } catch (e) {
        const now = reread && /\b404\b/.test(String(e instanceof Error ? e.message : e)) ? await this.read() : null;
        if (!now || now.path === latest.path) throw e;
        return await this.prepare(now, false);
      }
      this.flow.preparing('Checking', 1);
      if ((await this.ports.sha512(zip)) !== latest.sha512) throw new Error('the download does not match its checksum');
      const staged = await this.ports.stage(zip);
      this.flow.ready(() => void this.go(staged), latest.version);
    } catch (e) {
      this.flow.failed(e instanceof Error ? e.message : String(e), () => void this.retry());
    }
  }

  /** Try again, by the button or the timer: the release is read again, so a channel that moved on is followed. */
  private async retry(): Promise<void> {
    if (this.busy) return;
    this.busy = true;
    try {
      const latest = await this.read();
      if (!latest) return this.flow.idle();
      await this.prepare(latest, true);
    } catch (e) {
      this.flow.failed(e instanceof Error ? e.message : String(e), () => void this.retry());
    } finally {
      this.busy = false;
    }
  }

  private async go(staged: string): Promise<void> {
    if (this.going) return;
    this.going = true;
    try {
      await this.ports.swapAndRestart(staged);
    } catch (e) {
      this.going = false;
      this.flow.failed(e instanceof Error ? e.message : String(e), () => void this.go(staged));
    }
  }
}
