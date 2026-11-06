// The launchd job an older Handline installed (`hl service`, label dev.handline.daemon) keeps a daemon running, and would start one again
// under the app's own. The app is now the only thing that runs the live home's daemon, so at launch it unloads that job and removes its file.
import { execFile } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

export const OLD_LABEL = 'dev.handline.daemon';

export interface OldServiceDeps {
  /** The person's home folder, where Library/LaunchAgents is. */
  userHome: string;
  uid: number;
  /** Runs launchctl; a failure (the job was not loaded) is not an error here. */
  launchctl: (args: string[]) => Promise<void>;
}

const launchctl = (args: string[]): Promise<void> =>
  new Promise((resolve) => execFile('launchctl', args, { timeout: 30_000 }, () => resolve()));

/** Unloads and removes the old job when its file is there; true when there was one. */
export async function removeOldService(
  d: OldServiceDeps = { userHome: os.homedir(), uid: process.getuid?.() ?? 0, launchctl },
): Promise<boolean> {
  const plist = path.join(d.userHome, 'Library', 'LaunchAgents', `${OLD_LABEL}.plist`);
  if (!fs.existsSync(plist)) return false;
  await d.launchctl(['bootout', `gui/${d.uid}/${OLD_LABEL}`]);
  fs.rmSync(plist, { force: true });
  return true;
}
