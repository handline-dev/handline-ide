/** A path's last part: the folder or file name, with trailing slashes dropped and either slash kind read. */
export const baseName = (path: string) =>
  path
    .replace(/[\\/]+$/, '')
    .split(/[\\/]/)
    .pop() || path;
