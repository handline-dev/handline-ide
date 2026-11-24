// The app's theme follows the system's: `.dark` on <html> switches every colour token in styles.css.
const dark = matchMedia('(prefers-color-scheme: dark)');
const apply = () => document.documentElement.classList.toggle('dark', dark.matches);
apply();
dark.addEventListener('change', apply);
// Another app in front: the whole window draws in the unfocused colours (styles.css .unfocused).
const away = () => document.documentElement.classList.toggle('unfocused', !document.hasFocus());
away();
addEventListener('focus', away);
addEventListener('blur', away);
