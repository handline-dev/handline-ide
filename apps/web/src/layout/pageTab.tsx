// The app's own pages that open as a tab, the sidebar's Requests, Usage and Town: none takes an agent or an id.
import { RequestsTab } from '#src/requests/RequestsTab.tsx';
import { TownTab } from '#src/town/Town.tsx';
import { Usage } from '#src/usage/Usage.tsx';

export function PageTab({ opens }: { opens?: string | null }) {
  if (opens === 'requests') return <RequestsTab />;
  if (opens === 'usage') return <Usage />;
  if (opens === 'town') return <TownTab />;
  return null;
}
