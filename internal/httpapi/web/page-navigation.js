// Page identity, panel and navigation semantics have a single source of truth.
// Data loading and page-specific teardown remain the application's responsibility.
export const PAGES = Object.freeze([
  ['timeline', 'session'], ['library', 'library'], ['topics', 'topics'],
  ['inbox', 'inbox'], ['settings', 'settings'],
].map(([name, button]) => Object.freeze({name, panel: `#${name}-panel`, button: `#${button}-view-button`, heading: `#${name}-heading`})));

export function normalizePage(view) {
  return PAGES.some(page => page.name === view) ? view : 'timeline';
}

export function focusPageHeading(root, view) {
  root.querySelector(PAGES.find(page => page.name === normalizePage(view)).heading)?.focus?.();
}

export function renderSetupPage(root, view) {
  if (!['onboarding', 'calibration'].includes(view)) throw new Error('Unknown setup page');
  for (const page of PAGES) {
    root.querySelector(page.panel)?.classList.add('hidden');
    const button = root.querySelector(page.button);
    button?.classList.toggle('selected', false);
    button?.setAttribute('aria-current', 'false');
  }
  for (const name of ['onboarding', 'calibration']) root.querySelector(`#${name}-panel`)?.classList.toggle('hidden', name !== view);
  root.querySelector('.view-switch')?.classList.add('hidden');
}

export function renderPageNavigation(root, view, {loading = false, focus = true} = {}) {
  const selected = loading ? 'timeline' : normalizePage(view);
  for (const id of ['onboarding', 'calibration']) root.querySelector(`#${id}-panel`)?.classList.add('hidden');
  root.querySelector('.view-switch')?.classList.toggle('hidden', loading);
  for (const page of PAGES) {
    const active = page.name === selected;
    root.querySelector(page.panel)?.classList.toggle('hidden', !active);
    const button = root.querySelector(page.button);
    button?.classList.toggle('selected', active);
    button?.setAttribute('aria-current', active ? 'page' : 'false');
  }
  if (!loading && focus) focusPageHeading(root, selected);
  return selected;
}
