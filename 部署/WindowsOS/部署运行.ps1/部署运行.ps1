#requires -Version 7.4
# Created by Jobs on 2026年11月14日，星期六.
[CmdletBinding()]
param(
    [ValidateSet('check','plan','status','run','stop','backup','help')][string]$Action = 'check',
    [string]$State = $env:JOBS_IM_DEPLOY_STATE,
    [switch]$Yes,
    [switch]$NoBackup
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$RepoId = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($Repo))).Substring(0,16).ToLowerInvariant()
if (!$State) { $State = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) "JobsIM/Instances/$RepoId" }
$State = [IO.Path]::GetFullPath($State)
$Parent = Split-Path $State -Parent
$Backups = if ($env:JOBS_IM_BACKUP_DIR) { [IO.Path]::GetFullPath($env:JOBS_IM_BACKUP_DIR) } else { Join-Path $Parent "Backups/$(Split-Path $State -Leaf)" }
$Recovery = if ($env:JOBS_IM_RECOVERY_DIR) { [IO.Path]::GetFullPath($env:JOBS_IM_RECOVERY_DIR) } else { Join-Path $Parent "RecoveryKeys/$(Split-Path $State -Leaf)" }
$HttpsPort = if ($env:JOBS_IM_HTTPS_PORT) { [int]$env:JOBS_IM_HTTPS_PORT } else { 9443 }
$RolePort = if ($env:JOBS_IM_INTERNAL_PORT) { [int]$env:JOBS_IM_INTERNAL_PORT } else { 18101 }
$NatsPort = if ($env:JOBS_IM_NATS_PORT) { [int]$env:JOBS_IM_NATS_PORT } else { 4222 }
$NatsMaxFile = if ($env:JOBS_IM_NATS_MAX_FILE_BYTES) { [long]$env:JOBS_IM_NATS_MAX_FILE_BYTES } else { 536870912 }
$NatsMaxMemory = if ($env:JOBS_IM_NATS_MAX_MEMORY_BYTES) { [long]$env:JOBS_IM_NATS_MAX_MEMORY_BYTES } else { 33554432 }
$NatsConfig = Join-Path $State 'Secrets/nats.conf'
$NatsStore = Join-Path $State 'Application/JetStream'
$NatsTokenFile = if ($env:JOBS_IM_NATS_TOKEN_FILE) { [IO.Path]::GetFullPath($env:JOBS_IM_NATS_TOKEN_FILE) } else { Join-Path $State 'Secrets/nats.token' }
$NatsUrl = if ($env:JOBS_IM_NATS_URL) { $env:JOBS_IM_NATS_URL } else { "nats://127.0.0.1:$NatsPort" }
$roleSource=Get-Content -LiteralPath (Join-Path $Repo '代码/后端/Go/cmd/jobsimd/main.go') -Raw
$roleMatch=[regex]::Match($roleSource,'(?m)^\s*roles := \[\]string\{([^}]+)\}')
if (!$roleMatch.Success) { throw '无法读取当前Go角色顺序；不猜测端口。' }
$Services=@([regex]::Matches($roleMatch.Groups[1].Value,'"([a-z_]+)"')|ForEach-Object{$_.Groups[1].Value})
if ('sync' -notin $Services -or 'journal' -notin $Services) { throw '当前源码缺sync/journal角色。' }
$Roles=@($Services|Where-Object{$_ -ne 'governance'})
$RoleLast=$RolePort+$Services.Count-1
$NatsProcess=$null
$Lock = $null
function Assert-StatePath {
    $root = [IO.Path]::GetPathRoot($State)
    if ($State.TrimEnd('\','/') -eq $root.TrimEnd('\','/') -or $State -eq $HOME) { throw 'state必须是独立目录。' }
    foreach ($path in @($State,$Backups,$Recovery)) {
        if ($path -eq $Repo -or $path.StartsWith($Repo.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw '实例、归档与解密私钥必须位于源码之外。' }
        if ((Test-Path $path) -and ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '不接管符号链接或junction目录。' }
    }
    foreach ($path in @($Backups,$Recovery)) {
        if ($path -eq $State -or $path.StartsWith($State.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw '备份与恢复私钥必须位于state之外。' }
    }
    if ($Recovery -eq $Backups -or $Recovery.StartsWith($Backups.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or $Backups.StartsWith($Recovery.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw '加密归档和解密私钥不能在同一目录树中。' }
    if ($HttpsPort -le 1024 -or $HttpsPort -gt 65535 -or $RolePort -le 1024 -or $RoleLast -gt 65535 -or $NatsPort -le 1024 -or $NatsPort -gt 65535) { throw '动态角色及NATS端口须在1025～65535。' }
    if ($NatsMaxFile -lt 536870912 -or $NatsMaxMemory -lt 33554432) { throw 'NATS磁盘下限512MiB、内存下限32MiB，可显式调大。' }
    if ($NatsUrl -cne "nats://127.0.0.1:$NatsPort" -or (Split-Path $NatsTokenFile -Parent) -ne (Join-Path $State 'Secrets')) { throw '本入口仅管理loopback NATS，token须在本实例Secrets内。' }
}
function Assert-Owner {
    $owner = Join-Path $State 'owner.json'
    if (Test-Path $owner) {
        $record = Get-Content -LiteralPath $owner -Raw | ConvertFrom-Json
        if ($record.project -ne $Repo -or $record.user -ne [Security.Principal.WindowsIdentity]::GetCurrent().User.Value) { throw '收据不属于本项目或当前用户，拒绝接管。' }
    } elseif ((Test-Path $State) -and @(Get-ChildItem -LiteralPath $State -Force).Count -gt 0) { throw '非空state无归属收据，拒绝接管。' }
}
function Tool([string]$Name,[string]$Override) {
    if ($Override -match "://|[\r\n]") { throw '工具须为本地可执行路径，无效输入不回显。' }
    if ($Override) { if (!(Test-Path -LiteralPath $Override)) { throw "指定工具不存在：$Name；检查位置=$Override" }; return $Override }
    $command = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue
    if (!$command) { throw "缺少$Name；请先准备工具。本入口不全局安装。" }
    return $command.Source
}
function Invoke-Tool([string]$Exe,[string[]]$Arguments,[hashtable]$Environment=@{},[string]$OutputFile='',[string]$WorkingDirectory=$Repo) {
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName=$Exe; $start.WorkingDirectory=$WorkingDirectory; $start.UseShellExecute=$false
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    foreach ($key in $Environment.Keys) { $start.Environment[$key]=[string]$Environment[$key] }
    $process=[Diagnostics.Process]::Start($start)
    $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
    $process.WaitForExit(); $out=$stdout.GetAwaiter().GetResult(); $err=$stderr.GetAwaiter().GetResult()
    if ($process.ExitCode -ne 0) { throw "工具失败：$([IO.Path]::GetFileName($Exe))，退出码$($process.ExitCode)。不回显可能含连接信息的工具错误。" }
    if ($OutputFile) { [IO.File]::WriteAllText($OutputFile,$out) } elseif ($out.Trim()) { Write-Host $out.Trim() }
}
function Find-Tools {
    if (!$IsWindows) { throw '此入口仅支持Windows。当前宿主不能代表Windows验收。' }
    $script:NatsBin=Tool 'nats-server.exe' $env:JOBS_IM_NATS_BIN
    $script:Go=Tool 'go.exe' $env:JOBS_IM_GO
    $script:OpenSSL=Tool 'openssl.exe' $env:JOBS_IM_OPENSSL
    $script:Npm=Tool 'npm.cmd' ''
    $script:Node=Tool 'node.exe' ''
    $script:Curl=Tool 'curl.exe' ''
    $script:Tar=Tool 'tar.exe' ''
    $script:PgDump=Tool 'pg_dump.exe' $(if ($env:JOBS_IM_PG_BIN) { Join-Path $env:JOBS_IM_PG_BIN 'pg_dump.exe' } else { '' })
    $script:PgRestore=Tool 'pg_restore.exe' $(if ($env:JOBS_IM_PG_BIN) { Join-Path $env:JOBS_IM_PG_BIN 'pg_restore.exe' } else { '' })
    Invoke-Tool $Go @('version')
    Invoke-Tool $PgDump @('--version')
    Invoke-Tool $OpenSSL @('version')
    Invoke-Tool $Node @('--version')
    Invoke-Tool $NatsBin @('--version')
}
function Confirm-Action {
    Write-Host "JobsIM Windows · $Action`nstate=$State`nHTTPS=$HttpsPort；内部角色=$RolePort～$RoleLast"
    Write-Host '此入口要求已准备的业务PG和物理独立journal PG；不会安装、启动、停止或删除外部PG。'
    if ($Action -eq 'run') { Write-Host '关闭此运行终端，服务将结束' -ForegroundColor Red }
    if (!$Yes -and (Read-Host '按回车确认，输入任意文字取消')) { throw '用户取消。' }
}
function Protect-Directory([string]$Directory) {
    if ((Test-Path -LiteralPath $Directory) -and ((Get-Item -LiteralPath $Directory).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '不跟随资源目录junction/链接。' }
    New-Item -ItemType Directory -Path $Directory -Force | Out-Null
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl=[Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true,$false)
    $acl.SetOwner($sid)
    $rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $Directory -AclObject $acl
}
function Prepare-State {
    Assert-Owner
    Protect-Directory $State
    foreach ($name in @('Application','TLS','Bin','Logs','Secrets')) { Protect-Directory (Join-Path $State $name) }
    $owner=Join-Path $State 'owner.json'
    if (!(Test-Path $owner)) { @{project=$Repo;user=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value}|ConvertTo-Json|Set-Content -LiteralPath $owner }
    $script:Lock=[IO.File]::Open((Join-Path $State 'transaction.lock'),[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)
}
function Server-IsRunning {
    $receipt=Join-Path $State 'process.json'
    if (!(Test-Path $receipt)) { return $false }
    $r=Get-Content -LiteralPath $receipt -Raw|ConvertFrom-Json
    $p=Get-Process -Id $r.pid -ErrorAction SilentlyContinue
    if (!$p) { return $false }
    if ($p.StartTime.ToUniversalTime().Ticks.ToString() -ne $r.started -or $p.Path -ne (Join-Path $State 'Bin/JobsIMd.exe')) { throw 'PID已复用或身份不匹配，拒绝控制陌生进程。' }
    return $true
}
function Require-Stopped {
    if (Server-IsRunning) { throw '本实例应用仍运行，必须先优雅结束；不强杀或在线备份。' }
    if (Get-NetTCPConnection -State Listen -LocalPort $HttpsPort -ErrorAction SilentlyContinue) { throw '应用端口仍有监听，不能证明应用已停；保留broker。' }
}
# 仅在确认后的真实启动/停止编译原生控制桥；check不执行Add-Type或Win32调用。
function Initialize-OwnedConsole {
    if ('JobsIMOwnedConsole' -as [type]) { return }
    Add-Type -TypeDefinition @'
using System;
using System.Collections;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
public static class JobsIMOwnedConsole {
    [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] struct StartupInfo {
        public int cb; public string reserved, desktop, title; public int x,y,xSize,ySize,xChars,yChars,fill,flags;
        public short show, reservedSize; public IntPtr reservedPointer, input, output, error;
    }
    [StructLayout(LayoutKind.Sequential)] struct ProcessInfo { public IntPtr process, thread; public uint pid, tid; }
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcessW(string application,StringBuilder command,IntPtr pa,IntPtr ta,bool inherit,uint flags,IntPtr environment,string directory,ref StartupInfo startup,out ProcessInfo information);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
    [DllImport("kernel32.dll")] public static extern uint GetConsoleCP();
    static string Quote(string value) {
        var result=new StringBuilder("\""); int slashes=0;
        foreach(char item in value) {
            if(item=='\\') { slashes++; continue; }
            if(item=='\"') { result.Append('\\',slashes*2+1).Append(item); slashes=0; continue; }
            result.Append('\\',slashes).Append(item); slashes=0;
        }
        return result.Append('\\',slashes*2).Append('"').ToString();
    }
    public static Process Start(string executable,string[] arguments,string directory,IDictionary overrides) {
        if(GetConsoleCP()==0) throw new InvalidOperationException("A real console is required for exact owned-group shutdown");
        var command=new StringBuilder(Quote(executable)); foreach(string value in arguments) command.Append(' ').Append(Quote(value));
        var values=new SortedDictionary<string,string>(StringComparer.OrdinalIgnoreCase);
        foreach(DictionaryEntry entry in Environment.GetEnvironmentVariables()) {
            string name=(string)entry.Key; if(!name.StartsWith("JOBS_IM_",StringComparison.OrdinalIgnoreCase) && !name.Equals("PGPASSWORD",StringComparison.OrdinalIgnoreCase)) values[name]=(string)entry.Value;
        }
        foreach(DictionaryEntry entry in overrides) values[(string)entry.Key]=(string)entry.Value;
        var block=new StringBuilder(); foreach(var entry in values) block.Append(entry.Key).Append('=').Append(entry.Value).Append('\0'); block.Append('\0');
        IntPtr environment=Marshal.StringToHGlobalUni(block.ToString()); ProcessInfo information;
        var startup=new StartupInfo(); startup.cb=Marshal.SizeOf(startup);
        try {
            if(!CreateProcessW(executable,command,IntPtr.Zero,IntPtr.Zero,false,0x00000200|0x00000400,environment,directory,ref startup,out information)) throw new Win32Exception(Marshal.GetLastWin32Error());
            try { var owned=Process.GetProcessById((int)information.pid); var handle=owned.SafeHandle; return owned; }
            finally { CloseHandle(information.thread); CloseHandle(information.process); }
        } finally {
            for(int index=0;index<block.Length;index++) Marshal.WriteInt16(environment,index*2,0);
            Marshal.FreeHGlobal(environment);
        }
    }
}
'@
}
# 验证仍是本入口创建的新进程组，再用独立控制助手发送仅该组CTRL_BREAK。
function Stop-OwnedGroup($Process,$Receipt) {
    $null=$Process.SafeHandle
    if (!$Receipt.groupCreated -or $Process.StartTime.ToUniversalTime().Ticks.ToString() -cne [string]$Receipt.started -or $Process.Path -cne [string]$Receipt.path -or (Get-FileHash -Algorithm SHA256 -LiteralPath $Process.Path).Hash -cne [string]$Receipt.sha256) { throw '进程组身份或原工具已改变，拒绝发送信号。' }
    $groupId=[int]$Process.Id
    $source=@"
Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public static class JobsIMConsoleSignal{[DllImport("kernel32.dll",SetLastError=true)]public static extern bool FreeConsole();[DllImport("kernel32.dll",SetLastError=true)]public static extern bool AttachConsole(uint pid);[DllImport("kernel32.dll",SetLastError=true)]public static extern bool GenerateConsoleCtrlEvent(uint kind,uint group);}'
[void][JobsIMConsoleSignal]::FreeConsole()
if(![JobsIMConsoleSignal]::AttachConsole($groupId)){exit 3}
if(![JobsIMConsoleSignal]::GenerateConsoleCtrlEvent(1,$groupId)){exit 4}
Start-Sleep -Milliseconds 100
[void][JobsIMConsoleSignal]::FreeConsole()
"@
    $encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($source))
    Invoke-Tool (Join-Path $PSHOME 'pwsh.exe') @('-NoLogo','-NoProfile','-NonInteractive','-EncodedCommand',$encoded)
    if (!$Process.WaitForExit(15000)) { throw '本入口进程组未按时退出，保留资源，不强杀。' }
}
# 只优雅停止新格式本实例应用；旧未知进程组仍要求原运行终端正常退出。
function Stop-Backend {
    if (!(Server-IsRunning)) { return }
    $receipt=Get-Content (Join-Path $State 'process.json') -Raw|ConvertFrom-Json
    if (!$receipt.PSObject.Properties['groupCreated']) { throw '旧应用无独立进程组收据；请回原终端Ctrl+C，不按名称强杀。' }
    $process=Get-Process -Id $receipt.pid
    try { Stop-OwnedGroup $process $receipt } finally { $process.Dispose() }
}
# 单个秘密文件禁止继承，仅当前用户可读写；不用Unix权限位伪装Windows ACL。
function Protect-SecretFile([string]$Path) {
    $item=Get-Item -LiteralPath $Path
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw '秘密文件不可为链接。' }
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl=[Security.AccessControl.FileSecurity]::new();$acl.SetAccessRuleProtection($true,$false);$acl.SetOwner($sid)
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','Allow'))
    Set-Acl -LiteralPath $Path -AclObject $acl
}
# 生成一次私有token与配置，已有材料漂移或缺失时保留并拒绝替换。
function Prepare-Nats {
    foreach($path in @($NatsTokenFile,$NatsConfig,$NatsStore)) { if((Test-Path -LiteralPath $path) -and ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'NATS材料不允许链接。'} }
    if (!(Test-Path -LiteralPath $NatsTokenFile)) {
        if((Test-Path $NatsConfig) -or (Test-Path $NatsStore)){throw '既有NATS状态缺token，不生成替代秘密。'}
        [IO.File]::WriteAllText($NatsTokenFile,[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()+"`n")
    }
    Protect-SecretFile $NatsTokenFile
    $token=[IO.File]::ReadAllText($NatsTokenFile).TrimEnd("`r","`n")
    if ($token -cnotmatch '^[0-9a-f]{64}$') { throw 'NATS token格式无效，不显示或替换原值。' }
    $storeJson=ConvertTo-Json $NatsStore -Compress; $pidJson=ConvertTo-Json (Join-Path $State 'nats-runtime.pid') -Compress; $logJson=ConvertTo-Json (Join-Path $State 'Logs/nats.log') -Compress
    $expected="listen: `"127.0.0.1:$NatsPort`"`npid_file: $pidJson`nlog_file: $logJson`nauthorization { token: `"$token`" }`njetstream { store_dir: $storeJson, max_file_store: $NatsMaxFile, max_memory_store: $NatsMaxMemory }`n"
    if (Test-Path $NatsConfig) { if([IO.File]::ReadAllText($NatsConfig) -cne $expected){throw 'NATS配置漂移，保留原件，需受控变更。'} } else { [IO.File]::WriteAllText($NatsConfig,$expected) }
    $token='';$expected='';Protect-SecretFile $NatsConfig;Protect-Directory $NatsStore
    Invoke-Tool $NatsBin @('-t','-c',$NatsConfig)
}
# 标准输入网络握手仅传至loopback，错误不回显认证载荷。
function Test-NatsReady {
    $client=[Net.Sockets.TcpClient]::new()
    try {
        $task=$client.ConnectAsync('127.0.0.1',$NatsPort);if(!$task.Wait(1000)){return $false};$task.GetAwaiter().GetResult()
        $stream=$client.GetStream();$stream.ReadTimeout=1000;$stream.WriteTimeout=1000
        $token=[IO.File]::ReadAllText($NatsTokenFile).TrimEnd("`r","`n")
        $bytes=[Text.Encoding]::ASCII.GetBytes("CONNECT {`"auth_token`":`"$token`",`"verbose`":true}`r`nPING`r`n");$token=''
        try{$stream.Write($bytes,0,$bytes.Length)}finally{[Array]::Clear($bytes,0,$bytes.Length)}
        $buffer=[byte[]]::new(4096);$response=''
        while($response.Length -lt 4096 -and !$response.Contains('PONG') -and !$response.Contains('-ERR')){$count=$stream.Read($buffer,0,$buffer.Length);if($count -le 0){break};$response+=[Text.Encoding]::ASCII.GetString($buffer,0,$count)}
        return $response.Contains('PONG') -and $response.Contains('"jetstream":true') -and !$response.Contains('-ERR')
    } catch { return $false } finally { $client.Dispose() }
}
# 按收据查验原broker工具、启动时刻和配置，check不发送任何网络请求。
function Get-OwnedNats {
    $file=Join-Path $State 'nats-process.json';if(!(Test-Path $file)){return $null}
    $receipt=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    $process=Get-Process -Id $receipt.pid -ErrorAction SilentlyContinue;if(!$process){return $null}
    $null=$process.SafeHandle
    if($process.StartTime.ToUniversalTime().Ticks.ToString() -cne $receipt.started -or $process.Path -cne $receipt.path -or (Get-FileHash -Algorithm SHA256 -LiteralPath $receipt.path).Hash -cne $receipt.sha256 -or (Get-FileHash -Algorithm SHA256 -LiteralPath $NatsConfig).Hash -cne $receipt.configSha256){$process.Dispose();throw 'NATS原进程身份或配置不匹配，不接管。'}
    return $process
}
# 应用已停止后停止自有broker，保留所有JetStream数据。
function Stop-Nats {
    Require-Stopped
    $process=Get-OwnedNats;if(!$process){return}
    $receipt=Get-Content (Join-Path $State 'nats-process.json') -Raw|ConvertFrom-Json
    try { Stop-OwnedGroup $process $receipt } finally { $process.Dispose() }
}
# 自有新进程组只接收配置文件路径，秘密不进命令行或子进程环境。
function Start-Nats {
    Initialize-OwnedConsole;Prepare-Nats
    $process=[JobsIMOwnedConsole]::Start($NatsBin,[string[]]@('-c',$NatsConfig),$State,@{})
    $script:NatsProcess=$process
    @{pid=$process.Id;started=$process.StartTime.ToUniversalTime().Ticks.ToString();path=$NatsBin;sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $NatsBin).Hash;configSha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $NatsConfig).Hash;groupCreated=$true}|ConvertTo-Json|Set-Content (Join-Path $State 'nats-process.json')
    for($attempt=0;$attempt -lt 20;$attempt++){if($process.HasExited){break};if(Test-NatsReady){return};Start-Sleep -Milliseconds 200}
    throw 'NATS认证未就绪，拒绝启动应用；保留私有日志和数据。'
}
# 冷态归档必须包含并明确列出broker范围，缺收据不能伪称已停。
function Write-NatsBackupManifest([string]$Path) {
    Stop-Nats
    if(Test-Path $NatsStore){
        $receiptFile=Join-Path $State 'nats-process.json'
        if(!(Test-Path $receiptFile) -or !(Test-Path $NatsConfig) -or !(Test-Path $NatsTokenFile)){throw 'NATS备份材料不完整，保留原件。'}
        $receipt=Get-Content $receiptFile -Raw|ConvertFrom-Json
        if((Get-FileHash -Algorithm SHA256 -LiteralPath $NatsConfig).Hash -cne $receipt.configSha256){throw 'NATS配置偏离原收据。'}
        @{component='NATS JetStream';state='stopped-before-archive';store='Application/JetStream';config='Secrets/nats.conf';token='Secrets/'+(Split-Path $NatsTokenFile -Leaf);restoreTested=$false}|ConvertTo-Json|Set-Content $Path
    } else {@{component='NATS JetStream';state='never-initialized';storeIncluded=$false}|ConvertTo-Json|Set-Content $Path}
}
function Database-Environment([string]$Role) {
    $name='JOBS_IM_'+$Role.ToUpperInvariant()+'_DATABASE_URL'
    $value=[Environment]::GetEnvironmentVariable($name)
    if (!$value -or $value -notmatch '^postgres(ql)?://') { throw "请预置$name，不能使用同一默认库代替独立角色。" }
    return $value
}
function Pg-Environment([string]$Dsn) {
    $uri=[Uri]$Dsn
    $parts=$uri.UserInfo.Split(':',2)
    if ($parts.Count -ne 2 -or !$uri.Host -or !$uri.AbsolutePath.Trim('/')) { throw 'PG连接需用户名、密码、主机和数据库。' }
    $variables=@{PGHOST=$uri.Host;PGPORT=$(if ($uri.Port -gt 0) {$uri.Port} else {5432});PGUSER=[Uri]::UnescapeDataString($parts[0]);PGPASSWORD=[Uri]::UnescapeDataString($parts[1]);PGDATABASE=[Uri]::UnescapeDataString($uri.AbsolutePath.Trim('/'))}
    foreach ($part in $uri.Query.TrimStart('?').Split('&',[StringSplitOptions]::RemoveEmptyEntries)) {
        $kv=$part.Split('=',2)
        if ($kv.Count -ne 2) { throw '无效PG查询参数。' }
        $key=[Uri]::UnescapeDataString($kv[0]);$value=[Uri]::UnescapeDataString($kv[1])
        switch ($key) {
            'sslmode' {$variables.PGSSLMODE=$value}
            'sslrootcert' {$variables.PGSSLROOTCERT=$value}
            'sslcert' {$variables.PGSSLCERT=$value}
            'sslkey' {$variables.PGSSLKEY=$value}
            'connect_timeout' {$variables.PGCONNECT_TIMEOUT=$value}
            default { throw 'PG备份入口不支持此URI参数，拒绝默默改变连接语义。' }
        }
    }
    return $variables
}
function Prepare-TLS {
    $tls=Join-Path $State 'TLS'
    if ((Test-Path "$tls/server.crt") -and (Test-Path "$tls/server.key") -and (Test-Path "$tls/ca.crt")) {
        Invoke-Tool $OpenSSL @('verify','-CAfile',"$tls/ca.crt","$tls/server.crt")
        Invoke-Tool $OpenSSL @('x509','-in',"$tls/server.crt",'-checkend','86400','-noout')
        return
    }
    if ((Test-Path "$tls/ca.key") -or (Test-Path "$tls/ca.crt")) { throw 'TLS材料不完整，拒绝重置信任根。' }
    Invoke-Tool $OpenSSL @('req','-x509','-newkey','rsa:3072','-nodes','-days','365','-keyout',"$tls/ca.key",'-out',"$tls/ca.crt",'-subj','/CN=JobsIM Local Instance CA','-addext','basicConstraints=critical,CA:TRUE')
    Invoke-Tool $OpenSSL @('req','-new','-newkey','rsa:2048','-nodes','-keyout',"$tls/server.key",'-out',"$tls/server.csr",'-subj','/CN=127.0.0.1')
    "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2`nextendedKeyUsage=serverAuth" | Set-Content "$tls/server.ext" -Encoding ascii
    Invoke-Tool $OpenSSL @('x509','-req','-in',"$tls/server.csr",'-CA',"$tls/ca.crt",'-CAkey',"$tls/ca.key",'-CAcreateserial','-days','90','-out',"$tls/server.crt",'-extfile',"$tls/server.ext")
}
function Backup-Instance {
    Stop-Backend;Require-Stopped;Stop-Nats
    Protect-Directory $Backups;Protect-Directory $Recovery
    if (!(Test-Path "$Recovery/key.pem")) {
        if (Test-Path "$Recovery/cert.pem") { throw '恢复密钥缺失，不替换既有密钥。' }
        Invoke-Tool $OpenSSL @('req','-x509','-newkey','rsa:3072','-nodes','-days','3650','-keyout',"$Recovery/key.pem",'-out',"$Recovery/cert.pem",'-subj','/CN=JobsIM Backup Recovery')
    }
    $stem='JobsIM-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[Guid]::NewGuid().ToString('N')
    $stage=Join-Path $Backups ('.staging-'+$stem)
    Protect-Directory $stage
    Write-NatsBackupManifest (Join-Path $stage 'broker-manifest.json')
    foreach ($role in $Roles) {
        Invoke-Tool $PgDump @('--format=custom',"--file=$stage/$role.dump") (Pg-Environment (Database-Environment $role))
        Invoke-Tool $PgRestore @('--list',"$stage/$role.dump") @{} "$stage/$role.list"
    }
    $tarball=Join-Path $stage 'payload.tar.gz'
    # Enumerate exact dumps so the archive never includes itself.
    $files=@('broker-manifest.json');foreach($role in $Roles){$files+="$role.dump";$files+="$role.list"}
    Invoke-Tool $Tar (@('-czf',$tarball,'-C',$State,'owner.json','Application','TLS','Secrets','-C',$stage)+$files)
    $archive=Join-Path $Backups ($stem+'.cms')
    Invoke-Tool $OpenSSL @('cms','-encrypt','-aes-256-gcm','-binary','-outform','DER','-in',$tarball,'-out',$archive,"$Recovery/cert.pem")
    $verified=Join-Path $stage 'verified.tar.gz'
    Invoke-Tool $OpenSSL @('cms','-decrypt','-binary','-inform','DER','-in',$archive,'-inkey',"$Recovery/key.pem",'-out',$verified)
    Invoke-Tool $Tar @('-tzf',$verified) @{} ($archive+'.manifest')
    (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash|Set-Content ($archive+'.sha256')
    foreach ($name in $files+@('payload.tar.gz','verified.tar.gz')) { Remove-Item -LiteralPath (Join-Path $stage $name) }
    Remove-Item -LiteralPath $stage
    Write-Host "已加密并解密目录验真：$archive`n恢复私钥位于state之外：$Recovery/key.pem"
    Write-Host '不回滚当前journal。未实现安全恢复向导；此记录不等同隔离恢复演练。'
}
function Run-Instance {
    Initialize-OwnedConsole
    if([JobsIMOwnedConsole]::GetConsoleCP() -eq 0){throw 'run需要真实Windows控制台以便精确优雅回收本实例进程组。'}
    Stop-Backend;Require-Stopped;Stop-Nats
    if (Test-Path (Join-Path $State 'Application/instance.json')) { Backup-Instance }
    $ports=@($HttpsPort,$NatsPort)+@($RolePort..$RoleLast)
    if (@($ports|Select-Object -Unique).Count -ne $ports.Count) { throw '监听端口重复。' }
    foreach ($port in $ports) { if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { throw "端口$port 被占用，不接管。" } }
    $environment=@{GOTOOLCHAIN='local';JOBS_IM_PUBLIC_URL="https://127.0.0.1:$HttpsPort"}
    foreach ($role in $Roles) { $environment['JOBS_IM_'+$role.ToUpperInvariant()+'_DATABASE_URL']=Database-Environment $role }
    $environment.JOBS_IM_DATABASE_URL=$environment.JOBS_IM_IDENTITY_DATABASE_URL
    $environment.JOBS_IM_GOVERNANCE_DATABASE_URL=$environment.JOBS_IM_IDENTITY_DATABASE_URL
    $environment.JOBS_IM_NATS_URL=$NatsUrl;$environment.JOBS_IM_NATS_TOKEN_FILE=$NatsTokenFile
    Prepare-TLS
    Invoke-Tool $Go @('build','-trimpath','-o',"$State/Bin/JobsIMd.exe",'./cmd/jobsimd') $environment '' (Join-Path $Repo '代码/后端/Go')
    # npm.cmd is invoked through cmd.exe; only fixed arguments enter its command string.
    Invoke-Tool $env:ComSpec @('/d','/c','npm.cmd run build') @{} '' (Join-Path $Repo '代码/前端/Web')
    Start-Nats
    $arguments=[string[]]@('--service','all','--addr',"127.0.0.1:$HttpsPort",'--internal-base-port',"$RolePort",'--state-dir',"$State/Application",'--tls-cert',"$State/TLS/server.crt",'--tls-key',"$State/TLS/server.key",'--web-dir',(Join-Path $Repo '代码/前端/Web/Dist'))
    $server=[JobsIMOwnedConsole]::Start((Join-Path $State 'Bin/JobsIMd.exe'),$arguments,$Repo,$environment)
    @{pid=$server.Id;started=$server.StartTime.ToUniversalTime().Ticks.ToString();path=(Join-Path $State 'Bin/JobsIMd.exe');sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $State 'Bin/JobsIMd.exe')).Hash;groupCreated=$true}|ConvertTo-Json|Set-Content (Join-Path $State 'process.json')
    $Lock.Dispose();$script:Lock=$null
    $ready=$false
    for ($attempt=0;$attempt -lt 30;$attempt++) {
        if ($server.HasExited) { break }
        try {
            Invoke-Tool $Curl @('--fail','--silent','--show-error','--cacert',"$State/TLS/ca.crt",'--max-time','1',"https://127.0.0.1:$HttpsPort/api/bootstrap") @{} "$State/bootstrap.json"
            $ready=$true;break
        } catch { Start-Sleep -Milliseconds 500 }
    }
    if (!$ready) { throw '严格TLS bootstrap检查未通过；保留收据，不宣称启动成功。若进程仍在运行，请回此终端Ctrl+C正常结束。' }
    $bootstrap=Get-Content "$State/bootstrap.json" -Raw|ConvertFrom-Json
    Invoke-Tool $OpenSSL @('x509','-in',"$State/TLS/ca.crt",'-noout','-fingerprint','-sha256') @{} "$State/ca.sha256"
    $fingerprint=(Get-Content "$State/ca.sha256" -Raw).Trim().Split('=',2)[1]
    @{deploymentId=$bootstrap.deploymentId;name=$bootstrap.name;url="https://127.0.0.1:$HttpsPort";protocolVersion=$bootstrap.protocolVersion;caSha256=$fingerprint}|ConvertTo-Json|Set-Content "$State/connection.json"
    Write-Host '关闭此运行终端，服务将结束' -ForegroundColor Red
    Write-Host "用户端=https://127.0.0.1:$HttpsPort；deploymentId=$($bootstrap.deploymentId)；CA SHA256=$fingerprint"
    Write-Host '管理写入口尚未交付，不生成假后台URL。二维码工具未接入，使用公开URL。'
    Write-Host "本CA=$State/TLS/ca.crt；客户端须受控核对，不安装全局CA，不绕过证书告警。"
    try { $server.WaitForExit();if($server.ExitCode -ne 0){throw "后端退出码$($server.ExitCode)"} }
    finally { if (!$server.HasExited) { Stop-Backend };Require-Stopped;Stop-Nats }
}
try {
    Assert-StatePath
    if ($Action -eq 'help') { Write-Host '部署运行.ps1 check|plan|status|run|stop|backup [-State 绝对目录] [-Yes] [-NoBackup]';exit 0 }
    Find-Tools;Assert-Owner
    if ($Action -in @('check','plan','status')) {
        Write-Host "只读$Action；不创建目录/密钥、不安装、不启动或停止资源。`n源码=$Repo`nstate=$State"
        if (Server-IsRunning) { Write-Host '匹配本实例后端运行中。' } else { Write-Host '没有匹配的本实例后端。' }
        Write-Host "NATS=$NatsUrl；JetStream=$NatsStore；秘密仅文件。"
        $broker=Get-OwnedNats;if($broker){Write-Host '匹配本实例NATS运行中。';$broker.Dispose()}
        exit 0
    }
    Confirm-Action
    if ($Action -in @('stop','backup') -and !(Test-Path (Join-Path $State 'owner.json'))) { Write-Host '实例未创建，无资源变更。';exit 0 }
    Prepare-State
    if ($Action -eq 'run') { Run-Instance }
    else { Stop-Backend;Require-Stopped;Stop-Nats;if($Action -eq 'backup' -or !$NoBackup){Backup-Instance};Write-Host '本实例后端已停止；外部PG与全部数据、信任材料均保留。' }
} catch {
    Write-Error $_.Exception.Message -ErrorAction Continue
    exit 2
} finally {
    if ($NatsProcess -and !(Server-IsRunning)) { Stop-Nats }
    if ($Lock) { $Lock.Dispose() }
}
