// The pairing codes and device tokens of the fake daemon: `device.pair` issues a code, `device.redeem` trades it for a token.
import crypto from 'node:crypto';
import { badRequest, refused } from '#src/errors.ts';

/** The fake's self-made certificate's fingerprint. */
const FAKE_FP = 'f'.repeat(64);

type Accepted = (said: string, extra: Record<string, unknown>) => unknown;

export class FakePairing {
  private codes = new Set<string>();
  private tokens = new Map<string, { id: string; name: string }>();

  /** What `device.pair` gives the founder: a one-time code to redeem. */
  issueCode(): string {
    const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // the Go daemon's, so a code looks the same
    const code = Array.from(crypto.randomBytes(12), (b, i) => (i && i % 4 === 0 ? '-' : '') + alphabet[b % alphabet.length]).join('');
    this.codes.add(code);
    return code;
  }

  knows(token: unknown): boolean {
    return typeof token === 'string' && this.tokens.has(token);
  }

  device(token: unknown): { id: string; name: string } | undefined {
    return typeof token === 'string' ? this.tokens.get(token) : undefined;
  }

  /** host, one of connection.addresses, names that address's door in place of the pick (the Tailscale name); one not listening is refused. */
  pair(accepted: Accepted, now: Date, host: unknown, addresses: { host: string; kind: string; listening: boolean }[]): unknown {
    let door = { host: 'mac.tail1.ts.net', scheme: 'https', fp: '' }; // Tailscale's own certificate: no fingerprint to pin
    if (host !== undefined && host !== null) {
      if (typeof host !== 'string' || host === '') throw badRequest('host is one of connection.addresses', 'host');
      const a = addresses.find((x) => x.host === host);
      if (!a?.listening) throw refused('network', `no open door serves ${host}`);
      // a self-made certificate on Wi-Fi rides with its fingerprint; the Tailscale IP is named by its MagicDNS name, as the daemon does
      if (a.kind === 'lan' || a.kind === 'mdns') door = { host, scheme: 'https', fp: FAKE_FP };
    }
    const code = this.issueCode();
    return accepted('Pairing code made.', {
      code,
      expires_at: new Date(now.getTime() + 300_000).toISOString(),
      fingerprint: door.fp,
      web_link: `${door.scheme}://${door.host}:7443/#pair=${code}${door.fp ? `&fp=${door.fp}` : ''}`,
    });
  }

  redeem(accepted: Accepted, p: Record<string, unknown>): unknown {
    if (typeof p.code !== 'string') throw badRequest('code is a string', 'code');
    if (typeof p.name !== 'string' || p.name === '') throw badRequest("name is the device's name, a string", 'name');
    if (!this.codes.delete(p.code.trim().toUpperCase())) throw refused('pairing_code', 'that pairing code is not valid');
    const token = 'hld_' + crypto.randomBytes(32).toString('hex'),
      id = 'dev_' + crypto.randomBytes(6).toString('hex');
    this.tokens.set(token, { id, name: p.name });
    return accepted('Paired.', { token, device: id });
  }
}
