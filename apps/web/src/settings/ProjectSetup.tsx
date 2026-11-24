import { baseName } from '#src/format/path.ts';
// A project's setup: the files copied and the script run in every new worktree an agent gets, and how the last one went.
import { classify, sentence, useWrite } from '@handline/app-data';
import { RpcError, type Repo } from '@handline/client';
import { Check, TriangleAlert } from 'lucide-react';
import { useState } from 'react';
import { ago } from '#src/format/time.ts';
import { Button } from '#src/ui/Button.tsx';
import { Field } from '#src/ui/Field.tsx';

const area =
  'w-full rounded-md border border-border bg-bg p-2 font-mono text-xs text-fg outline-none placeholder:text-muted focus:border-accent';

/** One line under the project: what its setup does, in a few words. */
export function setupSummary(s: Repo['setup']): string {
  const parts = [
    s.copy.length ? `copies ${s.copy.length === 1 ? s.copy[0] : `${s.copy.length} paths`}` : '',
    s.script ? 'runs a script' : '',
  ];
  const did = parts.filter(Boolean).join(', ');
  return did ? `Setup ${did}` : 'No setup';
}

export function ProjectSetup({ repo, close }: { repo: Repo; close: () => void }) {
  const [copy, setCopy] = useState(repo.setup.copy.join('\n'));
  const [script, setScript] = useState(repo.setup.script);
  const save = useWrite('repo.set');
  const fail = save.error ? classify(save.error) : null;
  const field = save.error instanceof RpcError ? save.error.data?.field : undefined;
  const changed = copy !== repo.setup.copy.join('\n') || script !== repo.setup.script;
  const last = repo.setup.last;
  return (
    <form
      className="flex flex-col gap-3 border-t border-border pt-3 pb-1 pl-6"
      onSubmit={(e) => {
        e.preventDefault();
        const paths = copy
          .split('\n')
          .map((l) => l.trim())
          .filter(Boolean);
        save.mutate({ path: repo.path, copy: paths, script: script.trim() }, { onSuccess: close });
      }}
    >
      <Field
        label="Files to copy"
        info="One path per line, from the project's folder into each new worktree: a file, a folder or a glob, e.g. .env. A file already there is kept."
        error={field === 'copy' && fail ? sentence(fail) : undefined}
      >
        <textarea rows={3} value={copy} placeholder=".env" onChange={(e) => (setCopy(e.target.value), save.reset())} className={area} />
      </Field>
      <Field
        label="Setup script"
        info="Runs in each new worktree before the agent starts, for at most 5 minutes. If it fails, the agent still starts."
        error={field === 'script' && fail ? sentence(fail) : undefined}
      >
        <textarea
          rows={3}
          value={script}
          placeholder="pnpm install"
          onChange={(e) => (setScript(e.target.value), save.reset())}
          className={area}
        />
      </Field>
      {last && <LastRun last={last} />}
      <div className="flex items-center gap-2">
        <Button type="submit" variant="primary" disabled={!changed || save.isPending}>
          {save.isPending ? 'Saving…' : 'Save'}
        </Button>
        <Button onClick={close}>Cancel</Button>
        {fail && field !== 'copy' && field !== 'script' && sentence(fail) && (
          <span className="text-xs text-danger" role="alert">
            {sentence(fail)}
          </span>
        )}
      </div>
    </form>
  );
}

function LastRun({ last }: { last: NonNullable<Repo['setup']['last']> }) {
  const where = baseName(last.worktree);
  return (
    <div className="flex flex-col gap-1 text-xs">
      <span className={`flex items-center gap-1.5 ${last.ok ? 'text-muted' : 'text-danger'}`}>
        {last.ok ? <Check size={12} /> : <TriangleAlert size={12} />}
        Last run {ago(last.at)} in {where}: {last.ok ? 'done' : last.error}
        {last.missing.length > 0 && <span className="text-muted"> · not in the project: {last.missing.join(', ')}</span>}
      </span>
      {last.output && (
        <details>
          <summary className="cursor-pointer text-muted">Output</summary>
          <pre className="mt-1 max-h-40 overflow-auto rounded-md bg-surface p-2 whitespace-pre-wrap">{last.output}</pre>
        </details>
      )}
    </div>
  );
}
