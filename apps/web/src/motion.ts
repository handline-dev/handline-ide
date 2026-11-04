/** The app's motion: one spring for panels that open, close or move. */
export const spring = { type: 'spring', stiffness: 420, damping: 38, mass: 0.8 } as const;
export const instant = { duration: 0 } as const;
