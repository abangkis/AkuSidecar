# Reinstalling the Windows development runtime

`scripts/restart-dev.ps1` builds the current AkuSidecar source and inspects the
development database before it stops AkuSupervisor's `akusidecar` service. If
the database marker names a newer runtime, the script refuses to replace the
working binary. This can happen even when the database schema number is the
same: a 0.9.1 last-writer marker cannot be opened by a 0.9.0 runtime.

Every development build first runs `scripts/test-native-reader-regression.ps1`.
The gate covers native-reader profile handoff and owner rotation, explicit
cancellation/shutdown, and the frontend click contracts. It runs without opening
Chrome or restarting the service. A failing test stops the build before runtime
binaries/helper assets are replaced; restart also defers Supervisor configuration
changes until the validated build succeeds. Do not bypass this gate to activate
an unverified fix. It can be run independently from the repository root:

```powershell
.\scripts\test-native-reader-regression.ps1
```

For source changes, use `restart-dev.ps1` after foreground/restart authorization.
A plain Supervisor restart only restarts the existing binary. After activation,
verify a real Open native post click, multiple tabs in one reader window, and
headless readiness after the reader closes. Unit tests cannot prove Windows
foreground activation or live social-source behavior.

The installed-app NSIS uninstaller and `%LOCALAPPDATA%\AkuBrowser\data` belong
to a different runtime. They do not reset the development database at
`AkuSidecar/runtime/aku-sidecar.db`. Ensure no installed-app process owns port
11122 before starting the development service.

To keep the existing development data, use a compatible binary instead of
rebuilding an older product version. To start clean on the current source:

1. Stop `akusidecar` through AkuSupervisor and confirm it has no owned process.
2. Build the development candidate with `scripts/build-dev.ps1 -OutputName
   aku-sidecar.next.exe`. Inspect the exact development database using that
   candidate's `--database-inspect` mode. Record its `fingerprint` and verify
   the reported status is `newer`.
3. Back up `runtime/aku-sidecar.db`, any `-wal`/`-shm` files, and
   `runtime/.runtime-version` to a project-local recovery directory. Verify
   the copied hashes. Keep the development browser profile untouched.
4. Run the candidate with `--database-action=fresh --database-confirm
   --database-expected-fingerprint <fingerprint>` and the same config/database
   path used for inspection. This built-in operation locks and verifies the
   database, then archives it under `runtime/database-backup-*`. Confirm a
   second inspection reports `absent`, and compare the archive's hashes with
   the pre-action backup.
5. Run `scripts/restart-dev.ps1 -WaitForIdleSeconds 0`. Verify AkuSupervisor
   owns a healthy `akusidecar`, `/api/health` reports the source version and a
   healthy database, the new `.runtime-version` marker matches, and Bridge is
   compatible. Repeat the restart once while idle to verify the ordinary path.

The `fresh` action creates a new development database on next startup. It does
not delete the archived history, the development Chrome profile, or any
installed-app data. Do not copy a newer database back onto a running older
runtime; restore data only with a matching runtime and an explicit recovery
decision.
