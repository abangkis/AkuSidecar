param()
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$qaParent = Join-Path $projectRoot 'runtime\qa\auth-session-sidecar'
foreach ($relative in @('runtime', 'runtime\qa', 'runtime\qa\auth-session-sidecar', '.go-build', '.go-build\x-session-verifier', '.go-build\mod')) {
    $directory = Join-Path $projectRoot $relative
    if (!(Test-Path -LiteralPath $directory -PathType Container)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Store QA paths must be ordinary project directories.' }
}
$runId = [guid]::NewGuid().ToString('N')
$taskRoot = Join-Path $qaParent ('run-manual-pilot-store-' + $runId)
New-Item -ItemType Directory -Path $taskRoot | Out-Null
$log = Join-Path $qaParent ('manual-pilot-store-' + $runId + '.log.txt')
$variables = @('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT','AKU_POLICY_STAGE','AKU_POLICY_OLD_PROBE')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$passed = $false
Push-Location -LiteralPath $projectRoot
try {
    $env:TEMP=$taskRoot; $env:TMP=$taskRoot; $env:GOTMPDIR=$taskRoot
    $env:GOCACHE=Join-Path $projectRoot '.go-build\x-session-verifier'
    $env:GOMODCACHE=Join-Path $projectRoot '.go-build\mod'
    $env:GOWORK='off'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'; $env:GOFLAGS=''
    $env:AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT=$null; $env:AKU_POLICY_STAGE=$null; $env:AKU_POLICY_OLD_PROBE=$null
    Write-Output ('Synthetic manual-pilot Store run: ' + $runId) | Tee-Object -FilePath $log
    foreach ($mode in @('default','auth_session_dev','auth_session_fixture','auth_session_dev auth_session_fixture')) {
        $goArguments = @('test','./internal/store','-count=1','-timeout=180s','-v')
        if ($mode -ne 'default') { $goArguments += @('-tags',$mode) }
        & go @goArguments 2>&1 | Tee-Object -FilePath $log -Append
        if ($LASTEXITCODE -ne 0) { throw ('Synthetic Store contracts failed: ' + $mode) }
        Write-Output ('PASS: Store contracts ' + $mode) | Tee-Object -FilePath $log -Append
    }
    # The archived v29 source is local and immutable; no fetch or runtime occurs.
    $baseline = '28fe840e91d75b184702949d51b0622b3a364f2b'
    $oldSource = Join-Path $taskRoot 'schema29-source'
    New-Item -ItemType Directory -Path $oldSource | Out-Null
    $archive = Join-Path $taskRoot 'schema29-source.tar'
    & git archive --format=tar -o $archive $baseline -- go.mod go.sum internal
    if ($LASTEXITCODE -ne 0) { throw 'Local schema-29 archive failed.' }
    & tar -xf $archive -C $oldSource
    if ($LASTEXITCODE -ne 0) { throw 'Local schema-29 extraction failed.' }
    $probe = Join-Path $taskRoot 'compat-probe.go'
    @'
package main
import (
 "context"
 "database/sql"
 "fmt"
 "os"
 "strings"
 "github.com/abangkis/AkuSidecar/internal/domain"
 "github.com/abangkis/AkuSidecar/internal/store"
)
func main() {
 if len(os.Args)!=3 { panic("synthetic probe arguments") }
 mode,path:=os.Args[1],os.Args[2]
 s,err:=store.Open(path,domain.DefaultSettings("expanded","quiet","promote_unused_budget",true))
 if mode=="reject" {
  if err==nil { s.Close(); panic("schema-29 binary opened schema-30") }
  if !strings.Contains(err.Error(),"database schema 30 is incompatible with required schema 29") { panic("unexpected schema-29 rejection") }
  fmt.Println("PASS: real schema-29 binary refused schema-30 database")
  return
 }
 if err!=nil { panic(err) }
 if err=s.Close(); err!=nil { panic(err) }
 db,err:=sql.Open("sqlite",path); if err!=nil { panic(err) }; defer db.Close()
 ctx:=context.Background()
 if mode=="seed" {
  if store.SchemaVersion!=29 { panic("pinned historical source is not schema 29") }
  _,err=db.ExecContext(ctx,`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES('synthetic-upgrade','update','running',1,1,'now');
INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('synthetic-run','synthetic-upgrade','x',0,'waiting_for_bridge','capture','now');
INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('synthetic-queued','synthetic-run','collect_visible','queued','{"synthetic":"preserve"}','now'),('synthetic-claimed','synthetic-run','collect_visible','claimed','{"synthetic":"preserve"}','now');
INSERT INTO meta(key,value) VALUES('manual_pilot_upgrade_sentinel','preserve');`)
  if err!=nil { panic(err) }
  fmt.Println("PASS: real schema-29 binary created synthetic queued/claimed work")
  return
 }
 if mode!="upgrade"||store.SchemaVersion!=30 { panic("invalid synthetic upgrade mode") }
 var count int; var version,marker string
 if db.QueryRowContext(ctx,`SELECT COUNT(*) FROM bridge_commands WHERE payload_json='{"synthetic":"preserve"}' AND ((id='synthetic-queued' AND status='queued') OR (id='synthetic-claimed' AND status='claimed'))`).Scan(&count)!=nil||count!=2 { panic("queue changed during migration") }
 if db.QueryRowContext(ctx,`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version)!=nil||version!="30" { panic("schema upgrade absent") }
 if db.QueryRowContext(ctx,`SELECT value FROM meta WHERE key='manual_pilot_upgrade_sentinel'`).Scan(&marker)!=nil||marker!="preserve" { panic("migration marker changed") }
 fmt.Println("PASS: schema-29 to schema-30 migration preserved queued/claimed work")
}
'@ | Set-Content -LiteralPath $probe -Encoding utf8NoBOM
    # Internal imports require the probe source to live inside its chosen module.
    $oldProbe = Join-Path $oldSource 'compat-probe.go'
    Copy-Item -LiteralPath $probe -Destination $oldProbe
    Push-Location -LiteralPath $oldSource
    try { & go build -o (Join-Path $taskRoot 'schema29-probe.exe') $oldProbe } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'Pinned schema-29 consumer build failed.' }
    $currentProbe = Join-Path $projectRoot ('.go-build\manual-pilot-probe-' + $runId + '.go')
    Copy-Item -LiteralPath $probe -Destination $currentProbe
    try { & go build -o (Join-Path $taskRoot 'schema30-probe.exe') $currentProbe } finally { Remove-Item -LiteralPath $currentProbe }
    if ($LASTEXITCODE -ne 0) { throw 'Current schema-30 consumer build failed.' }
    $database = Join-Path $taskRoot 'compatibility\synthetic.db'
    & (Join-Path $taskRoot 'schema29-probe.exe') seed $database 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Historical synthetic database creation failed.' }
    & (Join-Path $taskRoot 'schema30-probe.exe') upgrade $database 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0) { throw 'Same-database synthetic upgrade failed.' }
    $before = (Get-FileHash -LiteralPath $database -Algorithm SHA256).Hash
    & (Join-Path $taskRoot 'schema29-probe.exe') reject $database 2>&1 | Tee-Object -FilePath $log -Append
    if ($LASTEXITCODE -ne 0 -or (Get-FileHash -LiteralPath $database -Algorithm SHA256).Hash -ne $before) { throw 'Downgrade barrier did not preserve the synthetic database.' }
    Write-Output 'PASS: historical binary rejection left schema-30 database bytes unchanged.' | Tee-Object -FilePath $log -Append
    $passed=$true
    Write-Output ('Evidence: ' + $log)
} finally {
    Pop-Location
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
    if ($passed -and (Test-Path -LiteralPath $taskRoot -PathType Container)) {
        $resolved=(Resolve-Path -LiteralPath $taskRoot).ProviderPath
        if ($resolved -ne $taskRoot -or [IO.Path]::GetDirectoryName($resolved) -ne $qaParent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing cleanup outside owned Store QA root.' }
        if (Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }) { throw 'Refusing cleanup through a reparse point.' }
        Remove-Item -LiteralPath $resolved -Recurse -Force
        Write-Output 'Owned synthetic databases, historical source and probe binaries removed.'
    } elseif (!$passed) { Write-Warning ('Synthetic QA artifacts retained: ' + $taskRoot) }
}
