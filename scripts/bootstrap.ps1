<# Install a verified Windows release without Python, Bash, make or compilers.
   Source fallback uses the portable Python installer. #>
param(
    [Parameter(Mandatory = $true)][string]$Repo,
    [Parameter(Mandatory = $true)][string]$Client,
    [ValidateSet('ide', 'native')][string]$View = 'ide',
    [string]$SourceDir,
    [string]$Release = 'latest',
    [switch]$Source,
    [switch]$BuildOnly,
    [string]$Daemon
)
$ErrorActionPreference = 'Stop'
$utf8Output = New-Object System.Text.UTF8Encoding -ArgumentList $false
[Console]::OutputEncoding = $utf8Output
$OutputEncoding = $utf8Output
Add-Type -AssemblyName System.Net.Http
Add-Type -AssemblyName System.IO.Compression.FileSystem

function Get-StefResource([string]$Url, [long]$Maximum) {
    $handler = New-Object System.Net.Http.HttpClientHandler
    $clientHttp = New-Object System.Net.Http.HttpClient -ArgumentList $handler
    $cancel = New-Object System.Threading.CancellationTokenSource
    $cancel.CancelAfter(60000)
    $clientHttp.DefaultRequestHeaders.UserAgent.ParseAdd('stef-installer')
    $response = $null
    $stream = $null
    $memory = New-Object System.IO.MemoryStream
    try {
        $response = $clientHttp.GetAsync($Url, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead, $cancel.Token).GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) { throw ('GitHub returned HTTP ' + [int]$response.StatusCode) }
        $final = $response.RequestMessage.RequestUri
        $allowed = @('github.com', 'api.github.com', 'raw.githubusercontent.com', 'release-assets.githubusercontent.com', 'objects.githubusercontent.com', 'github-releases.githubusercontent.com')
        if ($final.Scheme -ne 'https' -or $allowed -notcontains $final.Host) { throw 'GitHub redirected outside its HTTPS release hosts.' }
        $length = $response.Content.Headers.ContentLength
        if ($null -ne $length -and $length -gt $Maximum) { throw 'The GitHub download is too large.' }
        $stream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $buffer = New-Object byte[] 65536
        while (($read = $stream.ReadAsync($buffer, 0, $buffer.Length, $cancel.Token).GetAwaiter().GetResult()) -gt 0) {
            if ($memory.Length + $read -gt $Maximum) { throw 'The GitHub download is too large.' }
            $memory.Write($buffer, 0, $read)
        }
        return ,$memory.ToArray()
    } finally {
        if ($stream) { $stream.Dispose() }
        if ($response) { $response.Dispose() }
        $memory.Dispose()
        $clientHttp.Dispose()
        $cancel.Dispose()
    }
}

function Assert-StefAsset($Asset, [string]$Tag) {
    $uri = [Uri]$Asset.browser_download_url
    $expected = '/ivandosevv/stef/releases/download/' + $Tag + '/' + $Asset.name
    if ($uri.Scheme -ne 'https' -or $uri.Host -ne 'github.com' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or [Uri]::UnescapeDataString($uri.AbsolutePath) -ne $expected) {
        throw 'The release asset does not belong to the stef GitHub repository.'
    }
    return $uri.AbsoluteUri
}

function Assert-RuntimePaths([string]$Project) {
    foreach ($relative in @('.stef', '.stef/runtime', '.stef/runtime/bin')) {
        $path = Join-Path $Project $relative
        if (Test-Path -LiteralPath $path) {
            $item = Get-Item -LiteralPath $path -Force
            if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                throw 'Keep .stef/runtime inside the project without directory links or junctions.'
            }
        }
    }
}

