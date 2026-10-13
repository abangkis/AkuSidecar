// Shared scaffolding for the opt-in resilience scenarios. Never discovers existing profiles.
import { launchFixtureChrome } from './chrome.mjs';
import { SessionStrategyController } from './strategies.mjs';
import { observeSessionMutations } from './observer.mjs';

export async function openScenario({ executable, root, origin }) {
  const browsers = [], observers = {};
  let session;
  try {
    const reader = await launchFixtureChrome(executable, root, 'owner'); browsers.push(reader);
    const collector = await launchFixtureChrome(executable, root, 'collector'); browsers.push(collector);
    const readerPage = await reader.page(origin);
    const { browserContextId } = await collector.send('Target.createBrowserContext', { disposeOnDetach: true });
    const collectorPage = await collector.page(origin, browserContextId);
    const pages = { reader: readerPage, collector: collectorPage };
    const events = [];
    const clients = Object.fromEntries(['reader', 'collector'].map(id => {
      const browser = id === 'reader' ? reader : collector;
      const scope = id === 'reader' ? {} : { browserContextId };
      return [id, {
        async replaceCookies({ cookies }) {
          await browser.send('Storage.clearCookies', scope);
          await browser.send('Storage.setCookies', { ...scope, cookies });
        },
        exportCookies: () => browser.send('Storage.getCookies', scope),
        async probe() {
          const response = await pages[id].request('/probe');
          return { authenticated: response.status === 200, identity: { accountId: response.body.account } };
        },
        async refresh() {
          const result = observers[id]
            ? await observers[id].authorizeRefresh(request => request('/rotate', 'POST'))
            : await pages[id].request('/rotate', 'POST');
          if (result.status !== 200) throw new Error('fixture_refresh_rejected');
        },
        clear: () => browser.isExited() ? Promise.resolve() : browser.send('Storage.clearCookies', scope),
      }];
    }));
    session = new SessionStrategyController({ origin, expectedAccountId: 'alpha', clients });
    return {
      session, clients, pages, events, observers, chrome: reader.product,
      browsers: { reader, collector },
      async observe() {
        for (const id of ['reader', 'collector']) {
          observers[id] = await observeSessionMutations(pages[id], {
            origin, sessionId: `fixture-${id}`, clientId: id,
            onUnexpected(event) {
              events.push(event);
              session.notifyExternalSessionChange(id, event.kind === 'unexpected_session_mutation' ? 'mutation' : 'coverage_lost');
            },
          });
        }
      },
      async close() {
        const observations = await Promise.allSettled(Object.values(observers).map(observer => observer.close()));
        const revoked = await session.revoke().catch(() => ({ ok: false }));
        const closed = await Promise.allSettled(browsers.map(b => b.close()));
        return { ok: revoked.ok && observations.every(r => r.status === 'fulfilled') && closed.every(r => r.status === 'fulfilled') && browsers.every(b => b.isExited()),
          browsersExited: browsers.every(b => b.isExited()) };
      },
    };
  } catch (error) {
    await session?.revoke().catch(() => {});
    await Promise.allSettled(browsers.map(b => b.close()));
    error.fixtureBrowsersExited = browsers.length === 2 && browsers.every(b => b.isExited());
    throw error;
  }
}
