// Runs in the sandboxed window before the page: the only thing the page gets from the shell is window.handline, and the device token
// the shell paired for it, put where the page keeps one (the page reads it from its own storage, as after a manual pairing).
import { contextBridge, ipcRenderer, webFrame } from 'electron';
import { makeApi } from '#src/api.ts';
import { CHANNELS } from '#src/bridge.ts';
import type { Grant } from '#src/grant.ts';

contextBridge.exposeInMainWorld('handline', {
  ...makeApi(ipcRenderer, () => webFrame.getZoomFactor()),
});
try {
  const given = ipcRenderer.sendSync(CHANNELS.token) as Grant | null;
  const daemon = new URLSearchParams(location.search).get('daemon');
  if (given?.token && daemon) localStorage.setItem(`token:${daemon}`, given.token);
  // The one host the page may reach over plain ws besides loopback: this machine's own Tailscale address.
  localStorage.setItem('shell:own-host', given?.ownHost ?? '');
  localStorage.setItem('shell:daemon-failure', given?.failure ?? '');
  localStorage.setItem('shell:token-pending', given?.pending ? '1' : '');
} catch {
  /* no token: the page shows its pairing screen */
}
// A token the shell gets later is kept where the page keeps one, before the page hears of it (Connect's onToken).
ipcRenderer.on(CHANNELS.deviceToken, (_e, t) => {
  const daemon = new URLSearchParams(location.search).get('daemon');
  if (typeof t !== 'string') return;
  localStorage.setItem('shell:token-pending', ''); // the shell has spoken: a reload does not wait again
  if (t && daemon) localStorage.setItem(`token:${daemon}`, t);
});
