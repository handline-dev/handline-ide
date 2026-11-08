import { QueryErrorResetBoundary } from '@handline/app-data';
import { RefreshCw, RotateCw, TriangleAlert } from 'lucide-react';
import { Component, Suspense, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';

type Failed = (retry: () => void) => ReactNode;

class Catch extends Component<{ failed: Failed; onReset: () => void; children: ReactNode }, { error: boolean }> {
  state = { error: false };
  static getDerivedStateFromError() {
    return { error: true };
  }
  render() {
    if (!this.state.error) return this.props.children;
    return this.props.failed(() => {
      this.props.onReset();
      this.setState({ error: false });
    });
  }
}

/** A view that reads with suspense: `loading` until its first data is in, `failed` (with a retry that reads again) when a read
 * throws. Later reads in it (an older page, a live line) do not show `loading` again. */
export function Boundary({ loading, failed, children }: { loading: ReactNode; failed: Failed; children: ReactNode }) {
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <Catch failed={failed} onReset={reset}>
          <Suspense fallback={loading}>{children}</Suspense>
        </Catch>
      )}
    </QueryErrorResetBoundary>
  );
}

/** `failed` for a whole panel or the app: the part that broke says so and offers a retry or a page reload, and the rest stays up. */
export const brokenView =
  (what: string): Failed =>
  (retry) => (
    <div className="flex h-full min-h-0 flex-1 flex-col items-center justify-center gap-5 p-6 text-center">
      <TriangleAlert size={40} className="text-danger" />
      <p className="text-base text-fg">{what} stopped working.</p>
      <div className="flex gap-3">
        <Button size="lg" variant="primary" onClick={retry}>
          <RotateCw /> Retry
        </Button>
        <Button size="lg" onClick={() => location.reload()}>
          <RefreshCw /> Reload page
        </Button>
      </div>
    </div>
  );