function Expand-StefArchive([string]$ArchivePath, [string]$Destination) {
    $zip = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' -ArgumentList ([StringComparer]::OrdinalIgnoreCase)
    [long]$total = 0
    try {
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName.TrimEnd('/')
            $parts = $name.Split('/')
            if (-not $name -or $name.StartsWith('/') -or $name.Contains('\') -or $name.Contains(':') -or $parts -contains '..' -or $parts -contains '.' -or $name.IndexOf([char]0) -ge 0) {
                throw 'Unsafe path in the release archive.'
            }
            foreach ($part in $parts) {
                if ($part -match '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)' -or $part.TrimEnd([char[]]'. ') -ne $part) { throw 'Unsafe Windows filename in the release archive.' }
            }
            $mode = ($entry.ExternalAttributes -shr 16) -band 0xF000
            if ($mode -ne 0 -and $mode -ne 0x8000 -and $mode -ne 0x4000) { throw 'Release archives cannot contain links or special files.' }
            $total += $entry.Length
            if (-not $seen.Add($name) -or $seen.Count -gt 512 -or $entry.Length -gt 67108864 -or $total -gt 209715200) { throw 'Duplicate or oversized files in the release archive.' }
            $output = [IO.Path]::GetFullPath((Join-Path $Destination $name))
            $prefix = $Destination.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
            if (-not $output.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe path in the release archive.' }
            if ($entry.FullName.EndsWith('/')) {
                [IO.Directory]::CreateDirectory($output) | Out-Null
                continue
            }
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($output)) | Out-Null
            $inputStream = $entry.Open()
            $outputStream = [IO.File]::Open($output, [IO.FileMode]::CreateNew)
            try {
                $buffer = New-Object byte[] 65536
                [long]$written = 0
                while (($read = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    $written += $read
                    if ($written -gt $entry.Length) { throw 'Oversized file in the release archive.' }
                    $outputStream.Write($buffer, 0, $read)
                }
                if ($written -ne $entry.Length) { throw 'Incomplete file in the release archive.' }
            } finally { $inputStream.Dispose(); $outputStream.Dispose() }
        }
    } finally { $zip.Dispose() }
}

function Invoke-StefSource([string]$Project, [string]$RuntimeRoot) {
    $pythonCommand = Get-Command py -CommandType Application -ErrorAction SilentlyContinue
    $prefix = @('-3')
    if (-not $pythonCommand) {
        $pythonCommand = Get-Command python3 -CommandType Application -ErrorAction SilentlyContinue
        if (-not $pythonCommand) { $pythonCommand = Get-Command python -CommandType Application -ErrorAction SilentlyContinue }
        $prefix = @()
    }
    if (-not $pythonCommand) { throw 'No matching release is available. Source fallback needs Python 3.9+, Git, Go 1.25+, Node 22.12+ and npm.' }
    $installer = Join-Path (Split-Path $PSScriptRoot -Parent) 'install.py'
    if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) {
        foreach ($tool in @('git', 'go', 'node', 'npm.cmd')) {
            if (-not (Get-Command $tool -CommandType Application -ErrorAction SilentlyContinue)) { throw ('Source fallback needs Git, Go 1.25+, Node 22.12+ and npm. Missing: ' + $tool) }
        }
        $bootstrap = Join-Path $RuntimeRoot 'bootstrap'
        [IO.Directory]::CreateDirectory($bootstrap) | Out-Null
        $installer = Join-Path $bootstrap 'install.py'
        [IO.File]::WriteAllBytes($installer, (Get-StefResource 'https://raw.githubusercontent.com/ivandosevv/stef/master/install.py' 2097152))
    }
    $arguments = @($installer, '--source', '--repo', $Project, '--client', $Client, '--view', $View)
    if ($SourceDir) { $arguments += @('--source-dir', $SourceDir) }
    if ($BuildOnly) { $arguments += '--build-only' }
    if ($Daemon) { $arguments += @('--daemon', $Daemon) }
    $previousAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { $result = & $pythonCommand.Source @prefix @arguments } finally { $ErrorActionPreference = $previousAction }
    if ($LASTEXITCODE -ne 0) { throw 'The stef source build or setup failed.' }
    [Console]::WriteLine(($result -join [Environment]::NewLine))
}

