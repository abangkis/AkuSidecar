import {probeError} from './x-native-value-validation.mjs';

// Fixed source target/readiness and one-observation gate policy.
const HOME_URL = 'https://x.com/home';

const X_ORIGIN = 'https://x.com';

class LoginObservationGate {
  constructor() {
    this.loginVisible = false;
    this.observationStarted = false;
    this.networkEnabled = false;
    this.reloadCount = 0;
  }

  setLoginVisible(value) {
    if (typeof value !== 'boolean') throw probeError('login_state_invalid');
    this.loginVisible = value;
  }

  beginObservation() {
    if (!this.loginVisible || this.observationStarted || this.networkEnabled || this.reloadCount !== 0) {
      throw probeError('observation_gate_closed');
    }
    this.observationStarted = true;
  }

  markNetworkEnabled() {
    if (!this.observationStarted || this.networkEnabled || this.reloadCount !== 0) {
      throw probeError('network_enable_order_invalid');
    }
    this.networkEnabled = true;
  }

  markReloaded() {
    if (!this.networkEnabled || this.reloadCount !== 0) throw probeError('reload_order_invalid');
    this.reloadCount = 1;
  }
}

function isTargetURL(rawURL) {
  if (typeof rawURL !== 'string' || rawURL.length > 2_048) return false;
  try {
    const parsed = new URL(rawURL);
    return parsed.origin === X_ORIGIN && !parsed.username && !parsed.password;
  } catch { return false; }
}

const ACCOUNT_READY_EXPRESSION = `(()=>{const visible=e=>!!e&&e.getClientRects().length>0&&getComputedStyle(e).visibility!=='hidden'&&getComputedStyle(e).display!=='none';return location.origin==='https://x.com'&&location.pathname==='/home'&&visible(document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"]'))&&visible(document.querySelector('a[data-testid="AppTabBar_Profile_Link"]'))})()`;

function isHomeTarget(value) {
  if (!isTargetURL(value)) return false;
  try { return new URL(value).pathname === '/home'; } catch { return false; }
}

export {HOME_URL, X_ORIGIN, ACCOUNT_READY_EXPRESSION, isTargetURL, isHomeTarget, LoginObservationGate};
