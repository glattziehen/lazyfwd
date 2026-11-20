# Installs the lazyfwd binary from a GitHub release.
#   irm https://github.com/glattziehen/lazyfwd/releases/latest/download/install.ps1 | iex
# LAZYFWD_VERSION picks a release tag (default: latest); LAZYFWD_INSTALL_DIR the target
# (default: %LOCALAPPDATA%\Programs\lazyfwd); LAZYFWD_DOWNLOAD_URL a mirror of the release assets;
# LAZYFWD_NO_MODIFY_PATH leaves the user PATH alone.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # Windows PowerShell's progress bar slows downloads tenfold.

$repo = 'glattziehen/lazyfwd'
$version = if ($env:LAZYFWD_VERSION) { $env:LAZYFWD_VERSION } else { 'latest' }
$dir = if ($env:LAZYFWD_INSTALL_DIR) { $env:LAZYFWD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\lazyfwd' }

function Fail($msg) {
  throw "lazyfwd install: $msg"
}

if ($env:OS -ne 'Windows_NT') { Fail 'this installer is for Windows; use install.sh elsewhere' }

# The registry holds the machine's architecture; emulated processes (Windows PowerShell on ARM)
# see AMD64 in $env:PROCESSOR_ARCHITECTURE and in .NET's OSArchitecture.
$native = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment').PROCESSOR_ARCHITECTURE
$arch = switch ($native) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { Fail "no binary for $_; install with go install instead" }
}

$base = if ($env:LAZYFWD_DOWNLOAD_URL) { $env:LAZYFWD_DOWNLOAD_URL }
elseif ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" }
else { "https://github.com/$repo/releases/download/$version" }

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "lazyfwd-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  # Archive names carry the version, so the checksums tell which archive the release has.
  Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
  $line = Get-Content (Join-Path $tmp 'checksums.txt') |
    Where-Object { ($_ -split '\s+')[1] -match "^lazyfwd_[0-9][^_]*_windows_$arch\.zip$" } |
    Select-Object -First 1
  if (-not $line) { Fail "checksums.txt lists no archive for windows_$arch" }
  $expected, $asset = $line -split '\s+'

  Write-Host "downloading $base/$asset"
  Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
  # Through .NET rather than Get-FileHash and Expand-Archive: those come from modules, which a
  # Windows PowerShell started with another PowerShell's module path cannot find.
  $stream = [System.IO.File]::OpenRead((Join-Path $tmp $asset))
  try { $actual = [System.BitConverter]::ToString([System.Security.Cryptography.SHA256]::Create().ComputeHash($stream)) -replace '-' }
  finally { $stream.Dispose() }
  if ($expected -ne $actual) { Fail "checksum mismatch for $asset" } # -ne ignores case

  Add-Type -AssemblyName System.IO.Compression.FileSystem
  [System.IO.Compression.ZipFile]::ExtractToDirectory((Join-Path $tmp $asset), (Join-Path $tmp 'archive'))
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  Move-Item -Force (Join-Path $tmp 'archive\lazyfwd.exe') (Join-Path $dir 'lazyfwd.exe')
} finally {
  Remove-Item -Recurse -Force $tmp
}

$exe = Join-Path $dir 'lazyfwd.exe'
Write-Host "installed $(& $exe --version) to $exe"

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $env:LAZYFWD_NO_MODIFY_PATH -and (($userPath -split ';') -notcontains $dir)) {
  [Environment]::SetEnvironmentVariable('Path', (@($userPath, $dir) | Where-Object { $_ }) -join ';', 'User')
  $env:Path = "$env:Path;$dir"
  Write-Host "added $dir to your user PATH; restart other terminals to pick it up"
}
