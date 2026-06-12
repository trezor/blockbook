#requires -Version 5.1
<#
  build-deps.ps1 — clone & statically build blockbook native deps into ./.dev
  Run from the blockbook repo root:  .\build-deps.ps1
#>

$ErrorActionPreference = 'Stop'

# --- Versions (keep in sync with go.mod) ---
# The grocksdb targets the specific version of RocksDB
# C API ; pebbe/zmq4 works with libzmq 4.3.x.
# Use the matched RocksDB tag so the grocksdb CGO wrappers link cleanly.
$RocksDbTag = 'v11.1.1'
$LibZmqTag  = 'v4.3.5'

# --- Toolchain locations ---
# Auto-detected instead of hardcoding C:\msys64: gcc already on PATH ->
# $env:MSYS2_ROOT hint -> common install locations (default installer,
# Chocolatey, Scoop) -> registry uninstall entries. Set MSYS2_ROOT to your
# MSYS2 root (e.g. D:\msys64) to override — e.g. on GitHub-hosted runners the
# msys2/setup-msys2 action reports its install root via its 'msys2-location'
# output, which the workflow exports as MSYS2_ROOT.
function Find-Ucrt64Gcc {
    <#
        Locate an MSYS2 UCRT64 gcc and return the directory containing gcc.exe,
        or $null if none is found. Returns '' (empty string) if a suitable gcc
        is already available on PATH and no changes are needed.
    #>

    # 1. Already configured? gcc on PATH that lives in a ucrt64\bin directory.
    $existing = Get-Command gcc.exe -ErrorAction SilentlyContinue
    if ($existing) {
        $dir = Split-Path -Parent $existing.Source
        if ($dir -match '\\ucrt64\\bin\\?$') {
            return ''  # already usable as-is
        }
    }

    $candidates = @()

    # 2. Explicit hint via environment variable.
    if ($env:MSYS2_ROOT) {
        $candidates += (Join-Path $env:MSYS2_ROOT 'ucrt64\bin\gcc.exe')
    }

    # 3. Common install locations.
    $candidates += @(
        'C:\msys64\ucrt64\bin\gcc.exe',                                      # default installer
        'C:\tools\msys64\ucrt64\bin\gcc.exe',                                # Chocolatey
        (Join-Path $env:USERPROFILE 'scoop\apps\msys2\current\ucrt64\bin\gcc.exe'), # Scoop
        'D:\msys64\ucrt64\bin\gcc.exe'
    )

    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path $candidate)) {
            return (Split-Path -Parent $candidate)
        }
    }

    # 4. Registry fallback: look for an MSYS2 uninstall entry with a location.
    $uninstallKeys = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    foreach ($keyGlob in $uninstallKeys) {
        $entries = Get-ItemProperty -Path $keyGlob -ErrorAction SilentlyContinue |
            Where-Object { $_.DisplayName -like 'MSYS2*' }
        foreach ($entry in $entries) {
            $root = $entry.InstallLocation
            if (-not $root -and $entry.UninstallString) {
                # UninstallString is typically "C:\msys64\uninstall.exe" or similar.
                $root = Split-Path -Parent ($entry.UninstallString.Trim('"'))
            }
            if ($root) {
                $gccPath = Join-Path $root 'ucrt64\bin\gcc.exe'
                if (Test-Path $gccPath) {
                    return (Split-Path -Parent $gccPath)
                }
            }
        }
    }

    return $null
}

$GccBin = Find-Ucrt64Gcc
if ($null -eq $GccBin) {
    throw 'MSYS2 UCRT64 gcc not found. Install MSYS2 + the UCRT64 toolchain, or set MSYS2_ROOT to your MSYS2 root.'
}
if ($GccBin -eq '') {
    # gcc already on PATH in a ucrt64\bin directory — derive the prefix from it.
    $GccBin = Split-Path -Parent (Get-Command gcc.exe).Source
}
$Ucrt64 = Split-Path -Parent $GccBin   # ...\ucrt64
$Gcc    = Join-Path $GccBin 'gcc.exe'
$Gpp    = Join-Path $GccBin 'g++.exe'
$Cmake  = Join-Path $GccBin 'cmake.exe'

# --- Paths ---
# Anchor to the directory the script is *run from* (the repo root), not the script's
# own location ($PSScriptRoot would be build\scripts after the move), so .dev is
# created in the repo root.
$Root = (Get-Location).Path
$Dev  = Join-Path $Root '.dev'

function Assert-Tool($path, $name) {
    if (-not (Test-Path $path)) {
        throw "$name not found at $path. Install the MSYS2 UCRT64 toolchain first (see Prerequisites)."
    }
    Write-Host "  [ok] $name -> $path"
}

