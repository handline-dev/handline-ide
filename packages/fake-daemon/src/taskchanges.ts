// task.changes: a task's commits as its seed gives them, and the files and diff of the one asked for or of them all.
import type { Methods } from '@handline/contract';
import { Failure, invalidParams } from '#src/errors.ts';

type Changes = Methods['task.changes']['result'];
type File = Changes['files'][number];

/** One seeded commit: its row in task.changes, and each file's counts and part of the diff. */
export type SeedCommit = Omit<Changes['commits'][number], 'files' | 'added' | 'deleted'> & {
  files: (File & { diff: string })[];
};

interface Tasks {
  find(id: unknown): { id: number; commits?: SeedCommit[] };
}

export function changesOf(p: Record<string, unknown>, tasks: Tasks): Changes {
  if (!Number.isInteger(p.task)) throw invalidParams('task is a task number', 'task');
  if (p.commit !== undefined && (typeof p.commit !== 'string' || !/^[0-9a-f]{4,64}$/.test(p.commit)))
    throw invalidParams("commit is a commit's hex id", 'commit');
  const t = tasks.find(p.task);
  const all = t.commits ?? [];
  const want = p.commit as string | undefined;
  const shown = want === undefined ? all : all.filter((c) => c.sha.startsWith(want));
  if (want !== undefined && shown.length !== 1)
    throw new Failure('not_found', `no commit ${want} of T${t.id}`, { what: 'commit', commit: want });
  // Each file once, in path order, its counts summed and its parts in commit order.
  const byPath = new Map<string, File & { diff: string }>();
  for (const c of shown)
    for (const f of c.files) {
      const have = byPath.get(`${f.repo}\0${f.path}`);
      if (have) Object.assign(have, { added: have.added + f.added, deleted: have.deleted + f.deleted, diff: have.diff + f.diff });
      else byPath.set(`${f.repo}\0${f.path}`, { ...f });
    }
  const files = [...byPath.entries()].sort(([a], [b]) => (a < b ? -1 : 1)).map(([, f]) => f);
  return {
    task: t.id,
    commits: all.map(({ files: fs, ...c }) => ({
      ...c,
      files: fs.length,
      added: fs.reduce((n, f) => n + f.added, 0),
      deleted: fs.reduce((n, f) => n + f.deleted, 0),
    })),
    commit: want === undefined ? null : shown[0].sha,
    files: files.map(({ diff: _, ...f }) => f),
    diff: files.map((f) => f.diff).join(''),
    truncated: false,
    left_out: [],
  };
}
