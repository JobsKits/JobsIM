#requires -Version 7.4
# Created by Jobs on 2026年11月18日，星期三.
# Host-isolated function tests only; never launch, stop or undeploy a user instance.
[CmdletBinding()]
param([string]$ReportDirectory='')
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$Repo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../../..'))
$source=Join-Path $Repo '部署/Windows/Deployment/【Windows】部署.ps1/【Windows】部署.ps1'
$other=Join-Path $Repo '部署/Windows/Undeployment/【Windows】反部署.ps1/【Windows】反部署.ps1'
$errors=$null;$tokens=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($source,[ref]$tokens,[ref]$errors)
if($errors.Count){throw ($errors|Out-String)}
foreach($node in $ast.FindAll({param($entry) $entry -is [Management.Automation.Language.FunctionDefinitionAst]},$false)){
    . ([ScriptBlock]::Create($node.Extent.Text))
}
$fixtureBase=if($IsWindows){[IO.Path]::GetTempPath()}else{Join-Path $HOME 'Library/Application Support/JobsIM/Regression/WindowsFunctionFixtures'}
$fixture=Join-Path $fixtureBase ('JobsIM-Windows-Undo-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($fixture)|Out-Null
$results=[Collections.Generic.List[object]]::new()
$script:Origin=$null;$script:Lock=$null;$script:FzfExecutable='';$script:Mode='local';$script:PublicURL='https://127.0.0.1:9443';$script:HttpsPort=9443;$script:RepoId='host-fixture';$script:ToolArch='amd64';$script:DatabaseURLs=@{};$script:Roles=@('identity','directory','conversation','message','media','control','sync','journal');$script:DataChoice='clear'
# Explicit host shims replace only Windows SID/ACL and process probes, not resource/undo/backup decisions.
function Get-CurrentOwnerSID{return 'HOST-FIXTURE-SID'}
# Actual host inode/uid/link metadata stands in for Windows FILE_ID_INFO/SID, which is never executed here.
if(!$IsWindows){
    function Get-ResourceIdentity([string]$Path){
        $value=& /usr/bin/stat -f '%d:%i|%u|%l' $Path
        if($LASTEXITCODE -ne 0){throw 'Host identity probe failed.'}
        $parts=([string]$value).Split('|');$item=Get-Item -LiteralPath $Path -Force
        if(!$item.PSIsContainer -and [long]$parts[2] -ne 1){throw 'Hard-linked host file refused.'}
        return [pscustomobject]@{objectId=$parts[0];ownerSid=$parts[1];links=$(if($item.PSIsContainer){1L}else{[long]$parts[2]});attributes=$(if($item.PSIsContainer){16}else{128})}
    }
}
$script:HostResourceIdentity=(Get-Item Function:\Get-ResourceIdentity).ScriptBlock
function Protect-Directory([string]$Directory){Assert-PlainPath $Directory;[IO.Directory]::CreateDirectory($Directory)|Out-Null}
function Require-Stopped{}
function Get-OwnedNats{return $null}
function Stop-Backend{}
function Stop-Nats{}
# SQL size metadata is explicit synthetic input; no host test connects to any PG server.
function Get-DatabaseBudgetBytes([string]$Role,[string]$OutputFile){return 64L}
function Test-Case([string]$Name,[scriptblock]$Body){
    try{& $Body;$results.Add([pscustomobject]@{name=$Name;status='passed'})}
    catch{$results.Add([pscustomobject]@{name=$Name;status='failed';error=$_.Exception.Message;position=$_.InvocationInfo.PositionMessage})}
}
function Assert-True([bool]$Value,[string]$Message){if(!$Value){throw $Message}}
function Assert-Rejected([scriptblock]$Body){$rejected=$false;try{& $Body}catch{$rejected=$true};if(!$rejected){throw 'Expected the protected action to reject.'}}
function New-FixtureState([string]$Name,[bool]$Existing=$false){
    $case=Join-Path $fixture $Name
    [IO.Directory]::CreateDirectory($case)|Out-Null
    $script:State=Join-Path $case 'instance';$script:Parent=$case;$script:Backups=Join-Path $case 'backups';$script:Recovery=Join-Path $case 'recovery';$script:StateInitialExisted=$Existing;$script:Origin=$null;$script:ExitStatus=0;$script:DataChoice='clear'
    [IO.Directory]::CreateDirectory($State)|Out-Null
    Initialize-Origin
    [IO.Directory]::CreateDirectory((Join-Path $State 'Application'))|Out-Null
    [IO.File]::WriteAllText((Join-Path $State 'Application/message.txt'),'fixture-message')
    Write-PrivateJson (Join-Path $State 'owner.json') @{project=$Repo;user='HOST-FIXTURE-SID'}
    Seal-Origin
}
Test-Case 'PowerShell pair parses and lifecycle bodies are identical except default action' {
    $right=Get-Content -LiteralPath $other -Raw
    Assert-True ($right.Replace("[string]`$Action = 'undeploy'","[string]`$Action = 'run'") -ceq (Get-Content -LiteralPath $source -Raw)) 'Pair bodies drifted.'
    $e=$null;$t=$null;[void][Management.Automation.Language.Parser]::ParseFile($other,[ref]$t,[ref]$e)
    Assert-True (!$e.Count) 'Undeployment parser failed.'
}
Test-Case 'clear performs an actual local inverse and retains external backup/key files' {
    New-FixtureState 'clear'
    [IO.Directory]::CreateDirectory($Backups)|Out-Null;[IO.Directory]::CreateDirectory($Recovery)|Out-Null
    [IO.File]::WriteAllText((Join-Path $Backups 'old.cms'),'original-archive')
    [IO.File]::WriteAllText((Join-Path $Recovery 'key.pem'),'matching-key-fixture')
    Undeploy-Instance
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'Application/message.txt'))) 'Owned business file was not removed.'
    Assert-True ((Test-Path -LiteralPath (Join-Path $State 'owner.json')) -and $script:ExitStatus -eq 2) 'Ownership proof/partial result was not retained.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $Backups 'old.cms') -Raw) -ceq 'original-archive') 'Old backup changed.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $Recovery 'key.pem') -Raw) -ceq 'matching-key-fixture') 'Recovery key changed.'
    $audit=@(Get-ChildItem -LiteralPath $Backups -Filter 'undeployment-*.json')
    Assert-True ($audit.Count -eq 1) 'Missing external inverse audit.'
    Assert-True ((Get-Content -LiteralPath $audit[0].FullName -Raw|ConvertFrom-Json).externalPostgreSQL -ceq 'retained-not-owned') 'External PG was falsely reported removed.'
}
Test-Case 'original empty directory survives inverse cleanup' {
    New-FixtureState 'empty-before' $true
    Undeploy-Instance
    Assert-True ((Test-Path -LiteralPath $State) -and (Test-Path -LiteralPath (Join-Path $State 'owner.json')) -and !(Test-Path -LiteralPath (Join-Path $State 'Application/message.txt'))) 'Original directory/proof was deleted or business was retained.'
}
Test-Case 'legacy state without origin refuses destructive cleanup' {
    New-FixtureState 'legacy'
    [IO.File]::Delete((Join-Path $State 'origin.json'))
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Legacy data was removed.'
}
Test-Case 'unknown resource added after seal blocks every removal' {
    New-FixtureState 'unknown'
    [IO.File]::WriteAllText((Join-Path $State 'unrelated.txt'),'keep')
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Known file removed before conflict gate.'
}
Test-Case 'changed owned file is preserved with original origin' {
    New-FixtureState 'changed'
    [IO.File]::WriteAllText((Join-Path $State 'Application/message.txt'),'later-user-change')
    Assert-Rejected {Undeploy-Instance}
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Application/message.txt') -Raw) -ceq 'later-user-change') 'Later content lost.'
}
Test-Case 'symbolic directory and file resources refuse traversal' {
    New-FixtureState 'link'
    $outside=Join-Path $Parent 'outside';[IO.Directory]::CreateDirectory($outside)|Out-Null
    [IO.File]::WriteAllText((Join-Path $outside 'keep.txt'),'outside')
    New-Item -ItemType SymbolicLink -Path (Join-Path $State 'Application/redirect') -Target $outside|Out-Null
    Assert-Rejected {Get-ResourceSnapshot}
    Assert-True ((Get-Content -LiteralPath (Join-Path $outside 'keep.txt') -Raw) -ceq 'outside') 'Link escaped state.'
}
Test-Case 'missing original backup blocks interrupted inverse retry' {
    New-FixtureState 'lost-backup';$script:DataChoice='backup'
    $origin=Read-Origin
    Write-PrivateJson (Join-Path $State 'undo.json') @{originHash=$origin.snapshotHash;choice='backup';backup=(Join-Path $Backups 'missing.cms');backupHash='00';removed=@();status='removing'}
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Cleanup continued without its backup.'
}
Test-Case 'interrupted cleanup resumes only its unchanged remaining entries' {
    New-FixtureState 'resume'
    $origin=Read-Origin;$first=@($origin.snapshot|Where-Object{$_.type -eq 'file'})[0]
    [IO.File]::Delete((Join-Path $State $first.path))
    Write-PrivateJson (Join-Path $State 'undo.json') @{originHash=$origin.snapshotHash;choice='clear';backup='';backupHash='';removed=@($first.path);status='removing'}
    Undeploy-Instance
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) -and (Test-Path -LiteralPath (Join-Path $State 'owner.json')) -and $script:ExitStatus -eq 2) 'Retry did not finish its bounded business inverse.'
}
Test-Case 'source mutation during backup prevents all source cleanup' {
    New-FixtureState 'backup-race';$script:DataChoice='backup'
    function Backup-Instance([switch]$ForUndeploy){
        [IO.Directory]::CreateDirectory($Backups)|Out-Null;$archive=Join-Path $Backups 'verified.cms';[IO.File]::WriteAllText($archive,'verified-fixture')
        [IO.File]::WriteAllText((Join-Path $State 'Application/message.txt'),'changed-after-backup')
        return [pscustomobject]@{archive=$archive;sha256=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash}
    }
    Assert-Rejected {Undeploy-Instance}
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Application/message.txt') -Raw) -ceq 'changed-after-backup') 'Changed source removed.'
    Assert-True (Test-Path -LiteralPath (Join-Path $Backups 'verified.cms')) 'Verified archive was discarded.'
}
Test-Case 'backup verification failure preserves source and creates no undo journal' {
    New-FixtureState 'backup-failed';$script:DataChoice='backup'
    function Backup-Instance([switch]$ForUndeploy){throw 'injected verification failure'}
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Source deleted on backup failure.'
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'undo.json'))) 'Failed backup authorized a cleanup.'
}
Test-Case 'exclusive transaction lock rejects a concurrent writer' {
    New-FixtureState 'lock';Enter-Transaction
    try{Assert-Rejected {$second=[IO.File]::Open($LockPath,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None);$second.Dispose()}}
    finally{$Lock.Dispose();$script:Lock=$null}
}
Test-Case 'server configuration rejects credential/path/port confusion and accepts explicit HTTPS origin' {
    foreach($address in @('http://im.example.test:9443','https://user:secret@im.example.test:9443','https://im.example.test:9443/path','https://im.example.test:9443?token=x','https://127.0.0.1:9443','https://im.example.test:443')){Assert-Rejected {Assert-ServerURL $address}}
    Assert-True ((Assert-ServerURL 'https://im.example.test:9443').Host -ceq 'im.example.test') 'Valid explicit server origin refused.'
}
Test-Case 'asset wire contract rejects forged checksum and foreign release source' {
    Assert-Rejected {Convert-AssetRow "fzf`twindows`tamd64`t0.74.0`tfzf.zip`tbad`thttps://github.com/junegunn/fzf/releases/download/v0.74.0/fzf.zip`tzip" 'fzf'}
    Assert-Rejected {Convert-AssetRow ("fzf`twindows`tamd64`t0.74.0`tfzf.zip`t"+('a'*64)+"`thttps://evil.example/fzf.zip`tzip") 'fzf'}
    $value=Convert-AssetRow "postgres`twindows`tamd64`t18.7`t-`t-`t-`tunsupported" 'postgres'
    Assert-True ($value.kind -ceq 'unsupported') 'Unsupported platform must not fabricate an executable.'
}
Test-Case 'C# native process bridge compiles on host without executing Win32 calls' {
    $definition=$ast.FindAll({param($entry)$entry -is [Management.Automation.Language.StringConstantExpressionAst] -and $entry.Value.Contains('public static class JobsIMOwnedConsole')},$true)|Select-Object -First 1
    Add-Type -TypeDefinition $definition.Value
    Assert-True ($null -ne ('JobsIMOwnedConsole' -as [type])) 'Native bridge did not compile.'
}
Test-Case 'independent recovery journal and matching credentials survive clear and repeat inverse' {
    New-FixtureState 'journal-retained'
    [IO.Directory]::CreateDirectory((Join-Path $State 'Postgres/journal'))|Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $State 'Postgres/business'))|Out-Null
    [IO.File]::WriteAllText((Join-Path $State 'Postgres/journal/control'),'independent-journal')
    [IO.File]::WriteAllText((Join-Path $State 'Postgres/journal.admin.password'),'matching-journal-secret-fixture')
    [IO.File]::WriteAllText((Join-Path $State 'Postgres/business/user-data'),'owned-business')
    Seal-Origin
    Undeploy-Instance
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'Postgres/business'))) 'Owned business cluster was not removed.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Postgres/journal/control') -Raw) -ceq 'independent-journal') 'Independent journal was removed.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Postgres/journal.admin.password') -Raw) -ceq 'matching-journal-secret-fixture') 'Journal recovery credential removed.'
    Undeploy-Instance
}
Test-Case 'foreign owner and path-injected managed PG profiles are refused' {
    New-FixtureState 'pg-profile'
    [IO.Directory]::CreateDirectory((Join-Path $State 'Postgres'))|Out-Null
    Write-PrivateJson (Join-Path $State 'Postgres/journal.connection-profile.json') @{schema=1;route='managed';project='another-project';state=$State;bin=$fixture;businessPort=17432;journalPort=17433;urls=@{}}
    Assert-Rejected {Read-ManagedPgProfile}
}
Test-Case 'unknown transaction-lock file is a conflict and is never implicitly deleted' {
    New-FixtureState 'unknown-lock'
    [IO.File]::WriteAllText((Join-Path $State 'transaction.lock'),'not-our-current-sidecar-lock')
    Assert-Rejected {Undeploy-Instance}
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'transaction.lock') -Raw) -ceq 'not-our-current-sidecar-lock') 'Unrecorded legacy lock removed.'
}
Test-Case 'Yes does not bypass the independent destructive choice' {
    New-FixtureState 'cancel';$script:Yes=$true
    function Select-KeyboardChoice([string[]]$Items,[string]$Header){return 'cancel'}
    Assert-True (!(Confirm-Undeploy)) 'Yes bypassed cancellation.'
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Cancellation changed data.'
}
Test-Case 'final undeploy confirmation requires an independent empty line even with Yes' {
    New-FixtureState 'final-confirmation';$script:Yes=$true
    function Select-KeyboardChoice([string[]]$Items,[string]$Header){if($Items[0] -like 'backup|*'){return 'backup'};return 'execute'}
    foreach($answer in @('YES',' ','cancel')){
        $script:confirmationFixture=$answer
        function Read-UndeployConfirmation{return $script:confirmationFixture}
        Assert-True (!(Confirm-Undeploy)) 'A nonempty final line authorized cleanup.'
    }
    function Read-UndeployConfirmation{return $null}
    Assert-True (!(Confirm-Undeploy)) 'EOF authorized cleanup.'
    function Read-UndeployConfirmation{return ''}
    Assert-True (Confirm-Undeploy) 'An explicit empty final line was refused.'
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Confirmation itself mutated source.'
}
Test-Case 'same-byte file replacement and directory replacement both preserve all source' {
    foreach($kind in @('file','directory','state')){
        New-FixtureState ('identity-replace-'+$kind)
        $file=Join-Path $State 'Application/message.txt'
        $bytes=[IO.File]::ReadAllBytes($file)
        if($kind -eq 'file'){
            [IO.File]::Move($file,(Join-Path $Parent 'original-message'))
            [IO.File]::WriteAllBytes($file,$bytes)
        }elseif($kind -eq 'directory'){
            [IO.Directory]::Move((Join-Path $State 'Application'),(Join-Path $Parent 'original-application'))
            [IO.Directory]::CreateDirectory((Join-Path $State 'Application'))|Out-Null
            [IO.File]::WriteAllBytes($file,$bytes)
        }else{
            $old=Join-Path $Parent 'original-state';[IO.Directory]::Move($State,$old)
            Copy-Item -LiteralPath $old -Destination $State -Recurse
        }
        Assert-Rejected {Undeploy-Instance}
        Assert-True ([Convert]::ToHexString([IO.File]::ReadAllBytes($file)) -ceq [Convert]::ToHexString($bytes)) 'Same-byte replacement was deleted.'
        Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'undo.json'))) 'Object replacement entered cleanup.'
    }
}
Test-Case 'owner and link-count drift reject actual undeploy before any deletion' {
    foreach($kind in @('owner','links')){
        New-FixtureState ('identity-drift-'+$kind)
        $script:identityDriftKind=$kind
        function Get-ResourceIdentity([string]$Path){
            $value=& $script:HostResourceIdentity $Path
            if($Path -ceq (Join-Path $State 'Application/message.txt')){
                if($script:identityDriftKind -eq 'owner'){$value.ownerSid='DIFFERENT-OWNER'}else{$value.links=2L}
            }
            return $value
        }
        Assert-Rejected {Undeploy-Instance}
        Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Identity drift deleted source.'
        Remove-Item Function:\Get-ResourceIdentity
    }
}
Test-Case 'actual host hard link and identity-free legacy ledger both block cleanup' {
    New-FixtureState 'hardlink-identity'
    New-Item -ItemType HardLink -Path (Join-Path $Parent 'outside-hardlink') -Target (Join-Path $State 'Application/message.txt')|Out-Null
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Hard-linked file was deleted.'
    New-FixtureState 'legacy-no-identity'
    $origin=Read-Origin;$origin.PSObject.Properties.Remove('rootIdentity')
    Write-PrivateJson (Join-Path $State 'origin.json') $origin
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Identity-free ledger authorized cleanup.'
}
Test-Case 'Windows object identity bridge compiles without executing native APIs' {
    Initialize-ResourceIdentity
    Assert-True ($null -ne ('JobsIMResourceIdentity' -as [type])) 'File identity bridge did not compile.'
}
Test-Case 'Quick tunnel launch has empty session config and explicit trusted origin CA/SAN' {
    New-FixtureState 'tunnel-arguments'
    [IO.Directory]::CreateDirectory((Join-Path $State 'Secrets'))|Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $State 'Logs'))|Out-Null
    $script:Cloudflared=Join-Path $Parent 'cloudflared-fixture.exe';[IO.File]::WriteAllText($Cloudflared,'not-executed')
    $script:captured=@()
    function Protect-SecretFile([string]$Path){Assert-PlainPath $Path}
    function Start-ConsoleProcess([string]$Executable,[string[]]$Arguments,[string]$Directory,[hashtable]$Environment){
        $script:captured=$Arguments
        $log=$Arguments[[Array]::IndexOf($Arguments,'--logfile')+1]
        [IO.File]::WriteAllText($log,'https://host-fixture.trycloudflare.com')
        return [pscustomobject]@{Id=999999;StartTime=[DateTime]::UtcNow;HasExited=$false}
    }
    $url=Start-QuickTunnel
    Assert-True ($url -ceq 'https://host-fixture.trycloudflare.com') 'Tunnel did not use the current log address.'
    Assert-True ('--origin-ca-pool' -in $captured -and '--origin-server-name' -in $captured -and 'localhost' -in $captured) 'Missing origin CA/SAN verification.'
    Assert-True ('--no-tls-verify' -notin $captured) 'Tunnel verification was bypassed.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Secrets/quick-session.yml') -Raw).Trim() -ceq '{}') 'Existing tunnel account config leaked into session.'
    $script:TunnelProcess=$null
}
Test-Case 'encrypted backup decrypts to identical bytes and retains its matching external key' {
    New-FixtureState 'cms-backup'
    $script:OpenSSL=(Get-Command openssl -CommandType Application|Select-Object -First 1).Source
    $script:Tar=(Get-Command tar -CommandType Application|Select-Object -First 1).Source
    $script:PgDump='fixture-pg-dump';$script:PgRestore='fixture-pg-restore';$script:NatsStore=Join-Path $State 'Application/JetStream'
    $nativeTool=(Get-Command Invoke-Tool).ScriptBlock
    function Database-Environment([string]$Role){return 'postgresql://fixture:fixture@127.0.0.1:1/fixture'}
    function Invoke-Tool([string]$Exe,[string[]]$Arguments,[hashtable]$Environment=@{},[string]$OutputFile='',[string]$WorkingDirectory=$Repo){
        if($Exe -ceq 'fixture-pg-dump'){
            $target=($Arguments|Where-Object{$_.StartsWith('--file=')}).Substring(7)
            [IO.File]::WriteAllText($target,'explicit synthetic PG fixture; real pg_dump is not being tested');return
        }
        if($Exe -ceq 'fixture-pg-restore'){[IO.File]::WriteAllText($OutputFile,'explicit injected pg_restore list');return}
        & $nativeTool $Exe $Arguments $Environment $OutputFile $WorkingDirectory
    }
    $result=Backup-Instance -ForUndeploy
    Assert-True ((Get-FileHash -LiteralPath $result.archive -Algorithm SHA256).Hash -ceq $result.sha256) 'Encrypted archive not bound to its bytes.'
    Assert-True ((Get-FileHash -LiteralPath $result.recoveryKey -Algorithm SHA256).Hash -ceq $result.recoveryKeyHash) 'Matching key not bound to archive verification.'
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Operational backup removed source.'
}
Test-Case 'server certificate SAN and matching key are checked before copying originals' {
    New-FixtureState 'server-tls'
    $script:OpenSSL=(Get-Command openssl -CommandType Application|Select-Object -First 1).Source
    $input=Join-Path $Parent 'input-certificates';Protect-Directory $input
    $ca=Join-Path $input 'ca.crt';$key=Join-Path $input 'ca.key';$server=Join-Path $input 'server.crt';$serverKey=Join-Path $input 'server.key';$csr=Join-Path $input 'server.csr';$ext=Join-Path $input 'server.ext'
    Invoke-Tool $OpenSSL @('req','-x509','-newkey','rsa:2048','-nodes','-days','7','-keyout',$key,'-out',$ca,'-subj','/CN=JobsIM Host Fixture CA','-addext','basicConstraints=critical,CA:TRUE','-addext','keyUsage=critical,keyCertSign,cRLSign','-addext','subjectKeyIdentifier=hash')
    Invoke-Tool $OpenSSL @('req','-new','-newkey','rsa:2048','-nodes','-keyout',$serverKey,'-out',$csr,'-subj','/CN=im.example.test')
    [IO.File]::WriteAllText($ext,"subjectAltName=DNS:im.example.test`nbasicConstraints=critical,CA:FALSE`nkeyUsage=critical,digitalSignature,keyEncipherment`nextendedKeyUsage=serverAuth`nsubjectKeyIdentifier=hash`nauthorityKeyIdentifier=keyid,issuer`n")
    Invoke-Tool $OpenSSL @('x509','-req','-in',$csr,'-CA',$ca,'-CAkey',$key,'-CAcreateserial','-days','7','-out',$server,'-extfile',$ext)
    $saved=@{};foreach($name in @('JOBS_IM_SERVER_CA_FILE','JOBS_IM_SERVER_CERT_FILE','JOBS_IM_SERVER_KEY_FILE')){$saved[$name]=[Environment]::GetEnvironmentVariable($name)}
    try{
        $env:JOBS_IM_SERVER_CA_FILE=$ca;$env:JOBS_IM_SERVER_CERT_FILE=$server;$env:JOBS_IM_SERVER_KEY_FILE=$serverKey
        $tls=Join-Path $State 'TLS';Protect-Directory $tls
        function Protect-SecretFile([string]$Path){Assert-PlainPath $Path}
        $script:PublicURL='https://wrong.example.test:9443'
        Assert-Rejected {Prepare-ServerTLS $tls}
        Assert-True (@(Get-ChildItem -LiteralPath $tls -Force).Count -eq 0) 'SAN failure changed the destination.'
        $script:PublicURL='https://im.example.test:9443';$before=(Get-FileHash -LiteralPath $serverKey -Algorithm SHA256).Hash
        Prepare-ServerTLS $tls
        Assert-True ((Get-FileHash -LiteralPath (Join-Path $tls 'server.key') -Algorithm SHA256).Hash -ceq $before) 'Copied key differs from its explicit source.'
        Assert-True ((Get-FileHash -LiteralPath $serverKey -Algorithm SHA256).Hash -ceq $before) 'Original server key changed.'
    }finally{foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')};$script:PublicURL='https://127.0.0.1:9443'}
}
Test-Case 'an existing external-data instance cannot silently switch to a new managed database' {
    New-FixtureState 'no-implicit-database-switch'
    [IO.File]::WriteAllText((Join-Path $State 'Application/instance.json'),'existing-instance-marker')
    function Select-KeyboardChoice([string[]]$Items,[string]$Header){return 'managed'}
    Assert-Rejected {Select-DatabaseRoute}
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Application/instance.json') -Raw) -ceq 'existing-instance-marker') 'Existing instance data was replaced.'
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'Postgres'))) 'A new cluster was created for an existing external instance.'
}
Test-Case 'clear preserves the exact instance secret, TLS, Secrets, journal and owner while removing other owned business bytes' {
    New-FixtureState 'all-recovery-material'
    $protected=@{
        'Application/instance.json'='{"deploymentId":"synthetic-deployment","secret":"synthetic-32-byte-identity-fixture"}'
        'TLS/ca.key'='synthetic-trust-key'
        'Secrets/operator.key'='synthetic-operator-key'
        'JournalDatabase/control'='synthetic-independent-journal'
    }
    foreach($relative in $protected.Keys){$target=Join-Path $State $relative;[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))|Out-Null;[IO.File]::WriteAllText($target,$protected[$relative])}
    [IO.Directory]::CreateDirectory((Join-Path $State 'Bin'))|Out-Null;[IO.File]::WriteAllText((Join-Path $State 'Bin/JobsIMd.exe'),'synthetic-owned-program')
    Seal-Origin
    Undeploy-Instance
    foreach($relative in $protected.Keys){Assert-True ((Get-Content -LiteralPath (Join-Path $State $relative) -Raw) -ceq $protected[$relative]) ('Recovery material changed: '+$relative)}
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) -and !(Test-Path -LiteralPath (Join-Path $State 'Bin'))) 'Owned business/program bytes were not removed.'
    Assert-True ((Test-Path -LiteralPath (Join-Path $State 'owner.json')) -and $script:ExitStatus -eq 2) 'Partial result/ownership proof is missing.'
    Assert-Rejected {Initialize-Origin}
}
Test-Case 'Quick descriptor uses system edge trust and direct descriptor retains its own CA contract' {
    $bootstrap=[pscustomobject]@{deploymentId='synthetic-deployment';protocolVersion='1'}
    try{
        $script:Mode='local';$script:AccessMode='quick';$script:PublicURL='https://synthetic.trycloudflare.com'
        $quick=New-ConnectionDescriptor $bootstrap 'origin-fixture-fingerprint'
        Assert-True ($quick.tlsTrust -ceq 'system' -and !$quick.ContainsKey('caSha256') -and $quick.originCaSha256 -ceq 'origin-fixture-fingerprint') 'Private origin CA was exposed as edge trust.'
        $script:Mode='server';$script:PublicURL='https://im.example.test:9443'
        $direct=New-ConnectionDescriptor $bootstrap 'direct-fixture-fingerprint'
        Assert-True ($direct.caSha256 -ceq 'direct-fixture-fingerprint' -and $direct.tlsTrust -ceq 'instance-ca' -and !$direct.ContainsKey('originCaSha256')) 'Direct CA descriptor contract changed.'
    }finally{$script:Mode='local';$script:AccessMode='loopback';$script:PublicURL='https://127.0.0.1:9443'}
}
Test-Case 'real Undeploy to Backup chain rejects a missing broker receipt before archive or deletion' {
    New-FixtureState 'broker-proof-failure';$script:DataChoice='backup'
    $script:OpenSSL=(Get-Command openssl -CommandType Application|Select-Object -First 1).Source;$script:Tar=(Get-Command tar -CommandType Application|Select-Object -First 1).Source
    $script:NatsStore=Join-Path $State 'Application/JetStream';[IO.Directory]::CreateDirectory($NatsStore)|Out-Null
    $script:PgDump='/usr/bin/false';$script:PgRestore='/usr/bin/false';Seal-Origin
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Broker proof failure removed source.'
    Assert-True (@(Get-ChildItem -LiteralPath $Backups -Filter '*.cms').Count -eq 0) 'Broker proof failure reached encryption.'
    Assert-True (!(Test-Path -LiteralPath (Join-Path $State 'undo.json'))) 'Broker proof failure committed undo responsibility.'
}
Test-Case 'native failing dump exit propagates through actual Undeploy and preserves source' {
    New-FixtureState 'dump-failure';$script:DataChoice='backup'
    $script:OpenSSL=(Get-Command openssl -CommandType Application|Select-Object -First 1).Source;$script:Tar=(Get-Command tar -CommandType Application|Select-Object -First 1).Source
    $script:NatsStore=Join-Path $State 'Application/JetStream';$script:PgDump=(Get-Command false -CommandType Application|Select-Object -First 1).Source;$script:PgRestore=$PgDump
    function Database-Environment([string]$Role){return 'postgresql://fixture:fixture@127.0.0.1:1/fixture'}
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Dump nonzero exit removed source.'
    Assert-True (@(Get-ChildItem -LiteralPath $Backups -Filter '*.cms').Count -eq 0) 'Dump failure reached encryption.'
}
Test-Case 'native failing restore-list exit propagates through actual Undeploy before archive' {
    New-FixtureState 'restore-list-failure';$script:DataChoice='backup'
    $script:OpenSSL=(Get-Command openssl -CommandType Application|Select-Object -First 1).Source;$script:Tar=(Get-Command tar -CommandType Application|Select-Object -First 1).Source
    $script:NatsStore=Join-Path $State 'Application/JetStream';$script:PgDump='fixture-dump-only';$script:PgRestore=(Get-Command false -CommandType Application|Select-Object -First 1).Source
    $nativeTool=(Get-Command Invoke-Tool).ScriptBlock
    function Database-Environment([string]$Role){return 'postgresql://fixture:fixture@127.0.0.1:1/fixture'}
    function Invoke-Tool([string]$Exe,[string[]]$Arguments,[hashtable]$Environment=@{},[string]$OutputFile='',[string]$WorkingDirectory=$Repo){
        if($Exe -ceq 'fixture-dump-only'){$target=($Arguments|Where-Object{$_.StartsWith('--file=')}).Substring(7);[IO.File]::WriteAllText($target,'explicit synthetic dump bytes');return}
        & $nativeTool $Exe $Arguments $Environment $OutputFile $WorkingDirectory
    }
    Assert-Rejected {Undeploy-Instance}
    Assert-True (Test-Path -LiteralPath (Join-Path $State 'Application/message.txt')) 'Restore-list nonzero exit removed source.'
    Assert-True (@(Get-ChildItem -LiteralPath $Backups -Filter '*.cms').Count -eq 0) 'Restore-list failure reached encryption.'
}
Test-Case 'backup completeness requires broker plus all eight dump/list pairs and rejects duplicate or escaping members' {
    New-FixtureState 'archive-required-set'
    $required=@('broker-manifest.json','database-connections.json','backup-files.sha256.json')
    foreach($role in $Roles){$required+=@(($role+'.dump'),($role+'.list'))}
    $manifest=Join-Path $Parent 'decoded.manifest'
    [IO.File]::WriteAllLines($manifest,@($required|Where-Object{$_ -ne 'broker-manifest.json'}))
    Assert-Rejected {Assert-BackupManifest $manifest $required}
    [IO.File]::WriteAllLines($manifest,$required);Assert-BackupManifest $manifest $required
    [IO.File]::WriteAllLines($manifest,($required+@('journal.dump')));Assert-Rejected {Assert-BackupManifest $manifest $required}
    [IO.File]::WriteAllLines($manifest,($required+@('../outside')));Assert-Rejected {Assert-BackupManifest $manifest $required}
}
Test-Case 'local mode defaults to loopback without public confirmation or cloudflared preparation' {
    New-FixtureState 'loopback-default';$script:Mode='';$script:ExplicitState=$true;$script:selectionCount=0;$script:ActiveBackend=$null;$script:TunnelProcess=$null
    function Test-ToolCandidate([string]$Path,[string]$ToolName,[string]$Target=''){return $true}
    function Install-VerifiedTool($Asset){throw 'Local menu must not download in this explicit host fixture.'}
    function Select-KeyboardChoice([string[]]$Items,[string]$Header){
        $script:selectionCount++
        if($selectionCount -eq 1){return 'local'}
        Assert-True ($Items[0].StartsWith('loopback|')) 'Public access was the default.'
        return 'loopback'
    }
    Select-RunMode
    Assert-True ($Mode -ceq 'local' -and $AccessMode -ceq 'loopback' -and $selectionCount -eq 2) 'Loopback selection required public setup.'
    $script:ToolArch='amd64'
}
Test-Case 'Quick start failure and late exit recover strict local origin while explicit retry can publish a new public origin' {
    New-FixtureState 'quick-recover';$script:Mode='local';$script:AccessMode='loopback';$script:Cloudflared='fixture-tool';$script:PublicURL='https://127.0.0.1:9443';$script:BackendArguments=@('fixture-only');$script:BackendEnvironment=@{};$script:ExpectedDeploymentId='fixture-deployment';$script:CaFingerprint='fixture-origin-ca';$script:TunnelProcess=$null;$script:ActiveBackend=[pscustomobject]@{HasExited=$false;ExitCode=0};$script:RecoveringTunnel=$false
    Write-PrivateJson (Join-Path $State 'bootstrap.json') @{deploymentId='fixture-deployment';protocolVersion='1'}
    $script:strictChecks=0;$script:qrRetired=0
    function Stop-Tunnel{}
    function Invoke-Capture([string]$Exe,[string[]]$Arguments,[int]$Seconds=30){return [pscustomobject]@{code=0;text='origin-ca-pool origin-server-name'}}
    function Start-BackendProcess([string[]]$Arguments,[hashtable]$Environment){Write-PrivateJson (Join-Path $State 'bootstrap.json') @{deploymentId='fixture-deployment';protocolVersion='1'};return [pscustomobject]@{HasExited=$false;ExitCode=0}}
    function Wait-BackendReady($Server,[bool]$Public=$false){$script:strictChecks++}
    function New-AccessQR{}
    function Expire-AccessQR{$script:qrRetired++}
    function Start-QuickTunnel{throw 'explicit injected network/start failure'}
    Assert-True (!(Try-EnableQuick)) 'Failed public setup was reported successful.'
    Assert-True ($AccessMode -ceq 'loopback' -and $PublicURL -ceq 'https://127.0.0.1:9443' -and !$ActiveBackend.HasExited) 'Healthy local service was discarded.'
    function Start-QuickTunnel{return 'https://fresh-fixture.trycloudflare.com'}
    Assert-True (Try-EnableQuick) 'Explicit successful retry did not publish.'
    Assert-True ($BackendEnvironment.JOBS_IM_PUBLIC_URL -ceq 'https://fresh-fixture.trycloudflare.com') 'BFF origin did not follow its real public URL.'
    $public=Get-Content -LiteralPath (Join-Path $State 'connection.json') -Raw|ConvertFrom-Json
    Assert-True ($public.tlsTrust -ceq 'system') 'Public descriptor used private-origin trust.'
    $script:TunnelProcess=[pscustomobject]@{HasExited=$true};Recover-TunnelToLocal 'explicit late-exit fixture'
    $local=Get-Content -LiteralPath (Join-Path $State 'connection.json') -Raw|ConvertFrom-Json
    Assert-True ($AccessMode -ceq 'loopback' -and $local.caSha256 -ceq 'fixture-origin-ca' -and !$ActiveBackend.HasExited -and $strictChecks -ge 3 -and $qrRetired -ge 3) 'Late failure did not retire public access and retain strict local service.'
    Assert-True ((Get-Content -LiteralPath (Join-Path $State 'Application/message.txt') -Raw) -ceq 'fixture-message') 'Origin switching changed message data.'
}
Test-Case 'a stop request fences a late restart only for its exact host PID and start time' {
    New-FixtureState 'late-stop-fence';$script:RunId='current-explicit-run'
    Write-PrivateJson (Join-Path $State 'stop-request.json') @{pid=$PID;started=(Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks.ToString();runId=$RunId;nonce='fixture'}
    Assert-Rejected {Assert-NoHostStop}
    Write-PrivateJson (Join-Path $State 'stop-request.json') @{pid=$PID;started='0';runId=$RunId;nonce='different-host-fixture'}
    Assert-NoHostStop
    Write-PrivateJson (Join-Path $State 'stop-request.json') @{pid=$PID;started=(Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks.ToString();runId='previous-run-in-the-same-pwsh-host';nonce='old-run-fixture'}
    Assert-NoHostStop
}
Test-Case 'disk reserve accepts only actual additional bytes above four GiB and rejects negative or overflowing budgets' {
    New-FixtureState 'actual-byte-budget';$script:fakeFree=4L*1024*1024*1024+20L
    function Get-FreeDiskBytes([string]$Path){return $script:fakeFree}
    Assert-DiskBudget $State 20
    Assert-Rejected {Assert-DiskBudget $State 21}
    Assert-Rejected {Assert-DiskBudget $State -1}
    Assert-Rejected {Assert-DiskBudget $State ([long]::MaxValue)}
    $script:fakeFree=4L*1024*1024*1024-1L
    Assert-Rejected {Assert-DiskBudget $State 0}
}
$summary=[pscustomobject]@{scope='host-isolated PowerShell functions; Windows SID/ACL/process explicitly shimmed; real host inode/uid/link metadata substitutes Windows file identity';passed=@($results|Where-Object{$_.status -eq 'passed'}).Count;failed=@($results|Where-Object{$_.status -eq 'failed'}).Count;cases=$results.ToArray();windowsNativeExecuted=$false;fixture=$fixture}
if($ReportDirectory){[IO.Directory]::CreateDirectory($ReportDirectory)|Out-Null;[IO.File]::WriteAllText((Join-Path $ReportDirectory 'summary.json'),($summary|ConvertTo-Json -Depth 8))}
$summary|ConvertTo-Json -Depth 8
if($summary.failed){exit 1}
