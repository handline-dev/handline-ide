// Makes the installable apps (`pnpm package [version] [arm64|x64]...`): the web build and the main process once, then for each architecture (both
// when none is named) its daemon and CLI and one electron-builder run, which makes that architecture's dmg and zip with its own Electron.
// Everything lands in release/: Handline-<version>-<arch>.dmg and -<arch>-mac.zip, and the file the app of that architecture reads to
// see whether it is up to date: latest-mac.yml for Apple silicon, latest-x64-mac.yml for Intel (src/pkg-update.ts).
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

const here = import.meta.dirname;
const top = path.resolve(here, '../..');
const version = process.argv[2] ?? 'dev';
const archs = process.argv.length > 3 ? process.argv.slice(3) : ['arm64', 'x64'];
if (archs.some((a) => a !== 'arm64' && a !== 'x64')) {
  console.error('usage: pnpm package [version] [arm64|x64]...');
  process.exit(2);
}

function run(cmd: string, args: string[], cwd = top, env: NodeJS.ProcessEnv = process.env): void {
  const r = spawnSync(cmd, args, { cwd, stdio: 'inherit', env });
  if (r.status !== 0) process.exit(r.status ?? 1);
}

run('pnpm', ['--filter', '@handline/web', 'build']);
run('node', ['build.ts'], here);
const release = path.join(here, 'release');
fs.rmSync(release, { recursive: true, force: true });
for (const arch of archs) {
  const bin = path.join(top, 'dist', `darwin-${arch}`);
  run('bash', ['tools/build-darwin.sh', version, arch === 'x64' ? 'amd64' : 'arm64', bin]);
  const out = path.join(release, arch);
  run(
    'pnpm',
    [
      'exec',
      'electron-builder',
      '--mac',
      `--${arch}`,
      '--config',
      'electron-builder.config.mjs',
      `-c.extraMetadata.version=${version === 'dev' ? '0.0.0-dev' : version}`,
      `-c.directories.output=${out}`,
    ],
    here,
    { ...process.env, HANDLINE_BIN: bin },
  );
  for (const f of fs.readdirSync(out)) {
    if (!/\.(dmg|zip|blockmap|yml)$/.test(f)) continue;
    fs.renameSync(path.join(out, f), path.join(release, f === 'latest-mac.yml' && arch === 'x64' ? 'latest-x64-mac.yml' : f));
  }
  fs.rmSync(out, { recursive: true, force: true });
}
