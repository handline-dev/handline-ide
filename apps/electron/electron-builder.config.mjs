// The installable app: Handline.app in a dmg and a zip, for one architecture at a time (package.ts passes --arm64 or --x64). Sign with a Developer ID by setting HANDLINE_SIGN_IDENTITY
// to its name (and, for notarization, the usual APPLE_* variables electron-builder reads); with none, the app is ad-hoc signed, which is enough
// to run on this Mac and is all there is without an Apple developer account.
import { createRequire } from 'node:module';
import process from 'node:process';

const developerId = !!process.env.HANDLINE_SIGN_IDENTITY;
// Where package.ts put this architecture's daemon and CLI (tools/build-darwin.sh).
const bin = process.env.HANDLINE_BIN ?? '../../dist/darwin';

export default {
  appId: 'dev.handline.app',
  // electron-builder cannot read the version from a range under pnpm.
  electronVersion: createRequire(import.meta.url)('electron/package.json').version,
  productName: 'Handline',
  directories: { output: 'release' },
  files: ['dist/**', 'assets/**', 'package.json'],
  // The web build, and the daemon, CLI and tmux that `tools/build-darwin.sh` and `tools/build-tmux.sh` made: main.ts finds them under Resources when the app is packaged.
  extraResources: [
    { from: '../web/dist', to: 'web' },
    { from: `${bin}/handlined`, to: 'bin/handlined' },
    { from: `${bin}/hl`, to: 'bin/hl' },
    // tmux, built with libevent inside (tools/build-tmux.sh), for a Mac that has none: the daemon finds it after the system's (go/kit/paths/carried.go).
    { from: `${bin}/tmux`, to: 'bin/tmux' },
    { from: `${bin}/licenses`, to: 'licenses' },
  ],
  // The link page's Open in the Handline app opens handline://pair links in this app (main.ts open-url).
  protocols: [{ name: 'Handline pairing link', schemes: ['handline'] }],
  npmRebuild: false,
  afterSign: 'ad-hoc-sign.mjs',
  mac: {
    category: 'public.app-category.developer-tools',
    icon: 'assets/icon.png',
    artifactName: 'Handline-${version}-${arch}-mac.${ext}',
    target: ['dmg', 'zip'],
    // No identity: electron-builder leaves the app unsigned and the afterSign hook signs it ad hoc.
    identity: developerId ? process.env.HANDLINE_SIGN_IDENTITY : null,
    hardenedRuntime: developerId,
    notarize: false,
    entitlements: developerId ? 'assets/entitlements.mac.plist' : undefined,
    entitlementsInherit: developerId ? 'assets/entitlements.mac.plist' : undefined,
  },
  dmg: { sign: false, artifactName: 'Handline-${version}-${arch}.${ext}' },
  // The address of the rolling release (.github/workflows/mac-release.yml): with it electron-builder writes latest-mac.yml next to the zip,
  // and the app learns where to look for updates from its own package.json (src/pkg-update-app.ts).
  publish: process.env.HANDLINE_UPDATE_URL ? { provider: 'generic', url: process.env.HANDLINE_UPDATE_URL } : undefined,
  extraMetadata: process.env.HANDLINE_UPDATE_URL ? { handlineUpdateUrl: process.env.HANDLINE_UPDATE_URL } : undefined,
};
