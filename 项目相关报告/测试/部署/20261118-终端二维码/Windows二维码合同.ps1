param([string]$Repo,[string]$Report,[string]$Go)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$records=@()
foreach($file in @(Get-ChildItem -LiteralPath (Join-Path $Repo '部署/Windows') -Recurse -File -Filter '*.ps1'|Where-Object {$_.Name -in @('【Windows】部署.ps1','【Windows】反部署.ps1')})){
    $tokens=$null;$errors=$null
    $ast=[System.Management.Automation.Language.Parser]::ParseFile($file.FullName,[ref]$tokens,[ref]$errors)
    if($errors.Count){throw ($errors|Out-String)}
    $definitions=@($ast.FindAll({param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -in @('Invoke-Tool','New-AccessQR')},$false)|ForEach-Object {$_.Extent.Text})
    if($definitions.Count -ne 2){throw 'missing QR functions'}
    . ([ScriptBlock]::Create(($definitions -join "`n")))
    $script:State=Join-Path $Report ('Windows调用-'+$file.BaseName)
    [IO.Directory]::CreateDirectory($State)|Out-Null
    $script:Backups=Join-Path $State 'Backups'
    $script:Mode='local';$script:AccessMode='loopback';$script:PublicURL='https://127.0.0.1:19443';$script:HttpsPort=19443
    function Assert-DiskBudget {param($path,$extra)}
    function Protect-Directory {param($path) [IO.Directory]::CreateDirectory($path)|Out-Null}
    function Start-Process {param([string]$FilePath) if(!(Test-Path -LiteralPath $FilePath -PathType Leaf)){throw 'preview is not an owned PNG'};[IO.File]::WriteAllText((Join-Path $State 'preview-path.txt'),$FilePath)}
    $env:NO_COLOR=''
    New-AccessQR
    $display=Join-Path $State 'AccessQR/terminal-output.txt'
    $plain=[IO.File]::ReadAllText($display)
    if($plain.Contains([char]27)){throw 'ANSI leaked into plain output'}
    if(!$plain.Contains('仅本机，手机不可用')){throw 'loopback label missing'}
    if(!(Test-Path -LiteralPath $script:QrPage)){throw 'page missing'}
    if(!(Test-Path -LiteralPath (Join-Path $State 'preview-path.txt'))){throw 'native own PNG request missing'}
    $records+=@{file=$file.FullName;syntax='passed';functionCall='passed';host='MacOS PowerShell; no Windows-native claim';noColorEmptyHonored=$true;ownPNGRequested=$true;loopbackLabel=$true}
}
if($records.Count -ne 2){throw 'expected two scripts'}
[IO.File]::WriteAllText((Join-Path $Report 'Windows二维码合同.json'),($records|ConvertTo-Json -Depth 5))
'Windows2 parse and isolated QR function calls passed'
