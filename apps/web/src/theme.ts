// The app's theme follows the system's: `.dark` on <html> switches every colour token in styles.css.
const dark = matchMedia('(prefers-color-scheme: dark)');
const apply = () => document.documentElement.classList.toggle('dark', dark.matches);
apply();
dark.addEventListener('change', apply);
