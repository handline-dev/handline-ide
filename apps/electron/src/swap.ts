// Puts a new Handline.app in place of the installed one: the old is renamed aside, the new takes its path, and the old one is put back if that fails.
// A rename inside one folder is atomic, and a running app keeps working through the files it has open, so the app can swap itself.
import fs from 'node:fs';

/** Removes a folder that holds an app bundle. Electron's fs reads an .asar file as a folder, so a plain recursive rm calls rmdir on it
 *  and fails with ENOTDIR; with asar off it is a file like any other. */
export function rmBundle(p: string): void {
  const was = process.noAsar;
  process.noAsar = true;
  try {
    fs.rmSync(p, { recursive: true, force: true });
  } finally {
    process.noAsar = was;
  }
}

/** Returns the path the old app was moved to; the caller removes it (now, or at the next start: the running process may still need it). */
export function swapBundle(installed: string, staged: string, aside = `${installed}.old-${process.pid}`): string {
  rmBundle(aside);
  fs.renameSync(installed, aside);
  try {
    fs.renameSync(staged, installed);
  } catch (e) {
    fs.renameSync(aside, installed);
    throw e;
  }
  return aside;
}

/** The folders a swap leaves beside the app (`Handline.app.old-<pid>`, the staging folder): removed at the next start. */
export function leftovers(parent: string, names: string[], app = 'Handline.app'): string[] {
  return names.filter((n) => n.startsWith(`${app}.old-`) || n === STAGING).map((n) => `${parent}/${n}`);
}

/** Where an update is unpacked: beside the installed app, so the swap is a rename and not a copy across volumes. */
export const STAGING = '.Handline-update';
