import { QueryErrorResetBoundary } from '@handline/app-data';
import { Component, Suspense, type ReactNode } from 'react';

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
