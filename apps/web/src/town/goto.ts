// Going straight to a house of the town, and to one of its people, from outside the picture (the top strip's lists): the town on screen
// says how (Town.tsx), as the app says how a page is brought up (layout/openTab.tsx).
type Go = (house: string, person?: { key: string; name: string }) => void;

let go: Go = () => {};
/** Set by the town while it is on screen. */
export const setTownGo = (to: Go) => {
  go = to;
};
/** Shows `house`'s office, and brings up `person`'s chat there when one is named. */
export const townGo: Go = (house, person) => go(house, person);
