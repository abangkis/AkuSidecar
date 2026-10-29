# Native derivation maintenance admission

Ordinary daily startup restores saved source/profile permission, verifies a new
owner and lets the existing scheduler include ready X/IG providers. This remains
the default for a user's normal `restart-dev.ps1` invocation.

An activation that forbids session transfer uses:

```powershell
.\scripts\restart-dev.ps1 -Actor codex -ActivationMode no-session-transfer
```

`-Actor codex` defaults to `no-session-transfer`. Explicit `-ActivationMode daily`
selects the ordinary path and is appropriate only when the operator has allowed
native derivation to resume. It does not erase an outstanding maintenance hold.
Plan-only and prepare-only commands never arm or release an operational hold.

The workflow first checks the candidate's dedicated
`--native-derivation-hold-probe` capability. An older candidate is rejected before
shutdown. Legacy native-host candidate-probe schemas are unchanged. After the
old process exits gracefully and the complete bundle is promoted, the workflow
atomically writes `aku-sidecar.exe.native-derivation-hold.json` beside the new
image, then asks Supervisor to start the registered stable executable. It does
not change Supervisor registration, the application config or Auto Update.

The new process reads and validates this bounded state before opening its
database, and binds the gate before HTTP, saved-owner restoration or scheduling.
Malformed state fails startup closed. The hold suppresses automatic native owner
restoration, ready-source admission, direct owner installation and queue entry
for X/IG. It neither grants profile permission nor substitutes for account/session
proof. Other available sources can continue under the normal Auto Update rules.

Settings shows **Held for maintenance** with **Resume X** or **Resume IG**. Rendering,
polling and seeing a saved checkbox cannot release the gate. Only the existing
explicit epoch-pinned Start request, with all acknowledgements, releases that
source's hold and creates a fresh verified owner. It does not itself start a
batch. IG's experimental acknowledgement remains mandatory. Stop still revokes
saved permission; it cannot release the other source.

Holds have no expiry and survive restart until explicitly released. Per-source
release is written atomically and rejects stale activation generations. Once
all held sources have been explicitly released, subsequent daily restarts again
restore them normally. The state is denial-only, outside the binary bundle and
outside saved application/profile settings; removing or fabricating it cannot
mint account evidence or a Store permit.

Offline checks cover helper ordering and capability rejection, bounded state
validation, saved permission with no owner/proof/derivation, queue/manual/automatic
admission, explicit source-scoped release and fresh verification. They do not
constitute a real-session transfer test or authorize activation.