Write-Host '== Checking UCRT64 toolchain =='
Write-Host "  [ok] UCRT64 -> $Ucrt64"
Assert-Tool $Gcc   'gcc'
Assert-Tool $Gpp   'g++'
Assert-Tool $Cmake 'cmake'

# Put UCRT64 bin first on PATH for this process (gcc, g++, cmake, ninja).
$env:PATH = "$Ucrt64\bin;$env:PATH"
$env:CC   = $Gcc
$env:CXX  = $Gpp

Write-Host ''
Write-Host '== Resetting .dev directory =='
if (Test-Path $Dev) {
    Write-Host "  removing existing $Dev"
    Remove-Item -Recurse -Force $Dev
}
New-Item -ItemType Directory -Path $Dev | Out-Null
Write-Host "  created $Dev"

# ---------------------------------------------------------------------------
# Clone
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '== Cloning sources =='
$ZmqSrc     = Join-Path $Dev 'libzmq'
$RocksDbSrc = Join-Path $Dev 'rocksdb'

git clone --depth 1 --branch $LibZmqTag  https://github.com/zeromq/libzmq.git   $ZmqSrc
git clone --depth 1 --branch $RocksDbTag https://github.com/facebook/rocksdb.git $RocksDbSrc

# ---------------------------------------------------------------------------
# Build libzmq (static)
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '== Building libzmq (static) =='
$ZmqBuild = Join-Path $ZmqSrc 'build'

& $Cmake -S $ZmqSrc -B $ZmqBuild -G Ninja `
    -DCMAKE_BUILD_TYPE=Release `
    "-DCMAKE_POLICY_VERSION_MINIMUM=3.5" `
    -DCMAKE_C_COMPILER="$Ucrt64/bin/gcc.exe" `
    -DCMAKE_CXX_COMPILER="$Ucrt64/bin/g++.exe" `
    -DBUILD_SHARED=OFF `
    -DBUILD_STATIC=ON `
    -DBUILD_TESTS=OFF `
    -DWITH_DOCS=OFF `
    -DENABLE_DRAFTS=OFF `
    -DZMQ_BUILD_TESTS=OFF `
    -DZMQ_HAVE_IPC=OFF `
    -DZMQ_HAVE_STRUCT_SOCKADDR_UN=OFF
if ($LASTEXITCODE -ne 0) { throw 'libzmq cmake configure failed' }

& $Cmake --build $ZmqBuild --config Release
if ($LASTEXITCODE -ne 0) { throw 'libzmq build failed' }

# ---------------------------------------------------------------------------
# Build RocksDB (static, with compression libs)
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '== Building RocksDB (static) =='
$RocksDbBuild = Join-Path $RocksDbSrc 'build'

& $Cmake -S $RocksDbSrc -B $RocksDbBuild -G Ninja `
    -DCMAKE_BUILD_TYPE=Release `
    "-DCMAKE_POLICY_VERSION_MINIMUM=3.5" `
    -DCMAKE_C_COMPILER="$Ucrt64/bin/gcc.exe" `
    -DCMAKE_CXX_COMPILER="$Ucrt64/bin/g++.exe" `
    -DROCKSDB_BUILD_SHARED=OFF `
    -DWITH_GFLAGS=OFF `
    -DWITH_TESTS=OFF `
    -DWITH_BENCHMARK_TOOLS=OFF `
    -DWITH_TOOLS=OFF `
    -DWITH_CORE_TOOLS=OFF `
    -DWITH_LZ4=ON `
    -DWITH_ZSTD=ON `
    -DWITH_ZLIB=ON `
    -DWITH_SNAPPY=ON `
    -DWITH_BZ2=ON `
    -DPORTABLE=ON `
    -DFAIL_ON_WARNINGS=OFF
if ($LASTEXITCODE -ne 0) { throw 'rocksdb cmake configure failed' }

& $Cmake --build $RocksDbBuild --config Release --target rocksdb
if ($LASTEXITCODE -ne 0) { throw 'rocksdb build failed' }

# Copy the archive next to include/ so CGO paths are simple.
Copy-Item (Join-Path $RocksDbBuild 'librocksdb.a') (Join-Path $RocksDbSrc 'librocksdb.a') -Force

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '== Done =='
Write-Host "  libzmq lib : $ZmqBuild\lib"
Get-ChildItem (Join-Path $ZmqBuild 'lib') -Filter *.a | ForEach-Object { Write-Host "    $($_.Name)" }
Write-Host "  libzmq inc : $ZmqSrc\include"
Write-Host "  rocksdb lib: $RocksDbSrc\librocksdb.a"
Write-Host "  rocksdb inc: $RocksDbSrc\include"