try {
    if (-not [IO.Path]::IsPathRooted($Repo) -or -not (Test-Path -LiteralPath $Repo -PathType Container)) { throw 'Choose an existing absolute project folder with -Repo.' }
    $project = (Resolve-Path -LiteralPath $Repo).ProviderPath.TrimEnd([IO.Path]::DirectorySeparatorChar)
    if ([IO.Path]::GetPathRoot($project).TrimEnd([IO.Path]::DirectorySeparatorChar) -eq $project -or $project -eq [IO.Path]::GetFullPath($HOME).TrimEnd([IO.Path]::DirectorySeparatorChar)) { throw 'Choose your project folder, not your home folder or a filesystem root.' }
    if (-not $Client.Trim()) { throw 'Choose the agent doing the work with -Client, or generic for other MCP clients.' }
    Assert-RuntimePaths $project
    $runtimeRoot = Join-Path $project '.stef/runtime'
    if ($Source) { Invoke-StefSource $project $runtimeRoot; exit 0 }
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    $goarch = switch ($architecture) { 'AMD64' { 'amd64' }; 'ARM64' { 'arm64' }; default { throw 'Windows releases require x64 or ARM64.' } }
    if ($Release -eq 'latest') { $apiUrl = 'https://api.github.com/repos/ivandosevv/stef/releases/latest' }
    elseif ($Release -match '^v?([0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?)$') { $apiUrl = 'https://api.github.com/repos/ivandosevv/stef/releases/tags/v' + $Matches[1] }
    else { throw 'Use latest or a release version such as 0.3.0.' }
    $releaseDocument = $null
    try { $releaseDocument = [Text.Encoding]::UTF8.GetString((Get-StefResource $apiUrl 2097152)) | ConvertFrom-Json }
    catch { if ($Release -ne 'latest' -or $_.Exception.Message -notmatch 'HTTP 404') { throw } }
    $archiveAsset = @()
    if ($releaseDocument) {
        if ($releaseDocument.draft -or $releaseDocument.prerelease -or $releaseDocument.tag_name -notmatch '^v?([0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?)$') { throw 'Use a published stable stef release.' }
        $version = $Matches[1]
        $archiveName = 'stef-' + $version + '-windows-' + $goarch + '.zip'
        $archiveAsset = @($releaseDocument.assets | Where-Object { $_.name -eq $archiveName })
    }
    if (-not $releaseDocument -or $archiveAsset.Count -eq 0) {
        if ($Release -ne 'latest') { throw 'The requested Windows release was not found.' }
        [Console]::Error.WriteLine('No matching portable release yet; using source fallback for your current project.')
        Invoke-StefSource $project $runtimeRoot
        exit 0
    }
    $checks = @($releaseDocument.assets | Where-Object { $_.name -eq ($archiveName + '.sha256') })
    if ($archiveAsset.Count -ne 1 -or $checks.Count -ne 1) { throw 'The release needs one matching archive and its SHA256 file.' }
    $checksum = [Text.Encoding]::ASCII.GetString((Get-StefResource (Assert-StefAsset $checks[0] $releaseDocument.tag_name) 4096)).Trim()
    if ($checksum -notmatch ('^([a-fA-F0-9]{64})\s+\*?' + [Regex]::Escape($archiveName) + '$')) { throw 'The release SHA256 file is invalid.' }
    $expectedHash = $Matches[1]
    [IO.Directory]::CreateDirectory($runtimeRoot) | Out-Null
    $stage = Join-Path $runtimeRoot ('.stef-release-' + [Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    try {
        $archivePath = Join-Path $stage 'release.zip'
        [IO.File]::WriteAllBytes($archivePath, (Get-StefResource (Assert-StefAsset $archiveAsset[0] $releaseDocument.tag_name) 104857600))
        if ((Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash -ne $expectedHash) { throw 'Release checksum mismatch. Your existing stef was preserved.' }
        $extracted = Join-Path $stage 'files'
        [IO.Directory]::CreateDirectory($extracted) | Out-Null
        Expand-StefArchive $archivePath $extracted
        $compiled = Join-Path $extracted 'stef.exe'
        if (-not (Test-Path -LiteralPath $compiled -PathType Leaf) -or (Get-Item -LiteralPath $compiled).Length -eq 0 -or -not (Test-Path -LiteralPath (Join-Path $extracted 'LICENSE') -PathType Leaf)) { throw 'The release is missing its stef binary or license.' }
        [IO.File]::Copy((Join-Path $extracted 'LICENSE'), (Join-Path $runtimeRoot 'LICENSE'), $true)
        $binDirectory = Join-Path $runtimeRoot 'bin'
        [IO.Directory]::CreateDirectory($binDirectory) | Out-Null
        $binary = Join-Path $binDirectory 'stef.exe'
        if (Test-Path -LiteralPath $binary) {
            if ((Get-Item -LiteralPath $binary -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'The existing stef binary cannot be a link.' }
            if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $compiled -Algorithm SHA256).Hash) {
                try { [IO.File]::Replace($compiled, $binary, $null) } catch { throw 'Close running stef processes before updating, then reconnect its MCP. Your existing binary was preserved.' }
            }
        } else { [IO.File]::Move($compiled, $binary) }
    } finally { if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force } }
    $receipt = @{ installed = $true; install_mode = 'release'; executable = $binary; repo = $project; version = $version }
    if ($BuildOnly) { $receipt.enabled = $false }
    else {
        $enableArgs = @('--enable', '--client', $Client, '--view', $View, '--repo', $project)
        if ($Daemon) { $enableArgs += @('--daemon', $Daemon) }
        $previousAction = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try { $result = & $binary @enableArgs } finally { $ErrorActionPreference = $previousAction }
        if ($LASTEXITCODE -ne 0) { throw 'The stef MCP setup failed; the project choice was preserved.' }
        $enabled = ($result -join [Environment]::NewLine) | ConvertFrom-Json
        foreach ($property in $enabled.PSObject.Properties) { $receipt[$property.Name] = $property.Value }
    }
    $receipt | ConvertTo-Json -Depth 30
} catch {
    [Console]::Error.WriteLine('stef install: ' + $_.Exception.Message)
    exit 1
}
