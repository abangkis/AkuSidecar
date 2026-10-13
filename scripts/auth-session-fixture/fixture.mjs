import { createServer } from 'node:http';
import { randomBytes } from 'node:crypto';

// Entirely invented accounts/tokens, bound to a fresh loopback port. No source credentials.
export async function startFixture() {
  const sessions = new Map();
  let epoch = 1, logins = 0, inflight = 0, overlap = 0, rotations = 0;
  const secretValues = new Set();
  function issue(account) {
    const token = randomBytes(32).toString('hex');
    secretValues.add(token); sessions.set(token, { account, epoch }); return token;
  }
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, 'http://127.0.0.1');
    res.setHeader('Cache-Control', 'no-store');
    res.setHeader('Content-Security-Policy', "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'");
    if (url.pathname === '/') { res.setHeader('Content-Type', 'text/html'); res.end('<!doctype html><title>AkuBrowser local auth fixture</title><h1>Local authentication fixture</h1>'); return; }
    res.setHeader('Content-Type', 'application/json');
    const reply = (status, value) => { res.statusCode = status; res.end(JSON.stringify(value)); };
    const token = (req.headers.cookie || '').split(';').map(s => s.trim()).find(s => s.startsWith('fixture_session='))?.slice(16);
    const session = sessions.get(token);
    if (url.pathname === '/login' && req.method === 'POST') {
      const account = url.searchParams.get('account') === 'beta' ? 'beta' : 'alpha';
      logins++;
      res.setHeader('Set-Cookie', `fixture_session=${issue(account)}; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=3600`);
      reply(200, { account, epoch }); return;
    }
    if (!session || session.epoch !== epoch) { reply(401, { state: 'needs_login' }); return; }
    if (url.pathname === '/probe') { reply(200, { account: session.account, epoch }); return; }
    if (url.pathname === '/cookie-evidence' || url.pathname === '/scoped/cookie-evidence') {
      reply(200, { names: (req.headers.cookie || '').split(';').map(s => s.trim().split('=')[0]).filter(Boolean) }); return;
    }
    if (url.pathname === '/logout' && req.method === 'POST') {
      sessions.clear(); epoch++; res.setHeader('Set-Cookie', 'fixture_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax');
      reply(200, { state: 'logged_out' }); return;
    }
    if ((url.pathname === '/rotate' && req.method === 'POST') || url.pathname === '/reader-rotate') {
      // Model a server that invalidates the shared credential on refresh.
      sessions.delete(token);
      rotations++;
      res.setHeader('Set-Cookie', `fixture_session=${issue(session.account)}; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=3600`);
      reply(200, { rotated: true }); return;
    }
    if (url.pathname === '/reader' || url.pathname === '/collect') {
      inflight++; overlap = Math.max(overlap, inflight);
      await new Promise(resolve => setTimeout(resolve, 200)); inflight--;
      reply(200, { account: session.account, epoch: session.epoch, postId: 'fixture-post-1' }); return;
    }
    reply(404, { state: 'missing' });
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  return {
    origin: `http://127.0.0.1:${server.address().port}`,
    stats: () => ({ logins, overlap, epoch, rotations }),
    assertSanitized(text) { if ([...secretValues].some(s => text.includes(s))) throw new Error('fixture_secret_in_receipt'); },
    close: () => new Promise((resolve, reject) => { server.closeAllConnections(); server.close(err => err ? reject(err) : resolve()); }),
  };
}
