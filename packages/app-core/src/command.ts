// Command cards: what both apps say about a command fence: whether a fence is one, what is risky about it, and how its run went.
const COMMAND_LANGS = new Set(['sh', 'bash', 'zsh', 'shell']);

/** A fence's language, as written after the three backticks, names a command. */
export const isCommandLang = (lang: string): boolean => COMMAND_LANGS.has(lang.trim().toLowerCase());

/** The command after `! `, Claude Code's way for an agent to ask the person to run one (a login, a password prompt), or null. */
export function bangOf(text: string): string | null {
  const m = /^! +(\S.*)$/.exec(text.trim());
  return m ? m[1].trim() : null;
}

/** Inline `! <command>` spans are numbered from this, apart from the fences, so a span never takes or moves a fence's run. */
export const BANG_FROM = 1000;

/** The command a fence holds, or null: an unnamed fence of one `! ` line, the command after it; a command language's text, without the
 * `$ ` prompts when every line has one; a console fence's, or an unnamed one that starts with `$ `, prompt lines without their `$ ` and
 * their `> ` continuations (the rest is output). Its run is named by the fence's place among all the code fences of its text, from 0,
 * so a change to what counts as a command moves no run. */
export function commandOf(lang: string, text: string): string | null {
  const lines = text.split('\n').filter((x) => x.trim());
  const bang = lang.trim() === '' && lines.length === 1 ? bangOf(lines[0]) : null;
  if (bang) return bang;
  if (isCommandLang(lang)) {
    const prompted = lines.length > 0 && lines.every((x) => /^\$ /.test(x));
    return (prompted ? lines.map((x) => x.slice(2)).join('\n') : text).trim() || null;
  }
  const l = lang.trim().toLowerCase();
  if (l !== 'console' && !(l === '' && /^\$ /.test(lines[0] ?? ''))) return null;
  const typed: string[] = [];
  let inCommand = false;
  for (const x of lines) {
    if (/^\$ /.test(x)) typed.push(x.slice(2).trim());
    else if (inCommand && /^> /.test(x)) typed.push(x.slice(2).trim());
    inCommand = /^[$>] /.test(x) && x.trimEnd().endsWith('\\');
  }
  return typed.length ? typed.join('\n') : null;
}

const RISKS: [RegExp, string][] = [
  [
    /\brm\s+(-[a-z]*r[a-z]*f|-[a-z]*f[a-z]*r|-r\s+-f|-f\s+-r|--recursive\b[^\n;|&]*--force|--force\b[^\n;|&]*--recursive)/i,
    'It deletes files and folders for good.',
  ],
  [/(^|[\s;&|(])sudo\b/, 'It runs with full rights on the computer.'],
  [/\bgit\s+push\b[^\n;|&]*(\s-f\b|--force)/, 'It overwrites history on the remote.'],
  [/\b(curl|wget)\b[^\n]*\|\s*(sudo\s+)?(ba|z)?sh\b/, 'It runs a script it downloads.'],
];

/** One plain line on why a command is risky to run, or null when nothing in it is. */
export function commandRisk(command: string): string | null {
  return RISKS.find(([re]) => re.test(command))?.[1] ?? null;
}

/** The line under a command card: "Running · 4s", "Done · exit 0 · 3s", "Failed · exit 1 · 3s" or "Stopped". `now` is ms since the epoch. */
export function runLine(
  run: { state: 'running' | 'done' | 'failed' | 'stopped'; exit?: number; seconds?: number; started_at: string },
  now: number,
): string {
  if (run.state === 'stopped') return 'Stopped';
  if (run.state === 'running') return `Running · ${Math.max(0, Math.round((now - Date.parse(run.started_at)) / 1000))}s`;
  return `${run.state === 'done' ? 'Done' : 'Failed'} · exit ${run.exit ?? '?'} · ${run.seconds ?? 0}s`;
}

/** A command with each `&&`, `||` or `;` step on its own line (`&&` and `||` lead the next line, `;` ends its own), the quoted text
 * and the lines it already has left as they are; nothing is dropped, so the lines read as the whole of what runs. */
export function commandSteps(cmd: string): string {
  let out = '';
  let quote = '';
  for (let i = 0; i < cmd.length; i++) {
    const c = cmd[i];
    if (quote) {
      if (c === '\\' && quote === '"' && i + 1 < cmd.length) out += c + cmd[++i];
      else out += c;
      if (c === quote) quote = '';
      continue;
    }
    if (c === '\\' && i + 1 < cmd.length) {
      out += c + cmd[++i];
      continue;
    }
    if (c === "'" || c === '"') quote = c;
    const two = cmd.slice(i, i + 2);
    if (two === '&&' || two === '||') {
      out = `${out.trimEnd()}\n${two} `;
      i += 1;
      while (/\s/.test(cmd[i + 1] ?? '')) i++;
      continue;
    }
    if (c === ';' && cmd[i + 1] !== ';' && cmd[i - 1] !== ';') {
      out = `${out.trimEnd()};\n`;
      while (/\s/.test(cmd[i + 1] ?? '')) i++;
      continue;
    }
    out += c;
  }
  return out.trim();
}

/** How a run proposal ended, as a word and a mark; null while it waits. */
export type RunOutcome = { mark: 'ok' | 'fail' | 'denied' | 'running' | 'other'; word: string };
export function runOutcome(p: { state: string; run?: { state?: string; exit?: number } }): RunOutcome | null {
  if (p.state === 'open') return null;
  if (p.state === 'rejected') return { mark: 'denied', word: 'Denied' };
  if (p.state === 'withdrawn') return { mark: 'other', word: 'Withdrawn' };
  const r = p.run?.state;
  if (r === 'done') return { mark: 'ok', word: 'Ran · succeeded' };
  if (r === 'failed') return { mark: 'fail', word: `Failed · exit ${p.run?.exit ?? '?'}` };
  if (r === 'running') return { mark: 'running', word: 'Running' };
  if (r === 'stopped') return { mark: 'fail', word: 'Stopped' };
  if (p.state === 'approved' && !r) return { mark: 'ok', word: 'Allowed' };
  return { mark: 'other', word: r ?? p.state };
}

/** A run proposal's command: its run's, else the `run: ` line of its words (a `cwd: ` line is the folder, not the command). */
export const runCommand = (p: { run?: { command: string }; diff?: string }): string =>
  p.run?.command ??
  (p.diff ?? '')
    .split('\n')
    .filter((l) => !l.startsWith('cwd: '))
    .map((l) => l.replace(/^run: /, ''))
    .join('\n')
    .trim();
