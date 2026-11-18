#requires -Version 7.4
# Created by Jobs on 2026年11月14日，星期六.
[CmdletBinding()]
param(
    [ValidateSet('check','plan','status','run','stop','backup','undeploy','help')][string]$Action = 'undeploy',
    [string]$State = $env:JOBS_IM_DEPLOY_STATE,
    [switch]$Yes,
    [switch]$NoBackup,
    [ValidateSet('local','server')][string]$Mode = '',
    [switch]$Check,
    [switch]$Plan
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ExplicitState = ![string]::IsNullOrWhiteSpace($State)
$PgBin = ''
$PgProfile = $null
$DatabaseURLs = @{}
$DatabaseRoute = ''
$CreatingPg = $false
$RunId = [Guid]::NewGuid().ToString('N')
$ExitStatus = 0
$Origin = $null
$DataChoice = ''
$FzfExecutable = ''
$QrBinary = ''
$QrPage = ''
$Cloudflared = ''
$AccessMode = 'loopback'
$BackendArguments = @()
$BackendEnvironment = @{}
$ExpectedDeploymentId = ''
$CaFingerprint = ''
$RecoveringTunnel = $false
$ActiveBackend = $null
$TunnelProcess = $null
$Repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../../..'))
$RepoId = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($Repo))).Substring(0,16).ToLowerInvariant()
if (!$State) { $State = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) "JobsIM/Instances/$RepoId/local" }
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
        Assert-PlainPath $path
        if ($path -eq $Repo -or $path.StartsWith($Repo.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw '实例、归档与解密私钥必须位于源码之外。' }
        if ((Test-Path -LiteralPath $path) -and ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '不接管符号链接或junction目录。' }
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
    if($Action -eq 'undeploy' -and (Test-Path -LiteralPath (Join-Path $State 'origin.json'))){$null=Read-Origin;return}
    $owner = Join-Path $State 'owner.json'
    if (Test-Path -LiteralPath $owner) {
        $record = Get-Content -LiteralPath $owner -Raw | ConvertFrom-Json
        if ($record.project -ne $Repo -or $record.user -ne (Get-CurrentOwnerSID)) { throw '收据不属于本项目或当前用户，拒绝接管。' }
    } elseif ((Test-Path -LiteralPath $State) -and @(Get-ChildItem -LiteralPath $State -Force).Count -gt 0) { throw '非空state无归属收据，拒绝接管。' }
}
function Tool([string]$Name,[string]$Override) {
    if ($Override -match "://|[\r\n]") { throw '工具须为本地可执行路径，无效输入不回显。' }
    if ($Override) { if (!(Test-Path -LiteralPath $Override)) { throw "指定工具不存在：$Name；检查位置=$Override" }; return $Override }
    $command = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (!$command) { throw "缺少$Name；请先准备工具。本入口不全局安装。" }
    return $command.Source
}
function Invoke-Tool([string]$Exe,[string[]]$Arguments,[hashtable]$Environment=@{},[string]$OutputFile='',[string]$WorkingDirectory=$Repo) {
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName=$Exe; $start.WorkingDirectory=$WorkingDirectory; $start.UseShellExecute=$false
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    foreach ($key in $Environment.Keys) { $start.Environment[$key]=[string]$Environment[$key] }
    Assert-DiskBudget $State 0
    $process=[Diagnostics.Process]::Start($start)
    if($IsWindows -and ('JobsIMOwnedConsole' -as [type])){[JobsIMOwnedConsole]::Track($process)}
    $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
    try{while(!$process.WaitForExit(500)){Assert-DiskBudget $State 0;if(Test-Path -LiteralPath $Backups){Assert-DiskBudget $Backups 0}}}
    catch{if(!$process.HasExited){$process.Kill($true);$process.WaitForExit()};throw}
    $out=$stdout.GetAwaiter().GetResult(); $err=$stderr.GetAwaiter().GetResult()
    if ($process.ExitCode -ne 0) { throw "工具失败：$([IO.Path]::GetFileName($Exe))，退出码$($process.ExitCode)。不回显可能含连接信息的工具错误。" }
    if ($OutputFile) { [IO.File]::WriteAllText($OutputFile,$out) } elseif ($out.Trim()) { Write-Host $out.Trim() }
}
# Resolve current official stable tools only after mode/impact confirmation, keeping external PG major unchanged.
function Find-Tools {
    if(!$IsWindows){throw 'Windows原生工具只能在Windows验证。'}
    Select-DatabaseRoute
    if($DatabaseRoute -eq 'managed' -and !$PgBin){Select-PostgresTools}
    Find-OperationalTools
    $script:Curl=Tool 'curl.exe' ''
    $source=Join-Path $Repo '部署/公共组件/依赖版本'
    $script:Resolver=Join-Path $State 'Tools/dependency-versions.exe'
    Invoke-Tool $Go @('-C',$source,'build','-mod=readonly','-o',$Resolver,'.') @{GOTOOLCHAIN='local';GOWORK='off'}
    $script:Go=Resolve-LatestTool 'go' $Go
    $script:FzfExecutable=Resolve-LatestTool 'fzf' $FzfExecutable
    $candidate=Get-Command 'node.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $script:Node=Resolve-LatestTool 'node' $(if($candidate){$candidate.Source}else{''})
    $candidate=Get-Command 'nats-server.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $script:NatsBin=Resolve-LatestTool 'nats-server' $(if($env:JOBS_IM_NATS_BIN){$env:JOBS_IM_NATS_BIN}elseif($candidate){$candidate.Source}else{''})
    $pg=Invoke-Capture $PgDump @('--version') 8
    if($pg.code -ne 0 -or $pg.text -notmatch '^pg_dump \(PostgreSQL\) (18\.\d+(?:\.\d+)?)'){throw '当前JobsIM支持已准备的PostgreSQL18客户端；不自动跨major升级外部PG。'}
    Write-Host ('PG支持约束：18.x；实际客户端：'+$Matches[1])
    $latest=Invoke-Capture $Resolver @('--tool','postgres','--os','windows','--arch',$script:ToolArch,'--locked-manifest',(Join-Path $Repo '部署/deploy-tools.lock.tsv')) 40
    if($latest.code -ne 0){throw '无法核对PG当前官方稳定版本；保留环境，不伪称最新。'}
    $record=Convert-AssetRow $latest.text 'postgres'
    Write-Host ('官方当前PG稳定版本：'+$record.version+'；不改既有18主版本，Windows成品无可验证摘要时不自动安装。')
    if($script:Origin){$Origin.externalPostgreSQL=$(if($DatabaseRoute -eq 'managed'){'owned-business-and-retained-independent-journal'}else{'retained-not-owned'});$Origin|Add-Member -NotePropertyName sharedToolPaths -NotePropertyValue @($Go,$FzfExecutable,$Node,$NatsBin,$PgBin) -Force}
    Invoke-Tool $OpenSSL @('version');Invoke-Tool $PgRestore @('--version')
}
function Confirm-Action {
    Write-Host "JobsIM Windows · $Action`nstate=$State`nHTTPS=$HttpsPort；内部角色=$RolePort～$RoleLast"
    Write-Host '默认自动准备本实例原生双PG；已安装PG18工具需核验，外部PG仅高级路线且不接管。'
    if (!$Yes -and [Console]::IsInputRedirected){throw '需要交互终端；非交互只读检查或明确-Yes（不绕过危险菜单）。'}
    if ($Action -eq 'run') { Write-Host '关闭此运行终端，服务将结束' -ForegroundColor Red }
    if(!$Yes){$answer=Read-Host '按回车确认，输入任意文字取消';if($null -eq $answer -or $answer -cne ''){throw '用户取消。'}}
}
function Protect-Directory([string]$Directory) {
    if ((Test-Path -LiteralPath $Directory) -and ((Get-Item -LiteralPath $Directory).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '不跟随资源目录junction/链接。' }
    [IO.Directory]::CreateDirectory($Directory) | Out-Null
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl=[Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true,$false)
    $acl.SetOwner($sid)
    $rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $Directory -AclObject $acl
}
# Initialize only a new state or an explicit legacy operational state; undeploy never calls this creator.
function Prepare-State {
    Assert-Owner
    $script:StateInitialExisted=Test-Path -LiteralPath $State
    Protect-Directory $State
    Initialize-Origin
    foreach ($name in @('Application','TLS','Bin','Logs','Secrets','WebDist','Tools')) { Protect-Directory (Join-Path $State $name) }
    $owner=Join-Path $State 'owner.json'
    if (!(Test-Path -LiteralPath $owner)) { Write-PrivateJson $owner @{project=$Repo;user=(Get-CurrentOwnerSID)} }
}
function Server-IsRunning {
    $receipt=Join-Path $State 'process.json'
    if (!(Test-Path -LiteralPath $receipt)) { return $false }
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
    [StructLayout(LayoutKind.Sequential)] struct BasicLimit {
        public long processTime,jobTime; public uint flags; public UIntPtr minWorking,maxWorking; public uint activeProcessLimit; public UIntPtr affinity; public uint priority,scheduling;
    }
    [StructLayout(LayoutKind.Sequential)] struct IOCounters { public ulong readOps,writeOps,otherOps,readBytes,writeBytes,otherBytes; }
    [StructLayout(LayoutKind.Sequential)] struct ExtendedLimit { public BasicLimit basic; public IOCounters io; public UIntPtr processMemory,jobMemory,peakProcess,peakJob; }
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr CreateJobObjectW(IntPtr security,string name);
    [DllImport("kernel32.dll",SetLastError=true)] static extern bool SetInformationJobObject(IntPtr job,int kind,ref ExtendedLimit limits,uint length);
    [DllImport("kernel32.dll",SetLastError=true)] static extern bool AssignProcessToJobObject(IntPtr job,IntPtr process);
    [DllImport("kernel32.dll",SetLastError=true)] static extern uint ResumeThread(IntPtr thread);
    [DllImport("kernel32.dll",SetLastError=true)] static extern bool TerminateProcess(IntPtr process,uint code);
    static IntPtr ownedJob=IntPtr.Zero;
    static void EnsureJob() {
        if(ownedJob!=IntPtr.Zero) return;
        IntPtr job=CreateJobObjectW(IntPtr.Zero,null);
        if(job==IntPtr.Zero) throw new Win32Exception(Marshal.GetLastWin32Error());
        var limits=new ExtendedLimit(); limits.basic.flags=0x2000;
        if(!SetInformationJobObject(job,9,ref limits,(uint)Marshal.SizeOf(limits))) { int error=Marshal.GetLastWin32Error();CloseHandle(job);throw new Win32Exception(error); }
        ownedJob=job;
    }
    public static void Track(Process process) { EnsureJob(); if(process.HasExited) return; if(!AssignProcessToJobObject(ownedJob,process.SafeHandle.DangerousGetHandle()) && !process.HasExited) throw new Win32Exception(Marshal.GetLastWin32Error()); }

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
        EnsureJob();
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
            if(!CreateProcessW(executable,command,IntPtr.Zero,IntPtr.Zero,false,0x00000200|0x00000400|0x00000004,environment,directory,ref startup,out information)) throw new Win32Exception(Marshal.GetLastWin32Error());
            try {
                if(!AssignProcessToJobObject(ownedJob,information.process) || ResumeThread(information.thread)==0xFFFFFFFF) {
                    int error=Marshal.GetLastWin32Error();TerminateProcess(information.process,2);throw new Win32Exception(error);
                }
                var owned=Process.GetProcessById((int)information.pid); var handle=owned.SafeHandle; return owned;
            }
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
    $receipt=Get-Content -LiteralPath (Join-Path $State 'process.json') -Raw|ConvertFrom-Json
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
        if((Test-Path -LiteralPath $NatsConfig) -or (Test-Path -LiteralPath $NatsStore)){throw '既有NATS状态缺token，不生成替代秘密。'}
        [IO.File]::WriteAllText($NatsTokenFile,[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()+"`n")
    }
    Protect-SecretFile $NatsTokenFile
    $token=[IO.File]::ReadAllText($NatsTokenFile).TrimEnd("`r","`n")
    if ($token -cnotmatch '^[0-9a-f]{64}$') { throw 'NATS token格式无效，不显示或替换原值。' }
    $storeJson=ConvertTo-Json $NatsStore -Compress; $pidJson=ConvertTo-Json (Join-Path $State 'nats-runtime.pid') -Compress; $logJson=ConvertTo-Json (Join-Path $State 'Logs/nats.log') -Compress
    $expected="listen: `"127.0.0.1:$NatsPort`"`npid_file: $pidJson`nlog_file: $logJson`nauthorization { token: `"$token`" }`njetstream { store_dir: $storeJson, max_file_store: $NatsMaxFile, max_memory_store: $NatsMaxMemory }`n"
    if (Test-Path -LiteralPath $NatsConfig) { if([IO.File]::ReadAllText($NatsConfig) -cne $expected){throw 'NATS配置漂移，保留原件，需受控变更。'} } else { [IO.File]::WriteAllText($NatsConfig,$expected) }
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
    $file=Join-Path $State 'nats-process.json';if(!(Test-Path -LiteralPath $file)){return $null}
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
    $receipt=Get-Content -LiteralPath (Join-Path $State 'nats-process.json') -Raw|ConvertFrom-Json
    try { Stop-OwnedGroup $process $receipt } finally { $process.Dispose() }
}
# 自有新进程组只接收配置文件路径，秘密不进命令行或子进程环境。
function Start-Nats {
    Initialize-OwnedConsole;Prepare-Nats
    $process=(Start-ConsoleProcess $NatsBin ([string[]]@('-c',$NatsConfig)) $State @{})
    $script:NatsProcess=$process
    @{pid=$process.Id;started=$process.StartTime.ToUniversalTime().Ticks.ToString();path=$NatsBin;sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $NatsBin).Hash;configSha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $NatsConfig).Hash;groupCreated=$true}|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $State 'nats-process.json')
    for($attempt=0;$attempt -lt 20;$attempt++){if($process.HasExited){break};if(Test-NatsReady){return};Start-Sleep -Milliseconds 200}
    throw 'NATS认证未就绪，拒绝启动应用；保留私有日志和数据。'
}
# 冷态归档必须包含并明确列出broker范围，缺收据不能伪称已停。
function Write-NatsBackupManifest([string]$Path) {
    Stop-Nats
    if(Test-Path -LiteralPath $NatsStore){
        $receiptFile=Join-Path $State 'nats-process.json'
        if(!(Test-Path -LiteralPath $receiptFile) -or !(Test-Path -LiteralPath $NatsConfig) -or !(Test-Path -LiteralPath $NatsTokenFile)){throw 'NATS备份材料不完整，保留原件。'}
        $receipt=Get-Content -LiteralPath $receiptFile -Raw|ConvertFrom-Json
        if((Get-FileHash -Algorithm SHA256 -LiteralPath $NatsConfig).Hash -cne $receipt.configSha256){throw 'NATS配置偏离原收据。'}
        @{component='NATS JetStream';state='stopped-before-archive';store='Application/JetStream';config='Secrets/nats.conf';token='Secrets/'+(Split-Path $NatsTokenFile -Leaf);restoreTested=$false}|ConvertTo-Json|Set-Content -LiteralPath $Path
    } else {@{component='NATS JetStream';state='never-initialized';storeIncluded=$false}|ConvertTo-Json|Set-Content -LiteralPath $Path}
}
function Database-Environment([string]$Role) {
    if($script:DatabaseURLs.ContainsKey($Role)){return $script:DatabaseURLs[$Role]}
    $profile=Read-ManagedPgProfile
    if($profile){return [string]$profile.urls.$Role}
    $name='JOBS_IM_'+$Role.ToUpperInvariant()+'_DATABASE_URL'
    $value=[Environment]::GetEnvironmentVariable($name)
    if (!$value -or $value -notmatch '^postgres(ql)?://') { throw "请预置$name，不能使用同一默认库代替独立角色。" }
    return $value
}
function Pg-Environment([string]$Dsn) {
    $uri=[Uri]$Dsn
    $parts=$uri.UserInfo.Split(':',2)
    if ($parts.Count -ne 2 -or !$uri.Host -or !$uri.AbsolutePath.Trim('/')) { throw 'PG连接需用户名、密码、主机和数据库。' }
    $variables=@{PGCONNECT_TIMEOUT='5';PGHOST=$uri.Host;PGPORT=$(if ($uri.Port -gt 0) {$uri.Port} else {5432});PGUSER=[Uri]::UnescapeDataString($parts[0]);PGPASSWORD=[Uri]::UnescapeDataString($parts[1]);PGDATABASE=[Uri]::UnescapeDataString($uri.AbsolutePath.Trim('/'))}
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
    if($Mode -eq 'server'){Prepare-ServerTLS $tls;return}
    if ((Test-Path -LiteralPath "$tls/server.crt") -and (Test-Path -LiteralPath "$tls/server.key") -and (Test-Path -LiteralPath "$tls/ca.crt")) {
        try { Invoke-Tool $OpenSSL @('verify','-x509_strict','-purpose','sslserver','-CAfile',"$tls/ca.crt","$tls/server.crt") }
        catch { throw '现有TLS未通过严格验证，保留CA与全部原件；CA有效才可原CA重签服务端证书，CA缺陷需受控信任迁移。' }
        Invoke-Tool $OpenSSL @('x509','-in',"$tls/server.crt",'-checkend','86400','-noout')
        return
    }
    if (@(Get-ChildItem -LiteralPath $tls -Force).Count -gt 0) { throw 'TLS材料不完整或存在未知内容，保留全部原件并停止。' }
    $staging=Join-Path $State ('.TLS-pending-'+[Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($staging) | Out-Null
    Invoke-Tool $OpenSSL @('req','-x509','-newkey','rsa:3072','-nodes','-days','365','-keyout',"$staging/ca.key",'-out',"$staging/ca.crt",'-subj','/CN=JobsIM Local Instance CA','-addext','basicConstraints=critical,CA:TRUE','-addext','keyUsage=critical,keyCertSign,cRLSign','-addext','subjectKeyIdentifier=hash')
    Invoke-Tool $OpenSSL @('req','-new','-newkey','rsa:2048','-nodes','-keyout',"$staging/server.key",'-out',"$staging/server.csr",'-subj','/CN=127.0.0.1')
    "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2`nbasicConstraints=critical,CA:FALSE`nkeyUsage=critical,digitalSignature,keyEncipherment`nextendedKeyUsage=serverAuth`nsubjectKeyIdentifier=hash`nauthorityKeyIdentifier=keyid,issuer" | Set-Content -LiteralPath "$staging/server.ext" -Encoding ascii
    Invoke-Tool $OpenSSL @('x509','-req','-in',"$staging/server.csr",'-CA',"$staging/ca.crt",'-CAkey',"$staging/ca.key",'-CAcreateserial','-days','90','-out',"$staging/server.crt",'-extfile',"$staging/server.ext")
    Invoke-Tool $OpenSSL @('verify','-x509_strict','-purpose','sslserver','-CAfile',"$staging/ca.crt","$staging/server.crt")
    # Both calls reject a changed/nonempty destination and never overwrite old keys.
    [IO.Directory]::Delete($tls,$false)
    [IO.Directory]::Move($staging,$tls)
}
function Backup-Instance([switch]$ForUndeploy) {
    Stop-Backend;Require-Stopped;Stop-Nats
    $beforeBackup=@(Get-ResourceSnapshot)
    $managed=Read-ManagedPgProfile
    if($managed){Initialize-OwnedConsole;Start-ManagedPg}
    try {
    Assert-DiskBudget $Backups (1024*1024);Assert-DiskBudget $Recovery (1024*1024)
    Protect-Directory $Backups;Protect-Directory $Recovery
    if (!(Test-Path -LiteralPath "$Recovery/key.pem")) {
        if (Test-Path -LiteralPath "$Recovery/cert.pem") { throw '恢复密钥缺失，不替换既有密钥。' }
        Invoke-Tool $OpenSSL @('req','-x509','-newkey','rsa:3072','-nodes','-days','3650','-keyout',"$Recovery/key.pem",'-out',"$Recovery/cert.pem",'-subj','/CN=JobsIM Backup Recovery')
    }
    $stem='JobsIM-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[Guid]::NewGuid().ToString('N')
    $stage=Join-Path $Backups ('.staging-'+$stem)
    Protect-Directory $stage
    Write-NatsBackupManifest (Join-Path $stage 'broker-manifest.json')
    foreach ($role in $Roles) {
        $databaseBytes=Get-DatabaseBudgetBytes $role (Join-Path $stage ($role+'.size'))
        if($databaseBytes -lt 0 -or $databaseBytes -gt (([long]::MaxValue-1048576)/2)){throw '数据库实际大小/预算无效。'}
        [IO.File]::WriteAllText((Join-Path $stage ($role+'.size')),[string]$databaseBytes)
        Assert-DiskBudget $Backups (2L*$databaseBytes+1048576L)
        Invoke-Tool $PgDump @('--format=custom',"--file=$stage/$role.dump") (Pg-Environment (Database-Environment $role))
        Invoke-Tool $PgRestore @('--list',"$stage/$role.dump") @{} "$stage/$role.list"
        if((Get-Item -LiteralPath "$stage/$role.dump").Length -eq 0 -or (Get-Item -LiteralPath "$stage/$role.list").Length -eq 0){throw '数据库dump/list为空，未归档或清理源。'}
    }
    if($managed){Stop-ManagedPg}
    $tarball=Join-Path $stage 'payload.tar.gz'
    # Enumerate exact dumps so the archive never includes itself.
    $privateConnections=@{};foreach($role in $Roles){$privateConnections[$role]=Database-Environment $role};Write-PrivateJson (Join-Path $stage 'database-connections.json') $privateConnections;$privateConnections.Clear()
    $files=@('broker-manifest.json','database-connections.json');foreach($role in $Roles){$files+="$role.dump";$files+="$role.list";$files+="$role.size"}
    $proof=@{};foreach($name in $files){$proof[$name]=(Get-FileHash -LiteralPath (Join-Path $stage $name) -Algorithm SHA256).Hash}
    Write-PrivateJson (Join-Path $stage 'backup-files.sha256.json') $proof;$files+='backup-files.sha256.json'
    Assert-BackupBudget $stage $files
    $sourceItems=@(Get-ChildItem -LiteralPath $State -Force|Where-Object{$_.Name -notin @('transaction.lock','undo.json')}|ForEach-Object{$_.Name})
    $null=Get-ResourceSnapshot
    Invoke-Tool $Tar (@('-czf',$tarball,'-C',$State)+$sourceItems+@('-C',$stage)+$files)
    $archive=Join-Path $Backups ($stem+'.cms')
    Invoke-Tool $OpenSSL @('cms','-encrypt','-aes-256-gcm','-binary','-outform','DER','-in',$tarball,'-out',$archive,"$Recovery/cert.pem")
    $verified=Join-Path $stage 'verified.tar.gz'
    Invoke-Tool $OpenSSL @('cms','-decrypt','-binary','-inform','DER','-in',$archive,'-inkey',"$Recovery/key.pem",'-out',$verified)
    if((Get-FileHash -LiteralPath $tarball -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $verified -Algorithm SHA256).Hash){throw '备份解密字节与原载荷SHA不同，保留源及归档，不清理。'}
    Invoke-Tool $Tar @('-tzf',$verified) @{} ($archive+'.manifest')
    $required=@($files)
    if(Test-Path -LiteralPath (Join-Path $State 'owner.json')){$required+='owner.json'}
    if(Test-Path -LiteralPath (Join-Path $State 'Application/instance.json')){$required+='Application/instance.json'}
    Assert-BackupManifest ($archive+'.manifest') $required
    (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash|Set-Content -LiteralPath ($archive+'.sha256')
    foreach ($name in $files+@('payload.tar.gz','verified.tar.gz')) { Remove-Item -LiteralPath (Join-Path $stage $name) }
    [IO.Directory]::Delete($stage,$false)
    Write-Host "已加密并解密目录验真：$archive`n恢复私钥位于state之外：$Recovery/key.pem"
    Write-Host '不回滚/删除外部journal。此处解密验真不等同隔离数据库恢复演练。'
    if($managed){Stop-ManagedPg}
    $afterBackup=@(Get-ResourceSnapshot)
    $unchangedBefore=@($beforeBackup|Where-Object{$_.path -notmatch '^(Postgres/(business|journal)(/|$)|pg-(business|journal)-process\.json$)'})
    $unchangedAfter=@($afterBackup|Where-Object{$_.path -notmatch '^(Postgres/(business|journal)(/|$)|pg-(business|journal)-process\.json$)'})
    if((Get-SnapshotHash $unchangedBefore) -cne (Get-SnapshotHash $unchangedAfter)){throw '备份期间非PG源资源变化，保留源与原备份，停止清理。'}
    if($ForUndeploy){return [pscustomobject]@{archive=$archive;sha256=(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash;sourceSnapshot=$afterBackup;recoveryKey=(Join-Path $Recovery 'key.pem');recoveryKeyHash=(Get-FileHash -LiteralPath (Join-Path $Recovery 'key.pem') -Algorithm SHA256).Hash}}
    } finally { if($managed){Stop-ManagedPg} }
}
function Run-Instance {
    Initialize-OwnedConsole
    if([JobsIMOwnedConsole]::GetConsoleCP() -eq 0){throw 'run需要真实Windows控制台以便精确优雅回收本实例进程组。'}
    Stop-Backend;Require-Stopped;Stop-Nats
    if (Test-Path -LiteralPath (Join-Path $State 'Application/instance.json')) { Backup-Instance }
    $ports=@($HttpsPort,$NatsPort)+@($RolePort..$RoleLast)
    if (@($ports|Select-Object -Unique).Count -ne $ports.Count) { throw '监听端口重复。' }
    foreach ($port in $ports) { if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { throw "端口$port 被占用，不接管。" } }
    $environment=@{GOTOOLCHAIN='local';JOBS_IM_PUBLIC_URL=$script:PublicURL;PATH=([IO.Path]::GetDirectoryName($Node)+[IO.Path]::PathSeparator+$env:PATH)}
    foreach ($role in $Roles) { $environment['JOBS_IM_'+$role.ToUpperInvariant()+'_DATABASE_URL']=Database-Environment $role }
    $environment.JOBS_IM_DATABASE_URL=$environment.JOBS_IM_IDENTITY_DATABASE_URL
    $environment.JOBS_IM_GOVERNANCE_DATABASE_URL=$environment.JOBS_IM_IDENTITY_DATABASE_URL
    $environment.JOBS_IM_NATS_URL=$NatsUrl;$environment.JOBS_IM_NATS_TOKEN_FILE=$NatsTokenFile
    Prepare-TLS
    Invoke-Tool $Go @('build','-trimpath','-o',"$State/Bin/JobsIMd.exe",'./cmd/jobsimd') $environment '' (Join-Path $Repo '代码/后端/Go')
    # npm's JavaScript CLI receives argument-array paths; cmd.exe never reinterprets state directory characters.
    $npmCLI=Join-Path ([IO.Path]::GetDirectoryName($Node)) 'node_modules/npm/bin/npm-cli.js'
    if(!(Test-Path -LiteralPath $npmCLI)){throw '所选Node缺配套npm CLI，未借另一套全局npm构建。'}
    Invoke-Tool $Node @($npmCLI,'run','build','--','--outDir',(Join-Path $State 'WebDist')) @{PATH=([IO.Path]::GetDirectoryName($Node)+[IO.Path]::PathSeparator+$env:PATH)} '' (Join-Path $Repo '代码/前端/Web')
    Start-Nats
    $arguments=[string[]]@('--service','all','--addr',"$BindAddress`:$HttpsPort",'--internal-base-port',"$RolePort",'--state-dir',"$State/Application",'--tls-cert',"$State/TLS/server.crt",'--tls-key',"$State/TLS/server.key",'--web-dir',(Join-Path $State 'WebDist'))
    $hostProcess=Get-Process -Id $PID
    Write-PrivateJson (Join-Path $State 'host-process.json') @{pid=$PID;started=$hostProcess.StartTime.ToUniversalTime().Ticks.ToString();runId=$RunId;active=$true}
    $script:BackendArguments=$arguments;$script:BackendEnvironment=$environment
    $server=Start-BackendProcess $arguments $environment
    $Lock.Dispose();$script:Lock=$null
    $ready=$false
    for ($attempt=0;$attempt -lt 30;$attempt++) {
        if ($server.HasExited) { break }
        try {
            Invoke-Tool $Curl (@('--fail','--silent','--show-error','--cacert',"$State/TLS/ca.crt",'--max-time','1')+$script:HealthResolve+@($script:PublicURL+'/api/bootstrap')) @{} "$State/bootstrap.json"
            $ready=$true;break
        } catch { Start-Sleep -Milliseconds 500 }
    }
    if (!$ready) { throw '严格TLS bootstrap检查未通过；保留收据，不宣称启动成功。若进程仍在运行，请回此终端Ctrl+C正常结束。' }
    $bootstrap=Get-Content -LiteralPath "$State/bootstrap.json" -Raw|ConvertFrom-Json
    Invoke-Tool $OpenSSL @('x509','-in',"$State/TLS/ca.crt",'-noout','-fingerprint','-sha256') @{} "$State/ca.sha256"
    $fingerprint=(Get-Content -LiteralPath "$State/ca.sha256" -Raw).Trim().Split('=',2)[1]
    @{deploymentId=$bootstrap.deploymentId;name=$bootstrap.name;url=$script:PublicURL;protocolVersion=$bootstrap.protocolVersion;caSha256=$fingerprint}|ConvertTo-Json|Set-Content -LiteralPath "$State/connection.json"
    Write-Host '关闭此运行终端，服务将结束' -ForegroundColor Red
    Write-Host "用户端=https://127.0.0.1:$HttpsPort；deploymentId=$($bootstrap.deploymentId)；CA SHA256=$fingerprint"
    $script:ExpectedDeploymentId=$bootstrap.deploymentId;$script:CaFingerprint=$fingerprint
    if($Mode -eq 'local' -and $AccessMode -eq 'quick'){$null=Try-EnableQuick}
    else{Publish-CurrentConnection}
    Write-Host '本项目无独立管理网页；原生/CLI使用同一API origin，仍须核对部署身份与CA。'
    Write-Host "本CA=$State/TLS/ca.crt；客户端须受控核对，不安装全局CA，不绕过证书告警。"
    try { Wait-InstanceSession $script:ActiveBackend }
    finally { Stop-Backend;Require-Stopped;Stop-Nats }
}
# Resolve owner identity only on the real Windows operation path; tests may inject their own explicit fixture identity.
function Get-CurrentOwnerSID {
    return [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
}
# Reject existing ancestors as well as leaf links before every private write or removal.
function Assert-PlainPath([string]$Path) {
    $cursor=[IO.Path]::GetFullPath($Path)
    while($cursor){
        if(Test-Path -LiteralPath $cursor){
            $item=Get-Item -LiteralPath $cursor -Force
            if($item.Attributes -band [IO.FileAttributes]::ReparsePoint){throw '资源含符号链接或junction，保留并停止。'}
        }
        $next=[IO.Path]::GetDirectoryName($cursor)
        if($next -eq $cursor){break};$cursor=$next
    }
}
# A sidecar lock survives removal of state, so new deployments cannot enter the undo transaction.
function Enter-Transaction {
    Assert-PlainPath $State;Assert-PlainPath $Parent
    if(!(Test-Path -LiteralPath $Parent)){[IO.Directory]::CreateDirectory($Parent)|Out-Null}
    $hash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($State))).Substring(0,20)
    $script:LockPath=Join-Path $Parent ('.JobsIM-'+$hash+'.lock')
    Assert-PlainPath $LockPath
    try{$script:Lock=[IO.File]::Open($LockPath,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)}
    catch{throw '本实例有部署/反部署事务进行中，未接管锁。'}
}
# Read stable Windows object identity and owner from one handle; never substitute content hashes for file identity.
function Initialize-ResourceIdentity {
    if('JobsIMResourceIdentity' -as [type]){return}
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
public sealed class JobsIMResourceRecord {
    public string ObjectId; public string OwnerSid; public uint Links; public uint Attributes;
}
public static class JobsIMResourceIdentity {
    [StructLayout(LayoutKind.Sequential)] struct FileIdInfo {
        public ulong VolumeSerialNumber;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst=16)] public byte[] Id;
    }
    [StructLayout(LayoutKind.Sequential)] struct FileInfo {
        public uint Attributes;
        public System.Runtime.InteropServices.ComTypes.FILETIME Creation, Access, Write;
        public uint Volume, SizeHigh, SizeLow, Links, IndexHigh, IndexLow;
    }
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]
    static extern SafeFileHandle CreateFile(string path,uint access,uint share,IntPtr security,uint mode,uint flags,IntPtr template);
    [DllImport("kernel32.dll",SetLastError=true)]
    static extern bool GetFileInformationByHandleEx(SafeFileHandle handle,int kind,out FileIdInfo info,uint size);
    [DllImport("kernel32.dll",SetLastError=true)]
    static extern bool GetFileInformationByHandle(SafeFileHandle handle,out FileInfo info);
    [DllImport("advapi32.dll",SetLastError=true)]
    static extern uint GetSecurityInfo(SafeFileHandle handle,uint type,uint security,out IntPtr owner,out IntPtr group,out IntPtr dacl,out IntPtr sacl,out IntPtr descriptor);
    [DllImport("advapi32.dll",CharSet=CharSet.Unicode,SetLastError=true)]
    static extern bool ConvertSidToStringSid(IntPtr sid,out IntPtr text);
    [DllImport("kernel32.dll")] static extern IntPtr LocalFree(IntPtr value);
    public static JobsIMResourceRecord Read(string path) {
        using(var handle=CreateFile(path,0x20080,7,IntPtr.Zero,3,0x02200000,IntPtr.Zero)) {
            if(handle.IsInvalid)throw new Win32Exception(Marshal.GetLastWin32Error());
            FileInfo basic; FileIdInfo id;
            if(!GetFileInformationByHandle(handle,out basic) || !GetFileInformationByHandleEx(handle,18,out id,(uint)Marshal.SizeOf<FileIdInfo>()))throw new Win32Exception(Marshal.GetLastWin32Error());
            if((basic.Attributes & 0x400)!=0)throw new InvalidOperationException("Reparse resource refused.");
            if((basic.Attributes & 0x10)==0 && basic.Links!=1)throw new InvalidOperationException("Hard-linked file refused.");
            IntPtr owner,group,dacl,sacl,descriptor;
            uint error=GetSecurityInfo(handle,1,1,out owner,out group,out dacl,out sacl,out descriptor);
            if(error!=0)throw new Win32Exception((int)error);
            try {
                IntPtr text;
                if(!ConvertSidToStringSid(owner,out text))throw new Win32Exception(Marshal.GetLastWin32Error());
                try{return new JobsIMResourceRecord{ObjectId=id.VolumeSerialNumber.ToString("X16")+":"+Convert.ToHexString(id.Id),OwnerSid=Marshal.PtrToStringUni(text),Links=basic.Links,Attributes=basic.Attributes};}
                finally{LocalFree(text);}
            }finally{LocalFree(descriptor);}
        }
    }
}
'@
}
function Get-ResourceIdentity([string]$Path) {
    if(!$IsWindows){throw 'Windows文件身份须在Windows原生验证，不能以SHA替代。'}
    Initialize-ResourceIdentity
    $value=[JobsIMResourceIdentity]::Read($Path)
    return [pscustomobject]@{objectId=$value.ObjectId;ownerSid=$value.OwnerSid;links=$value.Links;attributes=$value.Attributes}
}
# Enumerate without following directory links; metadata records are outside the deletion snapshot.
function Get-ResourceSnapshot([string]$Root=$State) {
    Assert-PlainPath $Root
    $rows=[Collections.Generic.List[object]]::new()
    if(!(Test-Path -LiteralPath $Root)){return @()}
    $pending=[Collections.Generic.Stack[string]]::new();$pending.Push($Root)
    while($pending.Count){
        $directory=$pending.Pop()
        foreach($item in @(Get-ChildItem -LiteralPath $directory -Force)){
            if($directory -ceq $Root -and $item.Name -in @('origin.json','undo.json')){continue}
            Assert-PlainPath $item.FullName
            $relative=[IO.Path]::GetRelativePath($Root,$item.FullName).Replace('\','/')
            $identity=Get-ResourceIdentity $item.FullName
            $digest=if($item.PSIsContainer){''}else{(Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash}
            if((Get-SnapshotHash @($identity)) -cne (Get-SnapshotHash @((Get-ResourceIdentity $item.FullName)))){throw '读取清单期间对象身份/归属发生变化，保留。'}
            $rows.Add([pscustomobject]@{path=$relative;type=$(if($item.PSIsContainer){'directory'}else{'file'});sha256=$digest;objectId=$identity.objectId;ownerSid=$identity.ownerSid;links=$identity.links;attributes=$identity.attributes})
            if($item.PSIsContainer){$pending.Push($item.FullName)}
        }
    }
    return @($rows.ToArray()|Sort-Object path)
}
# Stable JSON fingerprints bind the selected source bytes before backup and each destructive step.
function Get-SnapshotHash($Rows) {
    $values=@($Rows|Where-Object{$null -ne $_})
    $text=ConvertTo-Json -InputObject $values -Depth 8 -Compress
    return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($text)))
}
# Commit origin/undo receipts atomically without printing secrets or replacing unknown files.
function Write-PrivateJson([string]$Path,$Value) {
    Assert-PlainPath $Path
    $temporary=$Path+'.'+[Guid]::NewGuid().ToString('N')+'.tmp'
    [IO.File]::WriteAllText($temporary,($Value|ConvertTo-Json -Depth 16),[Text.UTF8Encoding]::new($false))
    [IO.File]::Move($temporary,$Path,$true)
}
# Only a new empty state can establish a pre-deployment origin; legacy states stay operational but unowned for undo.
function Initialize-Origin {
    $path=Join-Path $State 'origin.json'
    if(Test-Path -LiteralPath $path){
        $script:Origin=Read-Origin
        if($Origin.mode -cne $Mode){throw '实例模式不匹配：本地和服务器必须各用原state。'}
        if($Origin.undoStarted){throw '反部署尚未完成，不能覆盖原点或重新部署。'}
        if(!$Origin.sealed){throw '上次原点未封存，保留现场，不把现状冒充原点。'}
        if((Get-SnapshotHash (Get-ResourceSnapshot)) -cne $Origin.snapshotHash){throw '部署后资源发生后续修改，保留并先核对。'}
        $Origin.sealed=$false;Write-PrivateJson $path $Origin;return
    }
    if(@(Get-ChildItem -LiteralPath $State -Force).Count){$script:Origin=$null;return}
    $script:Origin=[pscustomobject]@{schema=1;project=$Repo;state=$State;user=(Get-CurrentOwnerSID);rootIdentity=(Get-ResourceIdentity $State);before=$(if($script:StateInitialExisted){'empty-directory'}else{'absent'});mode=$Mode;publicURL=$script:PublicURL;sealed=$false;undoStarted=$false;created=[DateTime]::UtcNow.ToString('o');snapshot=@();snapshotHash='';externalPostgreSQL='retained-not-owned';sharedTools='retained-not-owned'}
    Write-PrivateJson $path $Origin
}
# Validate project/user/state binding and the exact supported top-level resource boundary.
function Read-Origin {
    $path=Join-Path $State 'origin.json';Assert-PlainPath $path
    if(!(Test-Path -LiteralPath $path)){throw '缺部署前原点清单：旧实例只可停止/备份，反部署保留并阻断。'}
    $record=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json
    if($record.schema -ne 1 -or $record.project -cne $Repo -or $record.state -cne $State -or $record.user -cne (Get-CurrentOwnerSID) -or $record.before -cnotin @('absent','empty-directory')){throw '原点归属或模式不匹配，未清理。'}
    if(!$record.PSObject.Properties['rootIdentity'] -or (Get-SnapshotHash @($record.rootIdentity)) -cne (Get-SnapshotHash @((Get-ResourceIdentity $State)))){throw '原点缺文件身份或state目录已换壳/换归属，保留并停止。'}
    $allowed=@('Application','TLS','Bin','Logs','Secrets','WebDist','Tools','AccessQR','owner.json','process.json','nats-process.json','nats-runtime.pid','bootstrap.json','ca.sha256','connection.json','host-process.json','tunnel-process.json','Postgres','JournalDatabase','pg-business-process.json','pg-journal-process.json','stop-request.json')
    foreach($row in @($record.snapshot)){
        if(!$row.PSObject.Properties['objectId'] -or !$row.PSObject.Properties['ownerSid'] -or !$row.PSObject.Properties['links'] -or !$row.PSObject.Properties['attributes']){throw '旧清单缺Windows文件身份，保留并阻断清理。'}
        if($row.path -match '(^[\\/]|:|(^|/)\.\.(/|$))' -or $row.type -notin @('file','directory') -or $row.path.Split('/')[0] -notin $allowed){throw '原点包含未知/越界资源，保留并停止。'}
    }
    return $record
}
# Finalize after the application's and broker's own processes have exited, never during live writes.
function Seal-Origin {
    if(!$script:Origin){return}
    $hostFile=Join-Path $State 'host-process.json'
    if(Test-Path -LiteralPath $hostFile){$hostRecord=Get-Content -LiteralPath $hostFile -Raw|ConvertFrom-Json;$hostRecord.active=$false;Write-PrivateJson $hostFile $hostRecord}
    Require-Stopped
    $broker=Get-OwnedNats;if($broker){$broker.Dispose();throw 'NATS仍运行，不能封存资源清单。'}
    $script:Origin.snapshot=@(Get-ResourceSnapshot);$script:Origin.snapshotHash=Get-SnapshotHash $Origin.snapshot;$script:Origin.sealed=$true
    Write-PrivateJson (Join-Path $State 'origin.json') $Origin
}
# Use fzf with explicit rows, a default first row and no inherited shell/default commands.
function Select-KeyboardChoice([string[]]$Items,[string]$Header) {
    $fzf=Find-InstalledFzf
    $start=[Diagnostics.ProcessStartInfo]::new();$start.FileName=$fzf;$start.UseShellExecute=$false;$start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true
    $start.StandardInputEncoding=[Text.UTF8Encoding]::new($false);$start.StandardOutputEncoding=[Text.UTF8Encoding]::new($false)
    foreach($name in @('FZF_DEFAULT_OPTS','FZF_DEFAULT_OPTS_FILE','FZF_DEFAULT_COMMAND')){[void]$start.Environment.Remove($name)}
    foreach($argument in @('--no-multi','--layout=reverse','--height=60%','--border','--delimiter=|','--with-nth=2',('--header='+$Header))){$start.ArgumentList.Add($argument)}
    $process=[Diagnostics.Process]::Start($start)
    try{
        if($IsWindows){[JobsIMOwnedConsole]::Track($process)}
        $reply=$process.StandardOutput.ReadToEndAsync()
        foreach($item in $Items){$process.StandardInput.WriteLine($item)};$process.StandardInput.Close()
        while(!$process.WaitForExit(150)){
            if($script:ActiveBackend -and $ActiveBackend.HasExited){
                $process.Kill($true);$process.WaitForExit();throw '本次应用已退出，关闭本次菜单并回收会话。'
            }
            if($script:TunnelProcess -and $TunnelProcess.HasExited -and !$script:RecoveringTunnel){$process.Kill($true);$process.WaitForExit();return 'recover-tunnel'}
        }
        $text=$reply.GetAwaiter().GetResult().TrimEnd("`r","`n");$lines=@($text -split '\r?\n'|Where-Object{$_ -ne ''})
        if($process.ExitCode -eq 130 -or ($process.ExitCode -eq 1 -and !$lines.Count)){return 'cancel'}
        if($process.ExitCode -ne 0 -or $lines.Count -ne 1 -or $Items -cnotcontains [string]$lines[0]){throw '选择器失败/空选/非法返回，取消且未清理。'}
        return ([string]$lines[0]).Split('|')[0]
    }finally{if(!$process.HasExited){$process.Kill($true);$process.WaitForExit()};$process.Dispose()}
}
# Default cancel is followed by an independent empty-line confirmation; -Yes cannot bypass either.
function Read-UndeployConfirmation {return [Console]::ReadLine()}
function Confirm-Undeploy {
    $script:DataChoice=Select-KeyboardChoice @('backup|数据库/文件数据备份（默认），验证后清理本地源','clear|本地数据直接清零（不备份，不删除外部PG）','cancel|取消') '反部署数据策略；Esc取消'
    if($DataChoice -eq 'cancel'){return $false}
    Write-Host "反部署准确范围：$State；外部八域PG、共享SDK、源码、外置备份和解密钥全部保留。"
    Write-Host '外部PG不属于本入口，反部署会报告未还原项；不会自动DROP数据库/角色。'
    if((Select-KeyboardChoice @('cancel|取消并保留全部资源','execute|执行上述反部署') '默认取消，核对范围后选择执行') -ne 'execute'){return $false}
    Write-Host '最后复核：仅直接回车执行；任意非空输入或EOF取消。'
    $answer=Read-UndeployConfirmation
    return $null -ne $answer -and $answer -ceq ''
}
# Verify stopped host finalization before obtaining the exclusive mutation lock.
function Wait-RunFinalization {
    $file=Join-Path $State 'host-process.json'
    if(!(Test-Path -LiteralPath $file)){return}
    $record=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    if([int]$record.pid -eq $PID){return}
    for($index=0;$index -lt 100;$index++){
        $latest=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
        if($latest.PSObject.Properties['active'] -and !$latest.active){$originFile=Join-Path $State 'origin.json';if(!(Test-Path -LiteralPath $originFile) -or (Get-Content -LiteralPath $originFile -Raw|ConvertFrom-Json).sealed){return}}
        $hostProcess=Get-Process -Id $record.pid -ErrorAction SilentlyContinue
        if(!$hostProcess){return}
        if($hostProcess.StartTime.ToUniversalTime().Ticks.ToString() -cne $record.started){throw '原部署宿主PID已复用，保留资源。'}
        Start-Sleep -Milliseconds 150
    }
    throw '原部署终端仍未完成退出，保留现场；待其结束再反部署。'
}
# Remove only inventory entries, checking the whole remaining tree before every step and journaling successful removals.
function Remove-VerifiedSnapshot($Original,$Journal,$Retained=@()) {
    $rows=@($Original|Sort-Object @{Expression={$_.path.Split('/').Count};Descending=$true},@{Expression={$_.type -eq 'file'};Descending=$true},path)
    foreach($row in $rows){
        if($row.path -in @($Journal.removed) -or $row.path -in @($Retained|ForEach-Object{$_.path})){continue}
        if($Journal.choice -eq 'backup'){
            if(!(Test-Path -LiteralPath $Journal.backup) -or (Get-FileHash -LiteralPath $Journal.backup -Algorithm SHA256).Hash -cne $Journal.backupHash){throw '原备份在清理过程中变化/丢失，停止续清。'}
            if($Journal.PSObject.Properties['recoveryKey'] -and (!(Test-Path -LiteralPath $Journal.recoveryKey) -or (Get-FileHash -LiteralPath $Journal.recoveryKey -Algorithm SHA256).Hash -cne $Journal.recoveryKeyHash)){throw '匹配解密钥变化/丢失，停止续清。'}
        }
        $expected=@($Original|Where-Object{$_.path -notin @($Journal.removed)})
        if((Get-SnapshotHash (Get-ResourceSnapshot)) -cne (Get-SnapshotHash $expected)){throw '确认后源资源变化；保留剩余源与已验备份，停止清理。'}
        $target=Join-Path $State $row.path;Assert-PlainPath $target
        $identity=Get-ResourceIdentity $target
        if($identity.objectId -cne $row.objectId -or $identity.ownerSid -cne $row.ownerSid -or $identity.links -ne $row.links -or $identity.attributes -ne $row.attributes){throw '删除前对象身份/归属/链接数发生变化，保留。'}
        if($row.type -eq 'file'){
            if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -cne $row.sha256){throw '文件在清理前变化，保留。'}
            [IO.File]::Delete($target)
        }else{[IO.Directory]::Delete($target,$false)}
        $Journal.removed=@($Journal.removed)+$row.path;Write-PrivateJson (Join-Path $State 'undo.json') $Journal
    }
}
# Perform real local inverse operations; external databases remain explicit unresolved resources.
function Undeploy-Instance {
    $origin=Read-Origin
    if(!$origin.sealed){throw '部署原点未封存，未清理。'}
    $journalPath=Join-Path $State 'undo.json'
    if(Test-Path -LiteralPath $journalPath){
        $journal=Get-Content -LiteralPath $journalPath -Raw|ConvertFrom-Json
        if($journal.originHash -cne $origin.snapshotHash -or $journal.choice -cne $DataChoice){throw '已有反部署责任，需沿原数据策略重试。'}
        if($journal.choice -eq 'backup' -and (!(Test-Path -LiteralPath $journal.backup) -or (Get-FileHash -LiteralPath $journal.backup -Algorithm SHA256).Hash -cne $journal.backupHash)){throw '原备份缺失/变化，禁止续清。'}
        if($journal.choice -eq 'backup' -and $journal.PSObject.Properties['recoveryKey'] -and (!(Test-Path -LiteralPath $journal.recoveryKey) -or (Get-FileHash -LiteralPath $journal.recoveryKey -Algorithm SHA256).Hash -cne $journal.recoveryKeyHash)){throw '原匹配解密钥已变化/丢失，未继续。'}
        if($journal.status -in @('local-business-removed-journal-retained','business-removed-recovery-retained')){
            if((Get-SnapshotHash (Get-ResourceSnapshot)) -cne (Get-SnapshotHash (Get-RetainedRecoveryRows $journal.snapshot))){throw '保留journal在反部署后发生变化，未接管。'}
            $script:ExitStatus=2
            Write-Host '本轮业务资源已撤回；独立journal/实例身份/匹配恢复材料仍保留，无需重复清理。';return
        }
    }else{
        $current=@(Get-ResourceSnapshot)
        if((Get-SnapshotHash $current) -cne $origin.snapshotHash){throw '资源清单与部署封存不符，保留未知或后改对象。'}
        $archive='';$archiveHash=''
        if($DataChoice -eq 'backup'){
            $result=Backup-Instance -ForUndeploy
            $archive=$result.archive;$archiveHash=$result.sha256
            if($result.PSObject.Properties['sourceSnapshot']){$current=@($result.sourceSnapshot)}
            elseif((Get-SnapshotHash (Get-ResourceSnapshot)) -cne $origin.snapshotHash){throw '备份期间源资源变化；保留源和原备份，禁止清理。'}
        }
        $journal=[pscustomobject]@{schema=1;originHash=$origin.snapshotHash;choice=$DataChoice;backup=$archive;backupHash=$archiveHash;snapshot=$current;removed=@();status='removing-local-resources';externalPostgreSQL='retained-not-owned'}
        if($DataChoice -eq 'backup' -and $result.PSObject.Properties['recoveryKey']){$journal|Add-Member -NotePropertyName recoveryKey -NotePropertyValue $result.recoveryKey;$journal|Add-Member -NotePropertyName recoveryKeyHash -NotePropertyValue $result.recoveryKeyHash}
        $origin.undoStarted=$true;Write-PrivateJson (Join-Path $State 'origin.json') $origin;Write-PrivateJson $journalPath $journal
    }
    $snapshot=if($journal.PSObject.Properties['snapshot']){@($journal.snapshot)}else{@($origin.snapshot)}
    $retained=@(Get-RetainedRecoveryRows $snapshot)
    Remove-VerifiedSnapshot $snapshot $journal $retained
    if((Get-SnapshotHash (Get-ResourceSnapshot)) -cne (Get-SnapshotHash $retained)){throw '反部署后仍有未知对象，未宣称回到原点。'}
    Protect-Directory $Backups
    $history=Join-Path $Backups ('undeployment-'+[DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfff')+'-'+[Guid]::NewGuid().ToString('N')+'.json')
    $journal.status=if($retained.Count){'business-removed-recovery-retained'}else{'local-resources-removed-external-pg-retained'}
    $journal|Add-Member -NotePropertyName retained -NotePropertyValue @($retained|ForEach-Object{$_.path}) -Force
    $journal|Add-Member -NotePropertyName partial -NotePropertyValue $true -Force
    $journal|Add-Member -NotePropertyName state -NotePropertyValue $State
    $journal|Add-Member -NotePropertyName origin -NotePropertyValue $origin
    Write-PrivateJson $history $journal
    if($retained.Count){
        $journal.status='business-removed-recovery-retained';Write-PrivateJson $journalPath $journal
        $script:ExitStatus=2
        Write-Host "部分反部署：本地业务已撤回；独立journal、Application/instance.json身份、TLS/Secrets/owner及匹配恢复材料保留；state=$State；外置备份/解密钥未清理。"
        return
    }
    foreach($name in @('transaction.lock','undo.json','origin.json')){
        $file=Join-Path $State $name;Assert-PlainPath $file
        if(Test-Path -LiteralPath $file){[IO.File]::Delete($file)}
    }
    if($origin.before -ceq 'absent'){[IO.Directory]::Delete($State,$false)}
    Write-Host "已撤回本项目清单内业务程序/文件/JetStream；审计=$history"
    Write-Host '部分完成：外部PostgreSQL及共享工具仍由维护者保留/处置，未声称数据库已清零或环境完全回原点。'
}

# Fixed built-in description precedes any writable setup or platform/tool checks.
function Show-Introduction {
    Write-Host 'JobsIM Windows 成对部署/反部署：本地默认仅本机（临时穿透可选） / 服务器HTTPS。'
    Write-Host 'check/plan/status/help只读；run写入本项目私有实例，默认自动准备原生业务PG与物理独立journal PG。'
    Write-Host '仅停止保留与完整反部署分别选择；清理默认先外置加密备份验真，清零需独立键盘复核。'
    Write-Host '不DROP外部PG，不清共享SDK、旧备份或匹配恢复钥；缺原点/冲突/取消均保留。'
    Write-Host '运行日志在实例Logs；Ctrl+C/关窗回收本次进程组，数据保留；Windows原生验收另列。'
}
# A structural plan is usable on a non-Windows host without invoking tools, network, locks or writers.
function Show-ReadOnlyPlan {
    $shownState=if($Mode -and !$ExplicitState){Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) "JobsIM/Instances/$RepoId/$Mode"}else{$State}
    if($Action -eq 'status' -and $IsWindows){
        Assert-Owner
        Write-Host ('本实例后端运行：'+(Server-IsRunning))
        $broker=Get-OwnedNats;Write-Host ('本实例NATS运行：'+[bool]$broker);if($broker){$broker.Dispose()}
        foreach($name in @('business','journal')){
            $receiptFile=Join-Path $State ('pg-'+$name+'-process.json')
            if(Test-Path -LiteralPath $receiptFile){
                $receipt=Get-Content -LiteralPath $receiptFile -Raw|ConvertFrom-Json;$process=Get-Process -Id $receipt.pid -ErrorAction SilentlyContinue
                $owned=$process -and $process.StartTime.ToUniversalTime().Ticks.ToString() -ceq $receipt.started -and $process.Path -ceq $receipt.path
                Write-Host ("本实例PG/$name 运行："+[bool]$owned);if($process){$process.Dispose()}
            }
        }
    }
    [pscustomobject]@{action=$Action;project=$Repo;state=$shownState;mode=$(if($Mode){$Mode}else{'interactive-fzf'});roles=$Services;httpsPort=$HttpsPort;nats=$NatsUrl;databaseRoute='default native owned business PG + physically independent retained journal; explicit external option; healthy PG18 bin required';undo='origin-bound local inverse, external PG/shared tools retained';windowsNativeVerified=$false}|ConvertTo-Json -Depth 5|Write-Host
}
# Capture short bounded tool probes without leaking inherited database credentials into their environment.
function Invoke-Capture([string]$Exe,[string[]]$Arguments,[int]$Seconds=30) {
    $start=[Diagnostics.ProcessStartInfo]::new();$start.FileName=$Exe;$start.UseShellExecute=$false;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    foreach($argument in $Arguments){$start.ArgumentList.Add($argument)}
    foreach($name in @($start.Environment.Keys)){if($name -like 'JOBS_IM_*' -or $name -eq 'PGPASSWORD' -or $name -like 'FZF_DEFAULT_*' -or $name -like 'TUNNEL_*'){$start.Environment.Remove($name)|Out-Null}}
    $process=[Diagnostics.Process]::Start($start);if($IsWindows -and ('JobsIMOwnedConsole' -as [type])){[JobsIMOwnedConsole]::Track($process)};$out=$process.StandardOutput.ReadToEndAsync();$err=$process.StandardError.ReadToEndAsync()
    try{
        if(!$process.WaitForExit($Seconds*1000)){$process.Kill($true);$process.WaitForExit();return [pscustomobject]@{code=75;text='query timed out'}}
        return [pscustomobject]@{code=$process.ExitCode;text=$out.GetAwaiter().GetResult().Trim()}
    }finally{$process.Dispose()}
}
# Normalize the resolver's eight-column wire contract, accepting only HTTPS official release origins.
function Convert-AssetRow([string]$Line,[string]$ToolName) {
    $fields=$Line.Split([char]9)
    if($fields.Count -ne 8 -or $Line -match '[\r\n]' -or $fields[0] -cne $ToolName -or $fields[1] -cne 'windows' -or $fields[2] -cne $script:ToolArch -or $fields[3] -notmatch '^\d+\.\d+(\.\d+)?$'){throw '工具发行元数据不匹配本机原生架构。'}
    $asset=[pscustomobject]@{tool=$fields[0];os=$fields[1];arch=$fields[2];version=$fields[3];filename=$fields[4];sha256=$fields[5];url=$fields[6];kind=$fields[7]}
    if($asset.kind -eq 'unsupported'){if($asset.filename -cne '-' -or $asset.sha256 -cne '-' -or $asset.url -cne '-'){throw 'unsupported发行元数据格式无效。'};return $asset}
    $url=[Uri]$asset.url
    if($url.Scheme -ne 'https' -or $url.UserInfo -or $url.Host -notin @('go.dev','github.com','nodejs.org') -or $asset.sha256 -notmatch '^[A-Fa-f0-9]{64}$' -or $asset.filename -match '[/\\:]' -or $asset.kind -notin @('zip','binary')){throw '官方工具来源/摘要/文件名无效，不下载。'}
    $expected=switch($ToolName){
        'go'{'https://go.dev/dl/'+$asset.filename}
        'fzf'{'https://github.com/junegunn/fzf/releases/download/v'+$asset.version+'/'+$asset.filename}
        'cloudflared'{'https://github.com/cloudflare/cloudflared/releases/download/'+$asset.version+'/'+$asset.filename}
        'nats-server'{'https://github.com/nats-io/nats-server/releases/download/v'+$asset.version+'/'+$asset.filename}
        'node'{'https://nodejs.org/dist/v'+$asset.version+'/'+$asset.filename}
        default{throw '无可验证Windows工具下载合同。'}
    }
    if($asset.url -cne $expected){throw '工具地址偏离其官方固定版本，停止。'}
    return $asset
}
# Bootstrap only Go/fzf from the maintained official lock; selected-mode checks always query fresh stable metadata.
function Get-BootstrapAsset([string]$ToolName) {
    $rows=@(Import-Csv -LiteralPath (Join-Path $Repo '部署/deploy-tools.lock.tsv') -Delimiter "`t"|Where-Object{$_.tool -ceq $ToolName -and $_.os -ceq 'windows' -and $_.arch -ceq $script:ToolArch})
    if($rows.Count -ne 1){throw '本机架构无唯一引导成品；请显式提供健康的原生Go/fzf。'}
    $row=$rows[0];return Convert-AssetRow (@($row.tool,$row.os,$row.arch,$row.version,$row.filename,$row.sha256,$row.url,$row.kind)-join "`t") $ToolName
}
# Download into a private versioned machine cache; no existing executable is overwritten.
function Install-VerifiedTool($Asset,[switch]$SingleAttempt) {
    if($Asset.kind -eq 'unsupported'){throw '官方没有可验证的本架构原生成品，请提供健康候选；不伪造摘要或使用仿真包。'}
    $cache=Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) ('JobsIM/ManagedTools/'+$RepoId+'/'+$Asset.tool+'/'+$Asset.version+'-'+$script:ToolArch)
    Assert-PlainPath $cache
    if(Test-Path -LiteralPath $cache){
        $record=Join-Path $cache 'asset.json'
        if(!(Test-Path -LiteralPath $record)){throw '工具缓存无原始来源收据，保留不覆盖。'}
        $metadata=Get-Content -LiteralPath $record -Raw|ConvertFrom-Json
        $binary=Join-Path $cache $metadata.executable
        Assert-PlainPath $binary
        if($metadata.archiveSHA -cne $Asset.sha256 -or (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash -cne $metadata.executableSHA){throw '工具缓存已变化，保留并停止。'}
        return $binary
    }
    Assert-DiskBudget $cache (1024*1024)
    $temporary=$cache+'.pending-'+[Guid]::NewGuid().ToString('N');Protect-Directory $temporary
    $archive=Join-Path $temporary $Asset.filename
    Write-Host ('官方工具：'+$Asset.tool+' '+$Asset.version+'；来源='+$Asset.url)
    $delay=1
    while($true){
        try{Invoke-WebRequest -Uri $Asset.url -OutFile $archive -MaximumRedirection 0 -TimeoutSec 90;break}
        catch{
            $exception=$_.Exception
            if($exception -isnot [Net.Http.HttpRequestException] -and $exception -isnot [TimeoutException]){throw}
            if($SingleAttempt){throw '本次公网工具获取未完成，可返回本机菜单重试。'}
            if(Test-Path -LiteralPath $archive){[IO.File]::Delete($archive)}
            Write-Host "网络获取未完成，${delay}秒后重试，Ctrl+C取消。";Start-Sleep -Seconds $delay;$delay=[Math]::Min(15,$delay*2)
        }
    }
    if((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ine $Asset.sha256){throw '下载SHA不匹配，保留现场且不执行。'}
    $payload=Join-Path $temporary 'payload';[IO.Directory]::CreateDirectory($payload)|Out-Null
    if($Asset.kind -eq 'binary'){Assert-DiskBudget $payload (Get-Item -LiteralPath $archive).Length;[IO.File]::Copy($archive,(Join-Path $payload ($Asset.tool+'.exe')))}
    else{
        $zip=[IO.Compression.ZipFile]::OpenRead($archive)
        try{
            $expanded=0L;foreach($entry in $zip.Entries){if($entry.Length -gt [long]::MaxValue-$expanded){throw 'ZIP展开长度溢出。'};$expanded+=$entry.Length}
            Assert-DiskBudget $payload $expanded
            foreach($entry in $zip.Entries){
                $name=$entry.FullName.Replace('\','/')
                if($name -match '(^/|:|(^|/)\.\.(/|$))' -or (($entry.ExternalAttributes -shr 16) -band 61440) -eq 40960){throw '工具归档含链接/逃逸路径，停止。'}
                $target=[IO.Path]::GetFullPath((Join-Path $payload $name))
                if(!$target.StartsWith($payload+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw '解压路径越界。'}
                if($name.EndsWith('/')){[IO.Directory]::CreateDirectory($target)|Out-Null}
                else{[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))|Out-Null;[IO.Compression.ZipFileExtensions]::ExtractToFile($entry,$target,$false)}
            }
        }finally{$zip.Dispose()}
    }
    $leaf=switch($Asset.tool){'go'{'go.exe'};'node'{'node.exe'};'nats-server'{'nats-server.exe'};default{$Asset.tool+'.exe'}}
    $binaries=@(Get-ChildItem -LiteralPath $payload -File -Recurse|Where-Object{$_.Name -ceq $leaf})
    if($binaries.Count -ne 1){throw '原生成品布局不唯一，停止。'}
    $relative=[IO.Path]::GetRelativePath($temporary,$binaries[0].FullName)
    Write-PrivateJson (Join-Path $temporary 'asset.json') @{tool=$Asset.tool;version=$Asset.version;source=$Asset.url;archiveSHA=$Asset.sha256;executable=$relative;executableSHA=(Get-FileHash -LiteralPath $binaries[0].FullName -Algorithm SHA256).Hash;scope='shared-by-local-and-server-instances'}
    [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($cache))|Out-Null
    [IO.Directory]::Move($temporary,$cache)
    return Join-Path $cache $relative
}
# Verify PE machine and stable version using the executable itself rather than a directory label.
function Test-ToolCandidate([string]$Path,[string]$ToolName,[string]$Target='') {
    if(!$Path -or !(Test-Path -LiteralPath $Path -PathType Leaf)){return $false};Assert-PlainPath $Path
    $stream=[IO.File]::OpenRead($Path)
    try{
        $reader=[IO.BinaryReader]::new($stream)
        if($reader.ReadUInt16() -ne 23117){return $false};$stream.Position=60;$offset=$reader.ReadInt32();$stream.Position=$offset
        if($reader.ReadUInt32() -ne 17744){return $false};$machine=$reader.ReadUInt16()
        if($machine -ne $(if($script:ToolArch -eq 'arm64'){43620}else{34404})){return $false}
    }finally{$stream.Dispose()}
    $flag=if($ToolName -eq 'go'){'version'}else{'--version'};$reply=Invoke-Capture $Path @($flag) 8
    $pattern=switch($ToolName){'go'{'^go version go(\d+\.\d+\.\d+)\s'};'node'{'^v(\d+\.\d+\.\d+)$'};'cloudflared'{'^cloudflared version (\d+\.\d+\.\d+)'};'nats-server'{'^nats-server: v(\d+\.\d+\.\d+)'};'postgres'{'^pg_dump \(PostgreSQL\) (\d+\.\d+(?:\.\d+)?)'};default{'^(\d+\.\d+\.\d+)'}}
    if($reply.code -ne 0 -or $reply.text -notmatch $pattern){return $false};$version=$Matches[1]
    if($Target -and [version]$version -lt [version]$Target){return $false}
    if($ToolName -eq 'go' -and [version]$version -lt [version]'1.27.0'){return $false}
    if($ToolName -eq 'node' -and [version]$version -lt [version]'22.12.0'){return $false}
    return $true
}
# Fresh stable metadata is queried after mode selection; only classified network errors retry.
function Resolve-LatestTool([string]$ToolName,[string]$Candidate,[switch]$SingleAttempt) {
    $delay=1
    while($true){
        $reply=Invoke-Capture $script:Resolver @('--tool',$ToolName,'--os','windows','--arch',$script:ToolArch,'--locked-manifest',(Join-Path $Repo '部署/deploy-tools.lock.tsv')) 40
        if($reply.code -eq 0){break};if($reply.code -ne 75){throw '官方稳定版本/摘要查询失败；保留环境，不猜测最新版。'}
        if($SingleAttempt){throw '本次公网工具版本查询暂不可用，本机服务保留，可菜单重试。'}
        Write-Host "查询暂未完成，${delay}秒后重试；Ctrl+C取消。";Start-Sleep -Seconds $delay;$delay=[Math]::Min(15,$delay*2)
    }
    $asset=Convert-AssetRow $reply.text $ToolName
    if(Test-ToolCandidate $Candidate $ToolName $asset.version){Write-Host "复用健康$ToolName，实际版本满足官方稳定目标$($asset.version)。";return $Candidate}
    $binary=Install-VerifiedTool $asset -SingleAttempt:$SingleAttempt
    if(!(Test-ToolCandidate $binary $ToolName $asset.version)){throw '新工具实际版本/架构/功能检查失败。'}
    return $binary
}
# Bootstrap minimal menu tools into a machine cache, then select mode before building application resources.
function Select-RunMode {
    if($Mode){throw '-Mode仅供plan审阅；真实运行须通过菜单选择。'}
    $script:ToolArch=switch([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()){'X64'{'amd64'};'Arm64'{'arm64'};default{throw '未支持本机原生架构。'}}
    $goCandidate=Get-Command 'go.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $script:Go=if($env:JOBS_IM_GO){$env:JOBS_IM_GO}elseif($goCandidate){$goCandidate.Source}else{''}
    if(!(Test-ToolCandidate $Go 'go')){$script:Go=Install-VerifiedTool (Get-BootstrapAsset 'go')}
    $fzfCandidate=Get-Command 'fzf.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $fzf=if($env:JOBS_IM_FZF_BIN){$env:JOBS_IM_FZF_BIN}elseif($fzfCandidate){$fzfCandidate.Source}else{''}
    if(!(Test-ToolCandidate $fzf 'fzf')){$fzf=Install-VerifiedTool (Get-BootstrapAsset 'fzf')}
    $script:FzfExecutable=$fzf
    $chosen=Select-KeyboardChoice @('local|本地部署（默认仅本机，临时穿透另选另确认）','server|服务器部署（独立state、正式HTTPS URL和证书）','cancel|取消') '回车选择；Esc取消；本地公网仅合成资料'
    if($chosen -eq 'cancel'){throw '已取消，未构建/启动业务；已验引导缓存可复用。'}
    $script:Mode=$chosen
    if(!$ExplicitState){$script:State=Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) "JobsIM/Instances/$RepoId/$Mode";$script:Parent=Split-Path $State -Parent}
    $script:Backups=if($env:JOBS_IM_BACKUP_DIR){[IO.Path]::GetFullPath($env:JOBS_IM_BACKUP_DIR)}else{Join-Path $Parent "Backups/$(Split-Path $State -Leaf)"}
    $script:Recovery=if($env:JOBS_IM_RECOVERY_DIR){[IO.Path]::GetFullPath($env:JOBS_IM_RECOVERY_DIR)}else{Join-Path $Parent "RecoveryKeys/$(Split-Path $State -Leaf)"}
    $script:NatsConfig=Join-Path $State 'Secrets/nats.conf';$script:NatsStore=Join-Path $State 'Application/JetStream';$script:NatsTokenFile=if($env:JOBS_IM_NATS_TOKEN_FILE){[IO.Path]::GetFullPath($env:JOBS_IM_NATS_TOKEN_FILE)}else{Join-Path $State 'Secrets/nats.token'}
    $script:PublicURL="https://127.0.0.1:$HttpsPort";$script:BindAddress='127.0.0.1';$script:HealthResolve=@()
    if($Mode -eq 'server'){
        $uri=Assert-ServerURL $env:JOBS_IM_SERVER_URL
        foreach($path in @($env:JOBS_IM_SERVER_CA_FILE,$env:JOBS_IM_SERVER_CERT_FILE,$env:JOBS_IM_SERVER_KEY_FILE)){if(!$path -or !(Test-Path -LiteralPath $path -PathType Leaf)){throw '服务器模式须提供现有CA/证书/私钥，原件只读且不自动换根。'};Assert-PlainPath $path}
        $script:PublicURL=$uri.GetLeftPart([UriPartial]::Authority);$script:BindAddress=if($env:JOBS_IM_SERVER_BIND){$env:JOBS_IM_SERVER_BIND}else{'0.0.0.0'}
        $ip=$null;if(![Net.IPAddress]::TryParse($BindAddress,[ref]$ip)){throw '服务器监听须为明确IP。'}
        $script:HealthResolve=@('--resolve',"$($uri.DnsSafeHost):$HttpsPort`:127.0.0.1")
    }else{
        $access=Select-KeyboardChoice @('loopback|仅本机（默认，不建立公网入口）','quick|本机＋公开临时Quick（仅合成资料）','cancel|取消') '默认仅本机；公开临时访问需单独明确选择'
        if($access -eq 'cancel'){throw '已取消。'}
        $script:AccessMode=$access
        if($access -eq 'quick' -and (Select-KeyboardChoice @('cancel|取消公开，保留仅本机选择','public|明确公开本次临时HTTPS（仍须JobsIM登录）') '任何获得链接的人可到达入口，仅用合成数据') -ne 'public'){$script:AccessMode='loopback'}
    }
}
# Server URLs are explicit HTTPS origins, with matching listening port and no credentials/path/query.
function Assert-ServerURL([string]$Value) {
    $uri=$null
    if(![Uri]::TryCreate($Value,[UriKind]::Absolute,[ref]$uri) -or $uri.Scheme -cne 'https' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/' -or $uri.Port -ne $HttpsPort -or $uri.Host -in @('0.0.0.0','::','127.0.0.1','localhost')){throw '服务器URL须为HTTPS公网origin、端口匹配本次监听，且不含秘密/路径/查询。'}
    return $uri
}
# Stop/backup/undo need only their operational tools; they never download or build application/SDK resources.
function Find-OperationalTools {
    $script:OpenSSL=Tool 'openssl.exe' $env:JOBS_IM_OPENSSL;$script:Tar=Tool 'tar.exe' ''
    if(!$script:PgBin){$profile=Read-ManagedPgProfile;if($profile){$script:PgBin=$profile.bin}elseif($env:JOBS_IM_PG_BIN){$script:PgBin=$env:JOBS_IM_PG_BIN}}
    $script:PgDump=Tool 'pg_dump.exe' $(if($script:PgBin){Join-Path $script:PgBin 'pg_dump.exe'}else{''})
    $script:PgRestore=Tool 'pg_restore.exe' $(if($script:PgBin){Join-Path $script:PgBin 'pg_restore.exe'}else{''})
}

# Reuse only a verified menu tool from PATH/explicit input or this project's managed machine cache.
function Find-InstalledFzf {
    if($script:FzfExecutable){return $script:FzfExecutable}
    if($env:JOBS_IM_FZF_BIN){return Tool 'fzf.exe' $env:JOBS_IM_FZF_BIN}
    $command=Get-Command 'fzf.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if($command){return $command.Source}
    $cache=Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) ('JobsIM/ManagedTools/'+$RepoId+'/fzf')
    Assert-PlainPath $cache
    if(Test-Path -LiteralPath $cache){
        foreach($directory in @(Get-ChildItem -LiteralPath $cache -Directory|Sort-Object Name -Descending)){
            Assert-PlainPath $directory.FullName;$file=Join-Path $directory.FullName 'asset.json'
            if(!(Test-Path -LiteralPath $file)){continue}
            $metadata=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
            $target=[IO.Path]::GetFullPath((Join-Path $directory.FullName $metadata.executable))
            if(!$target.StartsWith($directory.FullName+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'fzf缓存收据路径越界。'}
            Assert-PlainPath $target
            if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -cne $metadata.executableSHA){throw 'fzf缓存后续变化，未执行。'}
            return $target
        }
    }
    throw '缺可核验的fzf；先准备fzf或使用部署入口引导，反部署不擅自下载依赖。'
}
# Server TLS copies explicit original files into a private staging directory, preserving original CA/key files.
function Prepare-ServerTLS([string]$TLS) {
    $uri=Assert-ServerURL $script:PublicURL
    $inputs=@{ 'ca.crt'=$env:JOBS_IM_SERVER_CA_FILE;'server.crt'=$env:JOBS_IM_SERVER_CERT_FILE;'server.key'=$env:JOBS_IM_SERVER_KEY_FILE }
    foreach($name in $inputs.Keys){Assert-PlainPath $inputs[$name]}
    Invoke-Tool $OpenSSL @('verify','-x509_strict','-purpose','sslserver','-verify_hostname',$uri.DnsSafeHost,'-CAfile',$inputs['ca.crt'],$inputs['server.crt'])
    Invoke-Tool $OpenSSL @('x509','-in',$inputs['server.crt'],'-checkend','86400','-noout')
    $certificate=Invoke-Capture $OpenSSL @('x509','-in',$inputs['server.crt'],'-pubkey','-noout') 10
    $key=Invoke-Capture $OpenSSL @('pkey','-in',$inputs['server.key'],'-pubout') 10
    if($certificate.code -ne 0 -or $key.code -ne 0 -or $certificate.text -cne $key.text){throw '服务器证书与私钥不匹配，未更改旧材料。'}
    if(@(Get-ChildItem -LiteralPath $TLS -Force).Count){
        foreach($name in $inputs.Keys){
            $target=Join-Path $TLS $name
            if(!(Test-Path -LiteralPath $target) -or (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $inputs[$name] -Algorithm SHA256).Hash){throw '服务器TLS发生变化，须受控轮换；本入口不覆盖原点。'}
        }
        return
    }
    $stage=Join-Path $State ('.TLS-server-'+[Guid]::NewGuid().ToString('N'));Protect-Directory $stage
    foreach($name in $inputs.Keys){[IO.File]::Copy($inputs[$name],(Join-Path $stage $name),$false)}
    Protect-SecretFile (Join-Path $stage 'server.key')
    [IO.Directory]::Delete($TLS,$false);[IO.Directory]::Move($stage,$TLS)
}
# The exact binary/process receipt is replaced only by the newly created owned process group.
function Start-BackendProcess([string[]]$Arguments,[hashtable]$Environment) {
    Assert-NoHostStop
    $server=(Start-ConsoleProcess (Join-Path $State 'Bin/JobsIMd.exe') $Arguments $Repo $Environment)
    Write-PrivateJson (Join-Path $State 'process.json') @{pid=$server.Id;started=$server.StartTime.ToUniversalTime().Ticks.ToString();path=(Join-Path $State 'Bin/JobsIMd.exe');sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $State 'Bin/JobsIMd.exe')).Hash;groupCreated=$true}
    $script:ActiveBackend=$server
    return $server
}
# Both local strict-origin checks and public checks require HTTPS trust; no insecure curl/tunnel flag is used.
function Wait-BackendReady($Server,[bool]$Public=$false) {
    for($attempt=0;$attempt -lt 40;$attempt++){
        if($Server.HasExited){throw '本次应用在健康检查前退出。'}
        try{
            $url=if($Public){$script:PublicURL}else{"https://127.0.0.1:$HttpsPort"}
            $arguments=@('--fail','--silent','--show-error','--max-time','2')
            if(!$Public -or $Mode -eq 'server'){$arguments+=@('--cacert',"$State/TLS/ca.crt")}
            Invoke-Tool $Curl ($arguments+@($url+'/api/bootstrap')) @{} (Join-Path $State 'bootstrap.json')
            return
        }catch{Start-Sleep -Milliseconds 250}
    }
    throw '本次严格HTTPS bootstrap未就绪，不开放网页。'
}
# Quick Tunnel uses an empty session config, strict custom origin CA/SAN and no inherited account/TUNNEL state.
function Start-QuickTunnel {
    $config=Join-Path $State 'Secrets/quick-session.yml';[IO.File]::WriteAllText($config,"{}`n");Protect-SecretFile $config
    $log=Join-Path $State ('Logs/quick-tunnel-'+[Guid]::NewGuid().ToString('N')+'.log')
    if(Test-Path -LiteralPath $log){throw '旧隧道日志需先按原点反部署或归档；不从旧日志提取新URL。'}
    $environment=@{};foreach($name in [Environment]::GetEnvironmentVariables().Keys){if([string]$name -like 'TUNNEL_*'){$environment[[string]$name]=$null}}
    $arguments=@('tunnel','--config',$config,'--no-autoupdate','--url',"https://127.0.0.1:$HttpsPort",'--origin-ca-pool',"$State/TLS/ca.crt",'--origin-server-name','localhost','--logfile',$log)
    $script:TunnelProcess=(Start-ConsoleProcess $Cloudflared ([string[]]$arguments) $State $environment)
    Write-PrivateJson (Join-Path $State 'tunnel-process.json') @{pid=$TunnelProcess.Id;started=$TunnelProcess.StartTime.ToUniversalTime().Ticks.ToString();path=$Cloudflared;sha256=(Get-FileHash -LiteralPath $Cloudflared -Algorithm SHA256).Hash;groupCreated=$true}
    for($index=0;$index -lt 180;$index++){
        if($TunnelProcess.HasExited){throw '本次隧道退出，停止应用；不复用外部隧道。'}
        if(Test-Path -LiteralPath $log){
            $text=(Get-Content -LiteralPath $log -Tail 80)-join "`n"
            $match=[regex]::Match($text,'https://[a-z0-9-]+\.trycloudflare\.com')
            if($match.Success){return $match.Value}
        }
        Start-Sleep -Milliseconds 250
    }
    throw '本次45秒内没有取得Quick HTTPS地址，回收本次进程。'
}
# Stop only the original tunnel process group after PID/start/tool SHA revalidation.
function Stop-Tunnel {
    $file=Join-Path $State 'tunnel-process.json'
    if(!(Test-Path -LiteralPath $file)){return}
    $receipt=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    $process=Get-Process -Id $receipt.pid -ErrorAction SilentlyContinue
    if(!$process){return}
    try{Stop-OwnedGroup $process $receipt}finally{$process.Dispose()}
}
# Generate local offline QR cards from the same public URL; a failure retains URLs and running services.
function New-AccessQR {
    try{
        $source=Join-Path $Repo '部署/公共组件/地址二维码';$script:QrBinary=Join-Path $State 'Tools/access-qr.exe'
        Invoke-Tool $Go @('-C',$source,'build','-mod=readonly','-o',$QrBinary,'.') @{GOTOOLCHAIN='local';GOWORK='off'}
        $output=Join-Path $State 'AccessQR';Protect-Directory $output
        $label=if($Mode -eq 'server'){'服务器HTTPS｜Web/原生/CLI API origin'}elseif($AccessMode -eq 'quick'){'本次公网HTTPS｜Web/原生/CLI API origin'}else{'仅本机，手机不可用｜Web/原生/CLI API origin'}
        $arguments=@('--output-dir',$output,'--title','JobsIM 访问入口','--entry',($label+'|'+$script:PublicURL))
        if($Mode -eq 'local' -and $AccessMode -eq 'quick'){$arguments+=@('--entry',("仅本机，手机不可用｜严格TLS需信任实例CA|https://127.0.0.1:$HttpsPort"))}
        $width=0;$height=0;$ansi=$false
        try{
            if(![Console]::IsOutputRedirected){$width=[int]$Host.UI.RawUI.WindowSize.Width;$height=[int]$Host.UI.RawUI.WindowSize.Height}
            $ansi=$width -gt 0 -and $Host.UI.SupportsVirtualTerminal -and ![Environment]::GetEnvironmentVariables().Contains('NO_COLOR') -and $env:TERM -ne 'dumb'
        }catch{$width=0;$height=0;$ansi=$false}
        $arguments+=@('--terminal-width',[string]$width,'--terminal-height',[string]$height)
        if($ansi){$arguments+='--ansi'}
        $display=Join-Path $output 'terminal-output.txt'
        Invoke-Tool $QrBinary $arguments @{} $display
        $rendered=[IO.File]::ReadAllText($display)
        Write-Host $rendered
        [IO.File]::WriteAllText($display,($rendered -replace '\x1b\[[0-9;]*[A-Za-z]',''))
        $script:QrPage=Join-Path $output 'index.html'
        # NO_COLOR不能保证终端黑白极性；只打开本轮自己生成的本地PNG。
        if(!$ansi -or $rendered -notmatch '终端二维码（'){
            if([Environment]::UserInteractive -and $Host.Name -ne 'ServerRemoteHost'){
                Start-Process -FilePath (Join-Path $output '01.png')
                Write-Host '已请求打开本轮本地黑白二维码图片；回环地址仍仅本机可用。'
            }else{Write-Host '当前无本地图形预览；保留本轮PNG与完整URL，不宣称已显示可扫码图。'}
        }
    }catch{Write-Host '离线二维码生成或本机预览未完成；保留真实URL，服务继续，不将二维码失败当API通过。'}
}
# Invalidate the previous generation on normal exit; parent death leaves a dated page that is not availability proof.
function Expire-AccessQR {
    if($QrBinary -and $QrPage -and (Test-Path -LiteralPath $QrPage)){
        try{Invoke-Tool $QrBinary @('--expire','--output-dir',(Split-Path $QrPage -Parent))}catch{Write-Host '旧扫码页未能标记失效，请停止使用该码。'}
    }
}
# Poll service/tunnel liveness while a native keyboard menu can be reopened; no API payload or secret enters URLs.
function Wait-InstanceSession($Server) {
    $script:ActiveBackend=$Server
    Write-Host ('当前用户/API origin：'+$script:PublicURL)
    Write-Host '默认保留本机服务；按 O/回车打开菜单，Ctrl+C停止本次会话。'
    $show=$true
    while($script:ActiveBackend -and !$ActiveBackend.HasExited){
        if($script:TunnelProcess -and $TunnelProcess.HasExited){Recover-TunnelToLocal '本次隧道晚退';$show=$true}
        if($show){
            $items=@('public|打开当前用户网页（默认浏览器）')
            if($QrPage){$items+='qr|打开离线扫码页面'}
            if($Mode -eq 'local'){$items+='retry|明确尝试/重试本次Quick公网入口'}
            $items+=@('diagnose|查看本机与隧道诊断范围','pause|收起菜单（本机服务继续）','stop|停止本次会话并保留数据')
            $choice=Select-KeyboardChoice $items '公网失败保留本机；公开另确认；Esc只收起'
            switch($choice){
                'public'{Start-Process $script:PublicURL}
                'qr'{Start-Process $QrPage}
                'retry'{
                    if((Select-KeyboardChoice @('cancel|取消公开，继续本机','public|明确公开临时HTTPS（仅合成资料）') '公开入口任何获链接者可到达，应用认证仍必需') -eq 'public'){$null=Try-EnableQuick}
                }
                'diagnose'{Write-Host ("严格本机URL=https://127.0.0.1:$HttpsPort；当前origin=$PublicURL；本次隧道日志=$State/Logs；不输出凭据，不降低TLS。")}
                'recover-tunnel'{Recover-TunnelToLocal '菜单期间隧道退出'}
                'stop'{return}
                default{$show=$false}
            }
        }elseif([Console]::KeyAvailable){$key=[Console]::ReadKey($true);if($key.Key -in @([ConsoleKey]::Enter,[ConsoleKey]::O)){$show=$true}}
        Start-Sleep -Milliseconds 200
    }
    if($ActiveBackend -and $ActiveBackend.ExitCode -ne 0){throw '本次后端异常退出，保留数据与原始日志。'}
}
# Rebind only the owned Go process to the actual active origin; instance data and NATS remain untouched.
function Switch-BackendOrigin([string]$Origin,[string]$Access) {
    Stop-Backend
    $script:PublicURL=$Origin;$script:AccessMode=$Access
    $script:BackendEnvironment.JOBS_IM_PUBLIC_URL=$Origin
    $script:ActiveBackend=Start-BackendProcess $script:BackendArguments $script:BackendEnvironment
    Wait-BackendReady $script:ActiveBackend ($Access -eq 'quick')
}
# Public connection and QR publication happens only after strict bootstrap and matching deployment identity.
function Publish-CurrentConnection {
    $bootstrap=Get-Content -LiteralPath (Join-Path $State 'bootstrap.json') -Raw|ConvertFrom-Json
    if($bootstrap.deploymentId -cne $script:ExpectedDeploymentId){throw '当前入口部署身份不一致，未发布连接描述。'}
    Expire-AccessQR
    Write-PrivateJson (Join-Path $State 'connection.json') (New-ConnectionDescriptor $bootstrap $script:CaFingerprint)
    New-AccessQR
    Write-Host ('当前用户/API origin：'+$script:PublicURL)
}
# A failed or late tunnel never unconditionally ends a healthy local session; stale public QR/descriptor are retired.
function Recover-TunnelToLocal([string]$Reason) {
    $script:RecoveringTunnel=$true
    try{
        try{Stop-Tunnel}catch{Write-Host '原隧道身份/退出未确认，保留诊断，不控制未知进程。'}
        $script:TunnelProcess=$null
        Expire-AccessQR
        if($script:PublicURL -ceq "https://127.0.0.1:$HttpsPort" -and $script:ActiveBackend -and !$ActiveBackend.HasExited){$script:AccessMode='loopback';Wait-BackendReady $script:ActiveBackend $false}
        else{Switch-BackendOrigin "https://127.0.0.1:$HttpsPort" 'loopback'}
        Publish-CurrentConnection
        Write-Host ($Reason+'；已严格核验本机服务，保留数据。可菜单重试/诊断/停止，不降TLS。')
    }finally{$script:RecoveringTunnel=$false}
}
# Public access remains opt-in; preparation/start/public-health failures recover to the strict loopback origin.
function Try-EnableQuick {
    if($Mode -ne 'local'){throw '服务器模式不混建临时公网入口。'}
    try{
        $script:RecoveringTunnel=$true
        if($script:TunnelProcess){Stop-Tunnel;$script:TunnelProcess=$null}
        if(!$script:Cloudflared){
            $candidate=Get-Command 'cloudflared.exe' -CommandType Application -ErrorAction SilentlyContinue|Select-Object -First 1
            $script:Cloudflared=Resolve-LatestTool 'cloudflared' $(if($env:JOBS_IM_CLOUDFLARED_BIN){$env:JOBS_IM_CLOUDFLARED_BIN}elseif($candidate){$candidate.Source}else{''}) -SingleAttempt
        }
        $help=Invoke-Capture $Cloudflared @('tunnel','--help') 8
        if($help.code -ne 0 -or $help.text -notmatch 'origin-ca-pool' -or $help.text -notmatch 'origin-server-name'){throw '隧道工具缺严格origin信任功能，不降TLS。'}
        $address=Start-QuickTunnel
        Switch-BackendOrigin $address 'quick'
        Publish-CurrentConnection
        return $true
    }catch{
        Recover-TunnelToLocal '公网准备/启动/严格健康未完成'
        return $false
    }finally{$script:RecoveringTunnel=$false}
}
# Discover native PostgreSQL18 tools; manual input is a bin directory, never eight connection strings.
function Select-PostgresTools {
    $candidates=[Collections.Generic.List[string]]::new()
    if($env:JOBS_IM_PG_BIN){$candidates.Add([IO.Path]::GetFullPath($env:JOBS_IM_PG_BIN))}
    $command=Get-Command 'initdb.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if($command){$candidates.Add([IO.Path]::GetDirectoryName($command.Source))}
    $installation=Join-Path $env:ProgramFiles 'PostgreSQL'
    if(Test-Path -LiteralPath $installation){foreach($directory in @(Get-ChildItem -LiteralPath $installation -Directory)){$candidates.Add((Join-Path $directory.FullName 'bin'))}}
    $healthy=@()
    foreach($candidate in @($candidates.ToArray()|Select-Object -Unique)){
        try{
            Assert-PlainPath $candidate
            $complete=$true;foreach($leaf in @('initdb.exe','postgres.exe','pg_ctl.exe','psql.exe','pg_dump.exe','pg_restore.exe')){if(!(Test-Path -LiteralPath (Join-Path $candidate $leaf) -PathType Leaf)){$complete=$false}}
            if(!$complete){continue}
            $reply=Invoke-Capture (Join-Path $candidate 'pg_dump.exe') @('--version') 8
            if($reply.code -eq 0 -and $reply.text -match '^pg_dump \(PostgreSQL\) 18\.\d+'){$healthy+=$candidate}
        }catch{}
    }
    $items=@();for($index=0;$index -lt $healthy.Count;$index++){$items+=('candidate'+$index+'|已发现健康PG18工具：'+$healthy[$index])}
    $items+=@('manual|选择/拖入本机已安装PostgreSQL18 bin目录','cancel|取消（不下载无摘要EDB包）')
    $choice=Select-KeyboardChoice $items '选择已有原生PG18工具，自动创建本实例双集群；共享安装保留'
    if($choice -eq 'cancel'){throw '已取消PG准备，未创建数据库。'}
    if($choice -eq 'manual'){
        $path=Read-Host '拖入/输入已有PG18 bin目录；空行取消'
        if([string]::IsNullOrWhiteSpace($path)){throw '已取消PG准备。'}
        if($path.Length -ge 2 -and $path.StartsWith('"') -and $path.EndsWith('"')){$path=$path.Substring(1,$path.Length-2)}
        $script:PgBin=[IO.Path]::GetFullPath($path)
    }else{$script:PgBin=$healthy[[int]$choice.Substring(9)]}
    Assert-PlainPath $PgBin
    foreach($leaf in @('initdb.exe','postgres.exe','pg_ctl.exe','psql.exe','pg_dump.exe','pg_restore.exe')){if(!(Test-Path -LiteralPath (Join-Path $PgBin $leaf) -PathType Leaf)){throw '所选bin工具不完整，不能继续。'}}
    if(!(Test-ToolCandidate (Join-Path $PgBin 'pg_dump.exe') 'postgres')){throw 'PG工具架构/版本不匹配，不能用仿真包代替原生。'}
    $version=Invoke-Capture (Join-Path $PgBin 'pg_dump.exe') @('--version') 8
    if($version.text -notmatch '^pg_dump \(PostgreSQL\) 18\.\d+'){throw '本项目当前支持PG18；不跨major升级用户已有集群。'}
}
# Choose automatic instance-owned clusters by default; existing external database configuration is an explicit advanced route.
function Select-DatabaseRoute {
    $profile=Join-Path $State 'Postgres/journal.connection-profile.json'
    if(Test-Path -LiteralPath $profile){
        Assert-PlainPath $profile;$script:PgProfile=Get-Content -LiteralPath $profile -Raw|ConvertFrom-Json
        if($PgProfile.project -cne $Repo -or $PgProfile.state -cne $State -or $PgProfile.schema -ne 1){throw 'PG原点配置不属于本实例。'}
        if(!$PgProfile.initialized){throw '上次PG初始化未完成，保留原件并停止。'}
        $script:DatabaseRoute='managed';$script:PgBin=$PgProfile.bin;$script:DatabaseURLs=@{};foreach($property in $PgProfile.urls.PSObject.Properties){$DatabaseURLs[$property.Name]=[string]$property.Value};return
    }
    $choice=Select-KeyboardChoice @('managed|本实例原生双PG自动准备（默认，无需填写数据库URL）','external|高级：复用维护者已注入的8域外部PG（永不DROP）','cancel|取消') '数据源路线；首次不会接管既有外部数据库'
    if($choice -eq 'cancel'){throw '已取消数据库准备。'}
    if($choice -eq 'managed' -and (Test-Path -LiteralPath (Join-Path $State 'Application/instance.json'))){throw '已有实例未登记托管PG，不能自动切换数据源；请选择原外部库或新state，迁移须走恢复合同。'}
    $script:DatabaseRoute=$choice
    if($choice -eq 'external'){foreach($role in $Roles){$null=Database-Environment $role}}
}
# Validate a private managed profile before using stored URLs or issuing pg_ctl to its own data directories.
function Read-ManagedPgProfile {
    $file=Join-Path $State 'Postgres/journal.connection-profile.json'
    if(!(Test-Path -LiteralPath $file)){return $null}
    Assert-PlainPath $file
    $profile=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    if($profile.schema -ne 1 -or $profile.project -cne $Repo -or $profile.state -cne $State -or $profile.route -cne 'managed'){throw '托管PG配置归属不明，停止。'}
    Assert-PlainPath $profile.bin
    foreach($role in $Roles){
        $uri=[Uri][string]$profile.urls.$role
        $expectedPort=if($role -eq 'journal'){$profile.journalPort}else{$profile.businessPort}
        if($uri.Scheme -notin @('postgres','postgresql') -or $uri.Host -cne '127.0.0.1' -or $uri.Port -ne $expectedPort -or $uri.AbsolutePath -cne ('/jobsim_'+$role)){throw '托管PG路由偏离本实例原配置，未使用。'}
    }
    return $profile
}
# Create clusters and per-domain non-superuser credentials only in a fresh, origin-owned state.
function Initialize-ManagedPg {
    if($script:DatabaseRoute -ne 'managed'){return}
    $profile=Read-ManagedPgProfile
    if($profile){$script:PgProfile=$profile;Start-ManagedPg;return}
    $root=Join-Path $State 'Postgres';Protect-Directory $root
    if(@(Get-ChildItem -LiteralPath $root -Force).Count){throw '未登记PG目录非空，保留且不初始化/接管。'}
    $businessPort=if($env:JOBS_IM_BUSINESS_PG_PORT){[int]$env:JOBS_IM_BUSINESS_PG_PORT}else{17432}
    $journalPort=if($env:JOBS_IM_JOURNAL_PG_PORT){[int]$env:JOBS_IM_JOURNAL_PG_PORT}else{17433}
    foreach($port in @($businessPort,$journalPort)){
        if($port -lt 1025 -or $port -gt 65535 -or $port -eq $HttpsPort -or $port -eq $NatsPort -or $port -in @($RolePort..$RoleLast) -or (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)){throw '专属PG端口无效/占用，不接管现有服务。'}
    }
    if($businessPort -eq $journalPort){throw '业务PG与journal PG必须是物理独立集群/端口。'}
    $urls=@{};foreach($role in $Roles){$password=[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant();$port=if($role -eq 'journal'){$journalPort}else{$businessPort};$urls[$role]="postgresql://jobsim_$role`:$password@127.0.0.1:$port/jobsim_$role`?sslmode=disable"}
    $profile=[pscustomobject]@{schema=1;route='managed';project=$Repo;state=$State;bin=$PgBin;businessPort=$businessPort;journalPort=$journalPort;urls=$urls;initialized=$false}
    foreach($name in @('business','journal')){
        $passwordFile=Join-Path $root ($name+'.admin.password')
        [IO.File]::WriteAllText($passwordFile,[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant());Protect-SecretFile $passwordFile
        Invoke-Tool (Join-Path $PgBin 'initdb.exe') @('-D',(Join-Path $root $name),'--username=jobsim_admin','--auth-host=scram-sha-256','--auth-local=scram-sha-256',('--pwfile='+$passwordFile),'--encoding=UTF8','--no-instructions')
    }
    Write-PrivateJson (Join-Path $root 'journal.connection-profile.json') $profile
    $script:PgProfile=$profile;$script:DatabaseURLs=$urls
    $script:CreatingPg=$true
    Start-ManagedPg
    foreach($role in $Roles){
        $name=if($role -eq 'journal'){'journal'}else{'business'};$port=if($role -eq 'journal'){$journalPort}else{$businessPort}
        $admin=[IO.File]::ReadAllText((Join-Path $root ($name+'.admin.password')))
        $uri=[Uri]$urls[$role];$password=[Uri]::UnescapeDataString($uri.UserInfo.Split(':',2)[1]);$username='jobsim_'+$role
        $pgEnvironment=@{PGHOST='127.0.0.1';PGPORT=[string]$port;PGUSER='jobsim_admin';PGPASSWORD=$admin;PGDATABASE='postgres'}
        Invoke-PgSQL ("CREATE ROLE $username LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '$password';") $pgEnvironment
        Invoke-PgSQL ("CREATE DATABASE $username OWNER $username;") $pgEnvironment
        $pgEnvironment.PGDATABASE=$username
        Invoke-PgSQL ("GRANT EXECUTE ON FUNCTION pg_control_system() TO $username;") $pgEnvironment
        $password='';$admin='';$pgEnvironment.Clear()
    }
    $script:CreatingPg=$false
    $profile.initialized=$true;Write-PrivateJson (Join-Path $root 'journal.connection-profile.json') $profile
}
# Start only this instance's recorded data roots and keep separate physical cluster processes in the owned Job.
function Start-ManagedPg {
    $profile=Read-ManagedPgProfile;if(!$profile){return}
    if(!$profile.initialized -and !$script:CreatingPg){throw '本实例PG初始化未完成，保留现场，不猜测补建角色。'}
    $script:PgProfile=$profile;$script:DatabaseURLs=@{};foreach($property in $profile.urls.PSObject.Properties){$DatabaseURLs[$property.Name]=[string]$property.Value}
    foreach($name in @('business','journal')){
        $port=if($name -eq 'journal'){$profile.journalPort}else{$profile.businessPort};$data=Join-Path $State ('Postgres/'+$name)
        $receiptFile=Join-Path $State ('pg-'+$name+'-process.json')
        if(Test-Path -LiteralPath $receiptFile){
            $receipt=Get-Content -LiteralPath $receiptFile -Raw|ConvertFrom-Json;$existing=Get-Process -Id $receipt.pid -ErrorAction SilentlyContinue
            if($existing){if($existing.StartTime.ToUniversalTime().Ticks.ToString() -cne $receipt.started -or $existing.Path -cne $receipt.path -or $receipt.data -cne $data -or (Get-FileHash -LiteralPath $existing.Path -Algorithm SHA256).Hash -cne $receipt.sha256){throw 'PG PID/原工具/数据路径变化，未接管。'};continue}
        }
        if(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue){throw '托管PG端口被其他资源占用，不接管。'}
        Assert-PlainPath $data
        $binary=Join-Path $profile.bin 'postgres.exe';$ctl=Join-Path $profile.bin 'pg_ctl.exe'
        $process=(Start-ConsoleProcess $binary ([string[]]@('-D',$data,'-h','127.0.0.1','-p',[string]$port,'-c','shared_buffers=64MB','-c','max_connections=64')) $State @{})
        Write-PrivateJson $receiptFile @{pid=$process.Id;started=$process.StartTime.ToUniversalTime().Ticks.ToString();path=$binary;sha256=(Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash;data=$data;ctl=$ctl;ctlSHA=(Get-FileHash -LiteralPath $ctl -Algorithm SHA256).Hash;groupCreated=$true}
        $admin=[IO.File]::ReadAllText((Join-Path $State ('Postgres/'+$name+'.admin.password')))
        $ready=$false
        for($attempt=0;$attempt -lt 40;$attempt++){
            if($process.HasExited){break}
            try{Invoke-Tool (Join-Path $profile.bin 'psql.exe') @('--no-psqlrc','--set','ON_ERROR_STOP=1','--command','SELECT 1') @{PGHOST='127.0.0.1';PGPORT=[string]$port;PGUSER='jobsim_admin';PGPASSWORD=$admin;PGDATABASE='postgres'};$ready=$true;break}catch{Start-Sleep -Milliseconds 250}
        }
        $admin='';if(!$ready){throw '本次托管PG未就绪，不启动后端。'}
    }
}
# Exact receipts and postmaster.pid must agree before pg_ctl sends a fast, checkpointed shutdown to owned clusters.
function Stop-ManagedPg {
    foreach($name in @('business','journal')){
        $file=Join-Path $State ('pg-'+$name+'-process.json')
        if(!(Test-Path -LiteralPath $file)){continue};Assert-PlainPath $file
        $receipt=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json;$process=Get-Process -Id $receipt.pid -ErrorAction SilentlyContinue
        if(!$process){continue}
        try{
            $expected=Join-Path $State ('Postgres/'+$name)
            if($receipt.data -cne $expected -or $process.StartTime.ToUniversalTime().Ticks.ToString() -cne $receipt.started -or $process.Path -cne $receipt.path -or (Get-FileHash -LiteralPath $process.Path -Algorithm SHA256).Hash -cne $receipt.sha256 -or (Get-FileHash -LiteralPath $receipt.ctl -Algorithm SHA256).Hash -cne $receipt.ctlSHA){throw '托管PG身份/工具摘要不匹配，未发送停止请求。'}
            Assert-PlainPath $expected
            $postmaster=Join-Path $expected 'postmaster.pid'
            if(!(Test-Path -LiteralPath $postmaster) -or (Get-Content -LiteralPath $postmaster -TotalCount 1) -cne [string]$receipt.pid){throw 'postmaster PID与本实例收据不匹配。'}
            Invoke-Tool $receipt.ctl @('-D',$expected,'stop','-m','fast','-w','-t','15')
            if(!$process.WaitForExit(15000)){throw '托管PG没有完成停止/检查点，保留数据。'}
        }finally{$process.Dispose()}
    }
}
# Retain the independent journal, matching instance secret, trust/credential material and ownership proof in every ordinary mode.
function Get-RetainedRecoveryRows($Rows) {
    $retained=@($Rows|Where-Object{
        $_.path -ceq 'owner.json' -or $_.path -ceq 'Application/instance.json' -or
        $_.path -match '^(JournalDatabase|Secrets|TLS)(/|$)' -or
        $_.path -match '^Postgres/journal(?:/|\.|$)'
    })
    $parents=@()
    if(@($retained|Where-Object{$_.path -ceq 'Application/instance.json'}).Count){$parents+='Application'}
    if(@($retained|Where-Object{$_.path -match '^Postgres/journal(?:/|\.|$)'}).Count){$parents+='Postgres'}
    return @($Rows|Where-Object{$_.path -in $parents -or $_.path -in @($retained|ForEach-Object{$_.path})})
}
# A decryptable CMS is not a complete backup unless its exact required dump/manifest set is present once with safe paths.
function Assert-BackupManifest([string]$Manifest,[string[]]$Required) {
    $entries=@(Get-Content -LiteralPath $Manifest|Where-Object{$_ -ne ''}|ForEach-Object{([string]$_).Replace('\','/') -replace '^(\./)+',''})
    foreach($entry in $entries){if($entry -match '(^/|:|(^|/)\.\.(/|$)|[\r\n\t])'){throw '解密归档含越界/非法路径，未授予清理资格。'}}
    $files=@($entries|Where-Object{!$_.EndsWith('/')})
    if(@($files|Select-Object -Unique).Count -ne $files.Count){throw '解密归档含重复文件，未授予清理资格。'}
    foreach($name in $Required){if(@($files|Where-Object{$_ -ceq $name}).Count -ne 1){throw '解密备份缺少必需broker清单/域dump/配置/实例身份，保留源。'}}
}
# SQL containing new random role passwords is streamed to stdin, never argv, logs or a source file.
function Invoke-PgSQL([string]$SQL,[hashtable]$Environment) {
    $start=[Diagnostics.ProcessStartInfo]::new();$start.FileName=Join-Path $PgBin 'psql.exe';$start.UseShellExecute=$false;$start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    foreach($argument in @('--no-psqlrc','--set','ON_ERROR_STOP=1')){$start.ArgumentList.Add($argument)}
    foreach($name in $Environment.Keys){$start.Environment[$name]=[string]$Environment[$name]}
    $process=[Diagnostics.Process]::Start($start)
    try{
        [JobsIMOwnedConsole]::Track($process)
        $out=$process.StandardOutput.ReadToEndAsync();$err=$process.StandardError.ReadToEndAsync()
        $process.StandardInput.WriteLine($SQL);$process.StandardInput.Close();$SQL=''
        if(!$process.WaitForExit(30000)){$process.Kill($true);$process.WaitForExit();throw '本次数据库初始化超时，保留现场。'}
        if($process.ExitCode -ne 0){throw '本次数据库初始化失败，未回显可能含秘密的SQL错误。'}
    }finally{$process.Dispose()}
}

# The public descriptor follows the actual TLS endpoint; the private origin CA never becomes a Cloudflare edge trust root.
function New-ConnectionDescriptor($Bootstrap,[string]$Fingerprint) {
    $connection=@{deploymentId=$Bootstrap.deploymentId;url=$script:PublicURL;protocolVersion=$Bootstrap.protocolVersion}
    if($Mode -eq 'local' -and $AccessMode -eq 'quick'){$connection.tlsTrust='system';$connection.originURL="https://127.0.0.1:$HttpsPort";$connection.originCaSha256=$Fingerprint}
    else{$connection.tlsTrust='instance-ca';$connection.caSha256=$Fingerprint}
    return $connection
}
# A paired stop request fences late retry/fallback starts in the exact old host without signaling an unrelated process.
function Request-HostStop {
    $file=Join-Path $State 'host-process.json';if(!(Test-Path -LiteralPath $file)){return}
    Assert-PlainPath $file;$hostRecord=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    if([int]$hostRecord.pid -eq $PID -or ($hostRecord.PSObject.Properties['active'] -and !$hostRecord.active)){return}
    $process=Get-Process -Id $hostRecord.pid -ErrorAction SilentlyContinue
    if(!$process){return}
    try{
        if($process.StartTime.ToUniversalTime().Ticks.ToString() -cne $hostRecord.started){throw '原宿主PID已复用，未提交停止请求。'}
        Write-PrivateJson (Join-Path $State 'stop-request.json') @{pid=$hostRecord.pid;started=$hostRecord.started;runId=$(if($hostRecord.PSObject.Properties['runId']){$hostRecord.runId}else{''});nonce=[Guid]::NewGuid().ToString('N')}
    }finally{$process.Dispose()}
}
# A stop already requested for this host prevents a late Quick callback from reviving its backend.
function Assert-NoHostStop {
    $file=Join-Path $State 'stop-request.json';if(!(Test-Path -LiteralPath $file)){return}
    Assert-PlainPath $file;$request=Get-Content -LiteralPath $file -Raw|ConvertFrom-Json
    if($request.PSObject.Properties['runId'] -and $request.runId -ceq $script:RunId -and [int]$request.pid -eq $PID -and $request.started -ceq (Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks.ToString()){throw '本宿主已收到配对停止请求，不再重启本轮应用。'}
}
# Query the actual destination volume; this is a reserve policy, not a claimed Windows minimum configuration.
function Get-FreeDiskBytes([string]$Path) {
    $root=[IO.Path]::GetPathRoot([IO.Path]::GetFullPath($Path))
    return [long]([IO.DriveInfo]::new($root).AvailableFreeSpace)
}
# Keep the 4 GiB reserve untouched, rejecting negative/overflowing additional budgets.
function Assert-DiskBudget([string]$Path,[long]$Additional=0) {
    $free=Get-FreeDiskBytes $Path
    $reserve=4L*1024*1024*1024
    if($Additional -lt 0 -or $free -lt $reserve -or $Additional -gt $free-$reserve){throw '实际目标卷不足以保留4GiB及本阶段新增字节；保留数据并停止。'}
}
# Stream only verified-origin HTTPS downloads, reserving the announced size and checking every actual write.
function Receive-ToolArchive([string]$URL,[string]$Destination) {
    $client=[Net.Http.HttpClient]::new();$client.Timeout=[TimeSpan]::FromSeconds(90)
    $response=$null;$input=$null;$output=$null
    try{
        $response=$client.GetAsync($URL,[Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        [void]$response.EnsureSuccessStatusCode()
        if($response.RequestMessage.RequestUri.Scheme -cne 'https'){throw '工具下载离开HTTPS，拒绝写入。'}
        $length=$response.Content.Headers.ContentLength
        if($null -ne $length -and $length -gt 0){Assert-DiskBudget $Destination ([long]$length)}else{Assert-DiskBudget $Destination 0}
        $input=$response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $output=[IO.File]::Open($Destination,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        $buffer=[byte[]]::new(65536);$written=0L;$timer=[Diagnostics.Stopwatch]::StartNew()
        while($true){
            if($timer.Elapsed.TotalSeconds -gt 180){throw [TimeoutException]::new('本次工具下载总时限已到，保留原环境。')}
            $timeout=[Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(30))
            try{$count=$input.ReadAsync($buffer,0,$buffer.Length,$timeout.Token).GetAwaiter().GetResult()}catch [OperationCanceledException]{throw [TimeoutException]::new('本次工具下载流超时，保留原环境。')}finally{$timeout.Dispose()}
            if($count -le 0){break}
            Assert-DiskBudget $Destination $count
            $output.Write($buffer,0,$count);$written+=$count
        }
        $output.Flush($true)
        if($null -ne $length -and $length -ge 0 -and $written -ne $length){throw '下载实际字节数与Content-Length不符，未执行工具。'}
    }finally{if($output){$output.Dispose()};if($input){$input.Dispose()};if($response){$response.Dispose()};$client.Dispose()}
}
# Derive backup headroom from actual source and dump sizes, with room for tar/CMS/decrypt copies and metadata.
function Assert-BackupBudget([string]$Stage,[string[]]$Files) {
    $bytes=0L;$entries=0L
    foreach($row in @(Get-ResourceSnapshot)){
        $entries++
        if($row.type -eq 'file'){$size=(Get-Item -LiteralPath (Join-Path $State $row.path)).Length;if($size -gt [long]::MaxValue-$bytes){throw '备份源大小溢出。'};$bytes+=$size}
    }
    foreach($name in $Files){$size=(Get-Item -LiteralPath (Join-Path $Stage $name)).Length;if($size -gt [long]::MaxValue-$bytes){throw '备份大小溢出。'};$bytes+=$size;$entries++}
    if($entries -gt (([long]::MaxValue-$bytes)/2048)){throw '备份预算溢出。'}
    $total=$bytes+2048L*$entries
    if($total -gt (([long]::MaxValue-67108864)/6)){throw '备份预算溢出。'}
    $additional=6L*$total+64L*1024*1024
    Assert-DiskBudget $Backups $additional
    Write-Host ("备份预算：按当前源/dump实际$bytes 字节及$entries 个条目计算，新增预留$additional 字节，另保留4GiB。")
}

# Query actual logical database size with private libpq environment, never a password-bearing argument.
function Get-DatabaseBudgetBytes([string]$Role,[string]$OutputFile) {
    $psql=Tool 'psql.exe' $(if($script:PgBin){Join-Path $script:PgBin 'psql.exe'}elseif($env:JOBS_IM_PG_BIN){Join-Path $env:JOBS_IM_PG_BIN 'psql.exe'}else{''})
    Invoke-Tool $psql @('--no-psqlrc','--tuples-only','--no-align','--set','ON_ERROR_STOP=1','--command','SELECT pg_database_size(current_database())') (Pg-Environment (Database-Environment $Role)) $OutputFile
    $value=0L
    if(![long]::TryParse([IO.File]::ReadAllText($OutputFile).Trim(),[ref]$value) -or $value -lt 0){throw '数据库实际大小无法可靠解析，未继续dump。'}
    return $value
}
# One native launch boundary keeps PID/Job creation injectable in explicitly isolated host tests.
function Start-ConsoleProcess([string]$Executable,[string[]]$Arguments,[string]$Directory,[hashtable]$Environment){
    return [JobsIMOwnedConsole]::Start($Executable,$Arguments,$Directory,$Environment)
}
# Only main orchestrates actions; check/plan/help do not create locks, secrets, caches or services.
function Main {
    Show-Introduction
    if($Check){$script:Action='check'};if($Plan){$script:Action='plan'}
    if($Check -and $Plan){throw '-Check/-Plan不可同时使用。'}
    Assert-StatePath
    if($Action -eq 'help'){Write-Host '部署/反部署.ps1 check|plan|status|run|stop|backup|undeploy [-State 目录] [-Mode local|server（仅计划）] [-Yes] [-NoBackup（仅stop）]';return}
    if($Action -in @('check','plan','status')){Show-ReadOnlyPlan;return}
    if(!$IsWindows){throw '真实部署/反部署只支持Windows；当前宿主仅解析/隔离检查。'}
    Confirm-Action
    if($Action -eq 'run'){
        Initialize-OwnedConsole
        Select-RunMode
        Assert-StatePath;Assert-Owner
        Request-HostStop
        Stop-Backend;Require-Stopped;Stop-Nats;Wait-RunFinalization;Stop-ManagedPg
        Enter-Transaction
        Prepare-State
        try{Initialize-OwnedConsole;Find-Tools;Initialize-ManagedPg;Run-Instance}finally{
            if(!$script:Lock){Enter-Transaction}
            Stop-Backend;Require-Stopped;Stop-Nats
            if($script:TunnelProcess -and !$TunnelProcess.HasExited){Stop-Tunnel}
            Expire-AccessQR
            Stop-ManagedPg
            Seal-Origin
        }
        return
    }
    Assert-Owner
    if(!(Test-Path -LiteralPath $State)){Write-Host '实例目录不存在，无本地资源可反部署；外部PG和共享工具未处理。';return}
    if($Action -eq 'undeploy'){
        if($NoBackup){throw '-NoBackup不能扩大为反部署清零；请在数据策略菜单中选择。'}
        $entry=Select-KeyboardChoice @('stop|仅停止并保留（默认）','clean|进入反部署清理','cancel|取消') '停止保留与反部署清理是不同操作'
        if($entry -eq 'cancel'){return}
        if($entry -eq 'stop'){$script:Action='stop';$script:NoBackup=$true}
        elseif(!(Confirm-Undeploy)){return}
        if($Action -eq 'undeploy'){$null=Read-Origin}
    }
    if($Action -eq 'backup' -or ($Action -eq 'undeploy' -and $DataChoice -eq 'backup') -or ($Action -eq 'stop' -and !$NoBackup)){Find-OperationalTools}
    Request-HostStop
    Stop-Backend;Require-Stopped;Stop-Nats;Wait-RunFinalization;Stop-ManagedPg
    Enter-Transaction
    if($Action -eq 'undeploy'){Undeploy-Instance;return}
    if($Action -eq 'backup' -or !$NoBackup){
        if(Test-Path -LiteralPath (Join-Path $State 'origin.json')){$existingOrigin=Read-Origin;if($existingOrigin.sealed -and !$existingOrigin.undoStarted){$script:Origin=$existingOrigin}}
        Backup-Instance
        if($script:Origin){Seal-Origin}
    }
    Write-Host '本实例已停止；PG、应用数据、证书、备份与匹配钥全部保留。'
}
if($MyInvocation.InvocationName -ne '.'){
    try{Main;if($script:ExitStatus){exit $script:ExitStatus}}catch{Write-Error $_.Exception.Message -ErrorAction Continue;exit 2}
    finally{if($Lock){$Lock.Dispose();$script:Lock=$null}}
}
