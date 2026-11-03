import {performance} from 'node:perf_hooks';
import {probeError} from './x-native-value-validation.mjs';
import {standardChromeArgs} from './x-standard-chrome-args.mjs';
import {resolve} from 'node:path';
import {lstat, realpath} from 'node:fs/promises';

// Directory identity, launch envelope and consumed two-phase grant policy.
class TwoPhaseWorkflowGrant {
  constructor({ owner, pid, now = () => performance.now() }) {
    if (!Number.isSafeInteger(owner) || owner <= 0 || !Number.isSafeInteger(pid) || pid <= 0 || typeof now !== 'function') {
      throw probeError('workflow_state_invalid');
    }
    this.owner = owner;
    this.pid = pid;
    this.now = now;
    this.attempted = false;
    this.authorized = false;
    this.consumed = false;
    this.expiresAt = 0;
    this.lastNow = null;
  }

  readClock() {
    let now;
    try {
      now = this.now();
    } catch {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_invalid');
    }
    if (!Number.isFinite(now) || now < 0) {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_invalid');
    }
    if (this.lastNow !== null && now < this.lastNow) {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_regressed');
    }
    this.lastNow = now;
    return now;
  }

  authorize({ owner, oldPID, nonce, exitCode, identityUnchanged }) {
    if (this.attempted || this.authorized || this.consumed) throw probeError('workflow_grant_replay');
    this.attempted = true;
    if (owner !== this.owner || oldPID !== this.pid || typeof nonce !== 'string' || !/^[a-f0-9]{64}$/u.test(nonce)) {
      throw probeError('workflow_ticket_invalid');
    }
    if (exitCode !== 0) throw probeError('phase1_exit_required');
    if (identityUnchanged !== true) throw probeError('owned_profile_changed');
    this.expiresAt = this.readClock() + 10_000;
    this.authorized = true;
    return { authorized: true };
  }

  consume({ identityUnchanged }) {
    if (!this.authorized || this.consumed) throw probeError('workflow_grant_replay');
    if (this.readClock() >= this.expiresAt) {
      this.consumed = true;
      throw probeError('workflow_grant_expired');
    }
    if (identityUnchanged !== true) {
      this.consumed = true;
      throw probeError('owned_profile_changed');
    }
    this.consumed = true;
    return true;
  }
}

function twoPhaseChromeArgs(profile, remoteDebugging) {
  if (typeof remoteDebugging !== 'boolean') throw probeError('chrome_args_invalid');
  const args = standardChromeArgs(profile);
  if (remoteDebugging) args.splice(args.length - 1, 0, '--remote-debugging-pipe');
  return args;
}

function sameWindowsPath(left, right) {
  return typeof left === 'string' && typeof right === 'string' &&
    resolve(left).toLowerCase() === resolve(right).toLowerCase();
}

async function ordinaryDirectoryIdentity(path) {
  const item = await lstat(path, { bigint: true }).catch(() => null);
  const real = await realpath(path).catch(() => '');
  if (!item?.isDirectory() || item.isSymbolicLink() || !real || !sameWindowsPath(real, path)) {
    throw probeError('owned_directory_invalid');
  }
  const identity = {
    real: resolve(real).toLowerCase(),
    dev: item.dev.toString(),
    ino: item.ino.toString(),
    birthtimeNs: item.birthtimeNs.toString(),
  };
  if (identity.dev === '0' || identity.ino === '0' || identity.birthtimeNs === '0') {
    throw probeError('owned_directory_identity_unavailable');
  }
  return Object.freeze(identity);
}

function sameDirectoryIdentity(left, right) {
  return !!left && !!right && left.real === right.real && left.dev === right.dev &&
    left.ino === right.ino && left.birthtimeNs === right.birthtimeNs;
}

export {TwoPhaseWorkflowGrant, twoPhaseChromeArgs, sameWindowsPath, ordinaryDirectoryIdentity, sameDirectoryIdentity};
