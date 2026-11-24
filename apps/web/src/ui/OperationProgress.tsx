import { useOperation } from '@handline/app-data';
import { useEffect, useRef } from 'react';
import { Button } from '#src/ui/Button.tsx';

/** A long call the daemon runs: its step while it runs; once done (and `ready`, when the result must show elsewhere first) `onDone`;
 * on failure its message and Close. */
export function OperationProgress({
  operation,
  failed,
  ready = true,
  onDone,
  close,
}: {
  operation: string;
  /** Said when the daemon gives no reason. */
  failed: string;
  ready?: boolean;
  onDone: (result: unknown) => void;
  close: () => void;
}) {
  const op = useOperation(operation).data;
  const done = op?.state === 'done' && ready;
  const fired = useRef(false);
  useEffect(() => {
    if (!done || fired.current) return;
    fired.current = true;
    onDone(op?.result);
  });
  if (done) return null;
  if (op?.state === 'failed')
    return (
      <div className="flex flex-col gap-2 pb-4">
        <p className="text-danger">{op.message ?? failed}</p>
        <div className="flex justify-end">
          <Button onClick={close}>Close</Button>
        </div>
      </div>
    );
  return (
    <p className="pb-4 text-muted">
      {op?.step ?? 'Starting…'}
      {op?.steps_total ? ` (${op.steps_done}/${op.steps_total})` : ''}
    </p>
  );
}
