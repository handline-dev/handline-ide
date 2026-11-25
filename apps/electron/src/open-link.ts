// handline:// links: the link page's Open in the Handline app opens the app with one (macOS open-url). Each goes to the page as a
// navigate to `join`, which asks how this Mac joins; one that comes before the window can take it waits for it.
import { isTailscaleHost } from '@handline/client';

/** A pairing link, as the link page makes it: the link page is served only on a Tailscale name, so a link naming any other host came
 * from somewhere else (any web page can open a handline:// link) and is not for the app. */
export function isPairLink(url: string): boolean {
  if (url.length > 2048 || !/^handline:\/\/pair\?/.test(url)) return false;
  const host = new URL(url).searchParams.get('host');
  return !!host && isTailscaleHost(host);
}

export class OpenLinks {
  private waiting: string[] = [];
  private to: ((url: string) => void) | null = null;
  add(url: string): void {
    if (!isPairLink(url)) return;
    if (this.to) this.to(url);
    else this.waiting.push(url);
  }
  /** From now on each link goes to `to`, the ones that waited first. */
  serve(to: (url: string) => void): void {
    this.to = to;
    for (const u of this.waiting.splice(0)) to(u);
  }
}
