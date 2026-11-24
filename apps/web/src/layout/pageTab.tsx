// The app's own pages that open as a tab, the sidebar's Requests and Usage: none takes an agent or an id. Town covers the app instead
// (town/mode.tsx).
import { RequestsTab } from '#src/requests/RequestsTab.tsx';
import { Usage } from '#src/usage/Usage.tsx';

export function PageTab({ opens }: { opens?: string | null }) {
  if (opens === 'requests') return <RequestsTab />;
  if (opens === 'usage') return <Usage />;
  return null;
}
