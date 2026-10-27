param()
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$parent=Join-Path $project 'runtime\qa\auth-session-sidecar'
$id=[guid]::NewGuid().ToString('N')
$root=Join-Path $parent ('run-sqlite-upgrade-'+$id)
$log=Join-Path $parent ('sqlite-upgrade-'+$id+'.log.txt')
foreach($path in @($parent,$root,(Join-Path $project '.go-build\x-development-entry'),(Join-Path $project '.go-build\mod'))){
 if(!(Test-Path -LiteralPath $path)){New-Item -ItemType Directory -Path $path -Force | Out-Null}
 if((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'SQLite test roots must be ordinary directories.'}
}
$names=@('TEMP','TMP','GOTMPDIR','GOCACHE','GOMODCACHE','GOWORK','GOPROXY','GOSUMDB','GOTOOLCHAIN','CGO_ENABLED','GOFLAGS','AKU_OWNED_PROFILE_TEST_ROOT','AKU_SQLITE_UPGRADE_ROOT')
$previous=@{};foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location -LiteralPath $project
try{
 $env:TEMP=$root;$env:TMP=$root;$env:GOTMPDIR=$root
 $env:GOCACHE=Join-Path $project '.go-build\x-development-entry';$env:GOMODCACHE=Join-Path $project '.go-build\mod'
 $env:GOWORK='off';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0';$env:GOFLAGS=''
 $env:AKU_OWNED_PROFILE_TEST_ROOT=$root;$env:AKU_SQLITE_UPGRADE_ROOT=$root
 Write-Output ('Offline SQLite upgrade verification: '+$id) | Tee-Object -FilePath $log
 & go list -m modernc.org/sqlite modernc.org/libc golang.org/x/sys github.com/abangkis/ai4u-inference-sdk-go 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Selected module readback failed.'}
 # Compile the same Store fixture with pre-upgrade and upgraded module locks.
 # git show is local/read-only; the original checkout and module files stay intact.
 $baseline='ae97e52a1447da94c07ac28e0488ef89d6b5f0c5'
 $oldMod=@(& git show ($baseline+':go.mod'));if($LASTEXITCODE -ne 0){throw 'Old module lock read failed.'}
 $oldSum=@(& git show ($baseline+':go.sum'));if($LASTEXITCODE -ne 0){throw 'Old checksum lock read failed.'}
 if(!($oldMod -match '^\s*modernc.org/sqlite v1.29.10$')){throw 'Expected pre-upgrade driver baseline unavailable; update the explicit compatibility fixture baseline.'}
 [IO.File]::WriteAllLines((Join-Path $root 'old.mod'),[string[]]$oldMod,[Text.UTF8Encoding]::new($false))
 [IO.File]::WriteAllLines((Join-Path $root 'old.sum'),[string[]]$oldSum,[Text.UTF8Encoding]::new($false))
 $oldImage=Join-Path $root 'old-driver.exe';$newImage=Join-Path $root 'new-driver.exe'
 & go build -mod=readonly -modfile (Join-Path $root 'old.mod') -o $oldImage ./scripts/fixtures/sqlite-driver-compat 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Old-driver compatibility fixture compilation failed.'}
 & go build -mod=readonly -o $newImage ./scripts/fixtures/sqlite-driver-compat 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'New-driver compatibility fixture compilation failed.'}
 $compat=Join-Path $root 'compat.db'
 & $oldImage old-seed $compat 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Old driver seed failed.'}
 & $newImage new-read-write $compat 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'New driver upgrade/read-write failed.'}
 & $oldImage old-reopen $compat 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Old driver downgrade/readback failed.'}
 & go test ./internal/store -run '^TestSQLiteCancellationReleasesDatabase$' -count=5 -timeout=90s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Repeated cancellation/file-release regression failed.'}
 & go test ./internal/store -count=1 -timeout=180s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Store/migration/compatibility regression failed.'}
 & go test -tags 'auth_session_dev auth_session_fixture' ./internal/store -run '^(Test.*ManualPilot|Test.*SessionPolicy|Test.*Checkpoint|Test.*Guard|TestStoreRecords|TestDatabaseCompatibility|TestStoreRejects)' -count=1 -timeout=120s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Synthetic policy/guard compatibility regression failed.'}
 & go test ./internal/ownedprocess ./internal/ownedrpc ./internal/ownedprofile -count=1 -timeout=60s -v 2>&1 | Tee-Object -FilePath $log -Append
 if($LASTEXITCODE -ne 0){throw 'Windows ownership/transport regression failed.'}
 Write-Output 'PASS: same-database old/new/old driver compatibility, public cancellation stress, file release/reopen, full Store suite, synthetic guards and owned-process primitives. No browser, real session transfer or user database/runtime access.' | Tee-Object -FilePath $log -Append
 Write-Output ('Evidence: '+$log)
}finally{
 Pop-Location
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
 if(Test-Path -LiteralPath $root){
  $resolved=(Resolve-Path -LiteralPath $root).ProviderPath
  if($resolved -ne $root -or [IO.Path]::GetDirectoryName($resolved) -ne $parent -or ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Refusing cleanup outside owned SQLite test root.'}
  if(Get-ChildItem -LiteralPath $resolved -Recurse -Force | Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}){throw 'Refusing cleanup through a reparse point.'}
  Remove-Item -LiteralPath $resolved -Recurse -Force
 }
}
