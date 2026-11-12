// Below 760 px the app is phone-sized: modals and sheets fill the window.
import { useSyncExternalStore } from 'react';

const NARROW = '(max-width: 759px)';
const subscribe = (l: () => void) => {
  const m = matchMedia(NARROW);
  m.addEventListener('change', l);
  return () => m.removeEventListener('change', l);
};
export const useNarrow = () => useSyncExternalStore(subscribe, () => matchMedia(NARROW).matches);
