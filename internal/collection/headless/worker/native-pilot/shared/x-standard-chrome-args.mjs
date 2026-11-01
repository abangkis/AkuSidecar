import {isAbsolute} from 'node:path';

export function standardChromeArgs(profile) {
  if (typeof profile !== 'string' || !isAbsolute(profile)) throw new Error('invalid_profile');
  return [`--user-data-dir=${profile}`, '--profile-directory=Default', '--no-first-run',
    '--no-default-browser-check', '--new-window', 'https://x.com/home'];
}
