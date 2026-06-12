#requires -Version 5.1
<#
  build-blockbook.ps1 — statically build blockbook.exe against the deps in ./.dev
  Run from the blockbook repo root:  .\build-blockbook.ps1

  Modes:
    .\build-blockbook.ps1          build blockbook.exe + blockbookgen.exe (default)
    .\build-blockbook.ps1 test     build, then run the unit-test suite
#>

[CmdletBinding()]
param(
    # Build mode. 'build' (default) just builds the binaries; 'test' builds and
    # then runs the Go unit tests (mirrors `make test` in build/docker/bin/Makefile).
    [Parameter(Position = 0)]
    [ValidateSet('build', 'test')]
    [string]$Mode = 'build'
)

$ErrorActionPreference = 'Stop'

# --- Toolchain (UCRT64 GCC) ---
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

# --- Locate deps relative to this repo (.\.dev) ---
# Anchor to the directory the script is *run from* (the repo root), not the script's
# own location ($PSScriptRoot would be build\scripts after the move). This must match
# where build-deps.ps1 created .dev, and lets `go build blockbook.go` resolve too.
$Repo    = (Get-Location).Path
$Deps    = Join-Path $Repo '.dev'
$Zmq     = Join-Path $Deps 'libzmq'
$RocksDb = Join-Path $Deps 'rocksdb'

# --- Sanity checks ---
if (-not (Test-Path $Gcc))                                 { throw "gcc not found at $Gcc. Install the UCRT64 toolchain." }
if (-not (Test-Path (Join-Path $RocksDb 'librocksdb.a')))  { throw "librocksdb.a missing. Run .\build-deps.ps1 first." }
if (-not (Test-Path (Join-Path $Zmq 'build\lib')))         { throw "libzmq build missing. Run .\build-deps.ps1 first." }

# --- Put UCRT64 bin first on PATH and set the CGO toolchain ---
$env:PATH        = "$Ucrt64\bin;$env:PATH"
$env:CGO_ENABLED = '1'
$env:CC          = $Gcc
$env:CXX         = $Gpp

# --- Include paths: rocksdb + libzmq headers.
#     -DZMQ_STATIC makes zmq.h declare plain symbols instead of __declspec(dllimport),
#     so the static libzmq.a resolves (otherwise you get undefined __imp_zmq_* refs). ---
$env:CGO_CFLAGS   = "-I$RocksDb\include -I$Zmq\include -DZMQ_STATIC"
$env:CGO_CXXFLAGS = "-I$RocksDb\include -I$Zmq\include -DZMQ_STATIC"

# --- Library search paths ---
$ZmqLib     = Join-Path $Zmq 'build\lib'
$RocksDbLib = $RocksDb

# --- Static linking. Order matters: rocksdb/zmq first, then their deps,
#     compression libs from UCRT64, then Windows system libs. ---
$env:CGO_LDFLAGS = @(
    "-L$RocksDbLib", "-L$ZmqLib", "-L$Ucrt64\lib",
    '-lrocksdb', '-lzmq',
    '-llz4', '-lzstd', '-lz', '-lsnappy', '-lbz2',
    '-lstdc++', '-lm', '-lpthread',
    '-lws2_32', '-liphlpapi', '-lshlwapi', '-lrpcrt4', '-lbcrypt', '-ldbghelp',
    '-static', '-static-libgcc', '-static-libstdc++'
) -join ' '

# --- Pin Go bindings that match the native libs ---
# Commented out: for testing purpose
# go get -u -v github.com/linxGnu/grocksdb@v1.10.8
# if ($LASTEXITCODE -ne 0) { throw 'go get grocksdb failed' }
# go get -u -v github.com/pebbe/zmq4@v1.2.11
# if ($LASTEXITCODE -ne 0) { throw 'go get zmq4 failed' }
# go mod tidy
# if ($LASTEXITCODE -ne 0) { throw 'go mod tidy failed' }

# --- Version stamping (mirrors build/docker/bin/Makefile LDFLAGS) ---
# Inject version/gitcommit/buildtime into github.com/trezor/blockbook/common.
# Version precedence: $env:VERSION override -> configs/environ.json "version" -> 'devel'.
$Version = $env:VERSION
if (-not $Version) {
    $EnvironJson = Join-Path $Repo 'configs\environ.json'
    if (Test-Path $EnvironJson) {
        try {
            $Version = (Get-Content -Raw $EnvironJson | ConvertFrom-Json).version
        } catch {
            Write-Warning "Could not parse $EnvironJson : $($_.Exception.Message)"
        }
    }
}
if (-not $Version) { $Version = 'devel' }
$GitCommit = if ($env:GITCOMMIT) {
    $env:GITCOMMIT
} else {
    (git describe --always --dirty 2>$null)
}
if (-not $GitCommit) { $GitCommit = 'unknown' }
$BuildTime = (Get-Date -Format 'yyyy-MM-ddTHH:mm:sszzz')

$LdFlags = @(
    "-X github.com/trezor/blockbook/common.version=$Version",
    "-X github.com/trezor/blockbook/common.gitcommit=$GitCommit",
    "-X github.com/trezor/blockbook/common.buildtime=$BuildTime"
) -join ' '

# grocksdb_no_link: we provide -lrocksdb ourselves via CGO_LDFLAGS above.
go build -v -tags grocksdb_no_link -ldflags="$LdFlags" -o blockbook.exe blockbook.go
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }

# --- Package-definition generator (pure Go, no CGO/rocksdb/zmq needed) ---
$env:CGO_ENABLED = '0'
go build -v -o blockbookgen.exe build/templates/generate.go
if ($LASTEXITCODE -ne 0) { throw 'go build blockbookgen failed' }

Write-Host ''
Write-Host '=== blockbook.exe + blockbookgen.exe built (static) ==='

# --- Unit tests (only when invoked as `build-blockbook.ps1 test`) ---
# Mirrors the `test` target in build/docker/bin/Makefile:
#   go test -tags 'unittest' <all packages except contrib/ and tests/>
# Reuses the CGO toolchain/env configured above so the grocksdb/zmq cgo
# packages compile and link against the static deps in .\.dev.
if ($Mode -eq 'test') {
    Write-Host ''
    Write-Host '=== Running unit tests (-tags "unittest grocksdb_no_link") ==='

    # grocksdb cgo needs CGO re-enabled (the blockbookgen build above turned it off).
    $env:CGO_ENABLED = '1'

    # Package list: everything except contrib/ and tests/ (integration-only).
    $Packages = go list ./... |
        Where-Object { $_ -notmatch '^github.com/trezor/blockbook/(contrib|tests)(/|$)' }
    if ($LASTEXITCODE -ne 0) { throw 'go list failed' }

    # grocksdb_no_link: same tag as the blockbook.exe build above. Without it,
    # grocksdb's default cgo LDFLAGS inject `-ldl`, which doesn't exist on
    # Windows/UCRT64 (libdl is Linux-only) and breaks the link of every
    # rocksdb-using test binary (api, db, fiat, fourbyte, server) with
    # "cannot find -ldl". The tag suppresses those defaults so the static
    # deps from $env:CGO_LDFLAGS (set above) are used instead. The repo
    # Makefile omits the tag only because -ldl resolves on its Linux image.
    go test -tags 'unittest grocksdb_no_link' @Packages
    if ($LASTEXITCODE -ne 0) { throw 'unit tests failed' }

    Write-Host ''
    Write-Host '=== Unit tests passed ==='
}
