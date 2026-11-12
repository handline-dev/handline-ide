// The first task offered on the agent step: what the repo itself suggests, and the agent's name from its folder.

/** The agent's name from its folder's: lower case, anything else a dash. */
export const agentNameOf = (folder: string): string =>
  folder
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '') || 'agent';
type Card = { title: string; note: string; task: string };
/** What the repo itself offers to do first: its tests, its newest open issues, its TODOs, its uncommitted changes. */
export function factCards(
  f: { test_command: string | null; issues: { number: number; title: string }[] | null; todos: number; uncommitted: number } | undefined,
): Card[] {
  if (!f) return [];
  return [
    ...(f.test_command
      ? [{ title: 'Run the tests and fix what fails', note: f.test_command, task: `Run ${f.test_command} and fix what fails.` }]
      : []),
    ...(f.issues ?? []).map((i) => ({
      title: `#${i.number} ${i.title}`,
      note: 'Open issue',
      task: `Work on issue #${i.number}: ${i.title}`,
    })),
    ...(f.todos
      ? [
          {
            title: `Go through ${f.todos} TODOs`,
            note: 'TODO and FIXME comments',
            task: 'Go through the TODO and FIXME comments, and do the ones worth doing.',
          },
        ]
      : []),
    ...(f.uncommitted
      ? [
          {
            title: `Tidy ${f.uncommitted} uncommitted changes`,
            note: 'Review and commit them',
            task: 'Review the uncommitted changes and commit them in sensible pieces.',
          },
        ]
      : []),
  ];
}
export const LOOK_AROUND = 'Look around this project and propose 3 tasks worth doing next. Read only: change nothing.';
