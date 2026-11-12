// Open modals share one browser history entry: Back closes the top one (a modal with steps goes back a step instead), and when the
// last one closes some other way its entry comes off. Navigation pushed on top of it leaves it behind; Back skips such a dead entry.
import { useEffect, useLayoutEffect, useRef } from 'react';

type Layer = { onBack: () => void };
const stack: Layer[] = [];
const onModal = () => (history.state as { modal?: boolean } | null)?.modal === true;
/** A back() this file made: the popstate it causes is not the person's Back. */
let leaving = false;
let settling = false;

const pushEntry = () => history.pushState({ ...(history.state as object | null), modal: true }, '');

/** After a commit: one entry while any modal is open, none once all are closed. Run once the commit's cleanups and effects are done,
 * so a modal closing as another opens (or StrictMode's remount) never takes the entry off and puts it back. */
function settle() {
  if (settling) return;
  settling = true;
  queueMicrotask(() => {
    settling = false;
    if (leaving) return;
    if (stack.length && !onModal()) pushEntry();
    else if (!stack.length && onModal()) {
      leaving = true;
      history.back();
    }
  });
}

addEventListener('popstate', () => {
  if (leaving) {
    leaving = false;
    return settle();
  }
  const top = stack[stack.length - 1];
  if (!top) {
    // An entry left by a modal that closed under navigation: nothing to close here, so it is passed over.
    if (onModal()) {
      leaving = true;
      history.back();
    }
    return;
  }
  top.onBack();
  // Still open (it went back a step, or another is under it): the entry goes back on.
  settle();
});

/** While `open`, Back calls `onBack`: it closes the modal, or goes back a step. */
export function useBackCloses(open: boolean, onBack: () => void) {
  const back = useRef(onBack);
  useLayoutEffect(() => {
    back.current = onBack;
  });
  useEffect(() => {
    if (!open) return;
    const layer: Layer = { onBack: () => back.current() };
    stack.push(layer);
    settle();
    return () => {
      stack.splice(stack.indexOf(layer), 1);
      settle();
    };
  }, [open]);
}
