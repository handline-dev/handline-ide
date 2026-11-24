import { useNameOf, useTask, useWorkTranscript } from '@handline/app-data';
import { Markdown } from '#src/ui/Markdown.tsx';
import { Page } from '#src/ui/Page.tsx';
import { Spinner } from '#src/ui/Loading.tsx';
import { Step } from '#src/chat/TurnRecord.tsx';
import { since, useNow } from '#src/format/time.ts';

/** What a seat did for one task beside its agent's own work (`work:<seat>/<task>`), read only, kept current while it runs. */
export function WorkTab({ at }: { at: string }) {
  const slash = at.lastIndexOf('/');
  const seat = at.slice(0, slash);
  const task = Number(at.slice(slash + 1));
  const nameOf = useNameOf();
  const record = useWorkTranscript(seat, task);
  const title = useTask(task).data?.title;
  const owner = nameOf(seat);
  const now = useNow();
  const items = record.data?.items ?? [];
  const first = items[0]?.at;
  return (
    <div className="min-h-0 flex-1 overflow-y-auto py-4">
      <Page className="flex flex-col gap-2">
        <p className="text-sm text-muted">
          T{task} {title} · {owner}, alongside its main work{first && ` · started ${since(first, now)}`}
        </p>
        {record.isPending && <Spinner />}
        {record.isError && <p className="text-sm text-muted">This record could not be read.</p>}
        {record.isSuccess && !items.length && <p className="text-sm text-muted">Nothing done yet.</p>}
        {items.map((i) =>
          i.kind === 'reply' ? (
            <div key={i.id} className="min-w-0 text-sm text-fg">
              {i.text && <Markdown text={i.text} />}
            </div>
          ) : (
            <Step key={i.id} i={i} />
          ),
        )}
      </Page>
    </div>
  );
}
