import { hooks } from '@handline/app-data';
import { ago } from '#src/format/time.ts';
import { Markdown } from '#src/ui/Markdown.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

/** One task, opened from a T-number: its title and state, who opened it and who works it, the brief, the result, and its memos
 * oldest first. Read only for now; the task's verbs come with the Tasks page. */
export function TaskTab({ id }: { id: number }) {
  const q = hooks.useTaskGet({ task: id });
  const t = q.data;
  if (q.isError) return <p className="m-auto text-sm text-danger">T{id} did not load.</p>;
  if (!t) return <p className="m-auto text-sm text-muted">Loading T{id}</p>;
  const facts = [
    t.state,
    t.priority && `P${t.priority}`,
    `${t.from} → ${t.to}`,
    t.time_box && `time box ${t.time_box}`,
    `opened ${ago(t.created)}`,
    t.done && `done ${ago(t.done)}`,
  ].filter(Boolean);
  return (
    <ScrollFade keep={`task:${id}`} className="min-h-0 flex-1">
      <article className="mx-auto flex max-w-180 flex-col gap-5 px-6 py-8 text-base text-fg">
        <header className="flex flex-col gap-1">
          <span className="text-sm text-muted">T{t.id}</span>
          <h1 className="text-lg font-semibold">{t.title}</h1>
          <p className="text-xs text-muted">{facts.join(' · ')}</p>
          {(t.block_reason || t.hold_reason) && <p className="text-sm text-warning">{t.block_reason ?? t.hold_reason}</p>}
        </header>
        {t.body && <Markdown text={t.body} />}
        {t.evidence && (
          <section className="flex flex-col gap-1">
            <h2 className="text-sm font-semibold text-muted">Result</h2>
            <Markdown text={t.evidence} />
          </section>
        )}
        {t.memos.length > 0 && (
          <section className="flex flex-col gap-4">
            <h2 className="text-sm font-semibold text-muted">Memos</h2>
            {t.memos.map((m) => (
              <div key={m.id} className="flex flex-col gap-1 border-t border-border pt-3">
                <span className="text-xs text-muted">
                  {m.author} · {ago(m.at)}
                </span>
                <Markdown text={m.text} />
              </div>
            ))}
          </section>
        )}
      </article>
    </ScrollFade>
  );
}
