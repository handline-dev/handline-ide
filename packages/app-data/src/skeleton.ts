// The loading placeholder: shown only on first load, not before 150 ms (a fast answer shows no flash), and not for less than 300 ms
// once shown. No spinner anywhere: a region that waits draws this, a write that waits draws its pending mark.

/** The shared look for the web: a quiet pulse that stops when the OS asks for reduced motion. The phone draws its own. */
export const SKELETON_CSS = `.sk { background: var(--line); animation: sk-pulse 1.4s ease-in-out infinite; }
@keyframes sk-pulse { 50% { opacity: 0.5; } }
@media (prefers-reduced-motion: reduce) { .sk { animation: none; } }`;
