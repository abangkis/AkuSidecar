import { readFile, realpath } from 'node:fs/promises';
import { resolve, relative } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { workspace } from './chrome.mjs';

const exec = promisify(execFile);
export async function configuredCapture() {
  if (process.platform !== 'win32') throw new Error('Existing capture profile mode currently requires Windows.');
  const config = resolve(process.env.LOCALAPPDATA, 'AkuSupervisor/services.json');
  const service = JSON.parse(await readFile(config, 'utf8')).services?.akusidecar;
  if (!service) throw new Error('AkuSidecar Supervisor registration was not found.');
  const arg = flag => {
    const i = service.args.indexOf(flag);
    if (i < 0 || !service.args[i + 1]) throw new Error(`Registration is missing ${flag}.`);
    return service.args[i + 1];
  };
  const profilePath = await realpath(arg('--browser-profile'));
  const runtimePath = await realpath(resolve(workspace, 'AkuSidecar/runtime'));
  const child = relative(runtimePath, profilePath);
  if (!child || child.startsWith('..') || child.includes(':') || profilePath.endsWith('-ui-split-cft')) {
    throw new Error('Refusing a profile outside the registered Sidecar runtime capture directory.');
  }
  const executable = await realpath(arg('--chromium-path'));
  // Read only profile-selection metadata, never cookies or login secrets.
  const state = JSON.parse(await readFile(resolve(profilePath, 'Local State'), 'utf8'));
  const directory = state.profile?.last_used || 'Default';
  if (!/^[A-Za-z0-9 _-]+$/.test(directory)) throw new Error('Invalid Chrome profile directory.');
  await realpath(resolve(profilePath, directory));
  return { profilePath, executable, directory, config };
}

export async function profileOwners(profilePath) {
  const script = String.raw`$ErrorActionPreference = 'Stop'; $owners = @(Get-CimInstance Win32_Process -Filter "name = 'chrome.exe'" | Where-Object { $_.CommandLine -notlike '*--type=*' } | ForEach-Object { $m = [regex]::Match($_.CommandLine, '--user-data-dir=(?:"([^"]+)"|(\S+))'); $p = $m.Groups[1].Value + $m.Groups[2].Value; if ($p -and [IO.Path]::GetFullPath($p).TrimEnd('\') -eq $env:X_HEADLESS_PROFILE.TrimEnd('\')) { [pscustomobject]@{pid=$_.ProcessId; executable=$_.ExecutablePath} } }); ConvertTo-Json -InputObject $owners -Compress`;
  const { stdout } = await exec('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
    windowsHide: true, timeout: 10000, env: { ...process.env, X_HEADLESS_PROFILE: profilePath },
  });
  return JSON.parse(stdout.trim() || '[]');
}
