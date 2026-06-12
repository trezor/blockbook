# Building Blockbook on Windows (static, from-source deps)

This guide builds Blockbook's native dependencies (**ZeroMQ / libzmq** and **RocksDB**,
plus RocksDB's compression libraries) **from source** as **static libraries** using the
MSYS2 **UCRT64** GCC toolchain.

Everything is statically linked: the deps are built as `.a` archives and the final
`blockbook.exe` is linked against them with no runtime DLL dependency on zmq/rocksdb.

## Layout

All dependencies are cloned and built **inside the blockbook checkout**, under
`./.dev`:

```
blockbook/                            # this repo
├── .dev/                             # created/managed by build/scripts/build-deps.ps1
│   ├── libzmq/                       # ZeroMQ source + build
│   └── rocksdb/                      # RocksDB source + build
├── build/scripts/build-deps.ps1      # builds the native deps
└── build/scripts/build-blockbook.ps1 # builds blockbook.exe
```

The static archives and headers end up at:

```
.dev/libzmq/build/lib/libzmq.a       (or libzmq-static.a)
.dev/libzmq/include/                 (zmq.h, zmq_utils.h)
.dev/rocksdb/librocksdb.a
.dev/rocksdb/include/                (rocksdb/*.h)
```

> Add `/.dev/` to your `.gitignore` so the cloned/built deps aren't committed.

---

## Prerequisites — install these BEFORE creating any script

### 1. git and Go (from their homepages)

- Git: https://git-scm.com/download/win
- Go:  https://go.dev/doc/install

### 2. MSYS2 + UCRT64 toolchain + CMake/Ninja

Install MSYS2: https://www.msys2.org/

Then open the **MSYS2 UCRT64** shell **once** and install the toolchain, CMake, Ninja,
and the compression libraries RocksDB needs. After this, you never need the MSYS2 shell
again — everything else runs from PowerShell.

```bash
pacman -Syu
pacman -S --needed \
  base-devel \
  mingw-w64-ucrt-x86_64-toolchain \
  mingw-w64-ucrt-x86_64-cmake \
  mingw-w64-ucrt-x86_64-ninja \
  git \
  mingw-w64-ucrt-x86_64-lz4 \
  mingw-w64-ucrt-x86_64-zstd \
  mingw-w64-ucrt-x86_64-zlib \
  mingw-w64-ucrt-x86_64-snappy \
  mingw-w64-ucrt-x86_64-bzip2
```

This gives you (`%MSYS2_ROOT%` = your MSYS2 install root):

- `%MSYS2_ROOT%\ucrt64\bin\gcc.exe`, `g++.exe`, `cmake.exe`, `ninja.exe`
- static `.a` archives for lz4 / zstd / zlib / snappy / bzip2 under
  `%MSYS2_ROOT%\ucrt64\lib`, with headers under `%MSYS2_ROOT%\ucrt64\include`

> Visual Studio / MSBuild and vcpkg are **not** required.

### Where the build scripts look for MSYS2

Both build scripts **auto-detect** the MSYS2 installation — no fixed install
path is required. Detection order:

1. a `gcc.exe` already on `PATH` that lives in a `ucrt64\bin` directory
2. the `MSYS2_ROOT` environment variable (set it to your MSYS2 root to
   override detection)
3. common install locations (default installer, Chocolatey, Scoop)
4. the MSYS2 uninstall entry in the Windows registry

---

## Building the dependencies — `build/scripts/build-deps.ps1`

The script lives in the repo at [`build/scripts/build-deps.ps1`](../build/scripts/build-deps.ps1).
This single PowerShell script:

1. Verifies the UCRT64 toolchain (`gcc.exe`, `g++.exe`) is installed.
2. Clears `./.dev` if it already exists, then recreates it.
3. Clones libzmq and rocksdb into `./.dev`.
4. Builds each as a static library with the UCRT64 GCC toolchain.

Run it from the repo root in PowerShell:

```powershell
.\build\scripts\build-deps.ps1
```

If PowerShell blocks the script (execution policy), run it once as:

```powershell
powershell -ExecutionPolicy Bypass -File .\build\scripts\build-deps.ps1
```

Result:

- `.dev\libzmq\build\lib\libzmq.a` (CMake may name it `libzmq-static.a` / `libzmq.a`
  depending on version), headers in `.dev\libzmq\include`
- `.dev\rocksdb\librocksdb.a`, headers in `.dev\rocksdb\include`

> **Why `-DCMAKE_POLICY_VERSION_MINIMUM=3.5`?** CMake 4.x removed compatibility with
> projects that declare `cmake_minimum_required(VERSION <3.5)`. The libzmq
> (and some of RocksDB's bundled CMake files) still do, so configuring fails with:
> *"Compatibility with CMake < 3.5 has been removed from CMake."* This flag tells CMake
> to assume a 3.5 policy baseline and configure anyway. It's already included in the
> script above.
>
> Note it is **quoted** (`"-DCMAKE_POLICY_VERSION_MINIMUM=3.5"`). Without quotes,
> PowerShell splits the token at the `.` and passes `...=3` plus a stray `.5`, giving
> *"Invalid CMAKE_POLICY_VERSION_MINIMUM value 3"*. Always quote `-D` args that contain
> a dot in their value.

> **Why `-DZMQ_HAVE_IPC=OFF -DZMQ_HAVE_STRUCT_SOCKADDR_UN=OFF`?** On MSYS2/mingw,
> libzmq detects `afunix.h` and turns on the IPC (Unix-domain socket) transport,
> which compiles `ipc_address.hpp` — and that header `#include`s the POSIX-only
> `<sys/socket.h>`, which doesn't exist in the mingw headers. The build then dies with
> *"sys/socket.h: No such file or directory"*. These two flags pre-seed libzmq's
> `check_include_files` cache so IPC is left off and `ipc_address.*` is never compiled.
> Blockbook only uses libzmq's **TCP** transport (it connects to the backend over
> `tcp://`), so dropping IPC has no effect on Blockbook.

> **RocksDB / grocksdb versions.** The Go binding **`grocksdb`** (pinned in
> `build-blockbook.ps1`) is written against the specific version of **RocksDB** C API
>
> RocksDB also builds cleanly under GCC 16: GCC 13+ (and the GCC 16 in current
> UCRT64) cleaned up its standard headers and no longer transitively pulls in `<cstdint>`.
> Old RocksDB (9.8.4) used `uint64_t` in headers without `#include <cstdint>` and failed
> with *"'uint64_t' has not been declared"*; 10.x/11.x added those includes, so no
> `-include cstdint` workaround is needed.

---

## Build Blockbook against the static deps — `build/scripts/build-blockbook.ps1`

The script lives in the repo at [`build/scripts/build-blockbook.ps1`](../build/scripts/build-blockbook.ps1).
It points CGO at the static archives under `.\.dev` and links everything statically.

Run it from the repo root in PowerShell:

```powershell
.\build\scripts\build-blockbook.ps1
```

If PowerShell blocks the script (execution policy), run it once as:

```powershell
powershell -ExecutionPolicy Bypass -File .\build\scripts\build-blockbook.ps1
```

### Notes on the link flags

- `-tags grocksdb_no_link` tells `grocksdb` **not** to add its own `-lrocksdb`; we
  supply the static archive path and link order ourselves via `CGO_LDFLAGS`.
- `-DZMQ_STATIC` (in `CGO_CFLAGS`/`CGO_CXXFLAGS`) is **required** for static libzmq. On
  Windows `zmq.h` declares its functions `__declspec(dllimport)` by default, so the
  zmq4 wrappers generate calls to `__imp_zmq_*` symbols that only exist in a libzmq
  **DLL**. `ZMQ_STATIC` switches the header to plain symbol declarations that resolve
  against the static `libzmq.a`. Without it the link fails with
  *"undefined reference to `__imp_zmq_strerror`"* (and every other `zmq_*`).
- The static library **link order** matters with GCC: consumers (`-lrocksdb`, `-lzmq`)
  come **before** the libraries they depend on (`-llz4 -lzstd -lz -lsnappy -lbz2`,
  then `-lstdc++ -lm -lpthread`, then the Windows system libs).
- `-static -static-libgcc -static-libstdc++` produce a fully static `blockbook.exe`
  with no dependency on `libgcc_s` / `libstdc++` / `libwinpthread` DLLs.
- If libzmq's archive is named `libzmq-static.a`, link it with the explicit path
  instead of `-lzmq`, e.g. add `%ZMQ%\build\lib\libzmq-static.a` to `CGO_LDFLAGS`
  (or rename/copy it to `libzmq.a` and keep `-lzmq`).
- `-lbcrypt` / `-ldbghelp` are sometimes required by recent RocksDB on Windows; keep
  them if linking complains about missing `BCrypt*` / `SymInitialize` symbols, drop
  them if unused.

## Verify it's statically linked

With `objdump` / `ldd` from `%MSYS2_ROOT%\ucrt64\bin` on PATH:

```bat
ldd blockbook.exe
```

It should list only Windows system DLLs (KERNEL32, ws2_32, ...) — **no** libzmq,
librocksdb, libstdc++, libgcc_s, or libwinpthread.

If `ldd` lists `libstdc++-6.dll`, `libgcc_s_seh-1.dll` or `libwinpthread-1.dll`, the
static CRT flags didn't take — re-check `-static -static-libgcc -static-libstdc++`
in `build/scripts/build-blockbook.ps1`.

## Running `blockbookgen.exe` (package-definition generator)

`blockbookgen.exe` runs from plain **PowerShell/cmd** with **no external dependencies**.
For Bitcoin-family coins the config template calls `generateRPCAuth` to produce the
`rpcauth=` line.

> **Note.** `generateRPCAuth` is implemented in pure Go (`build/tools/rpcauth.go`, a port
> of `build/scripts/rpcauth.py`) using only the standard library (`crypto/rand`,
> `crypto/hmac`, `crypto/sha256`, `encoding/base64`). It no longer shells out to a Python
> interpreter, so no Python install or Unix pipeline (`/usr/bin/env bash -c ... | sed`) is
> required — it works on Windows and Unix alike.

Then, from the **repo root**:

```powershell
.\blockbookgen.exe bitcoin                 # or: bitcoin_testnet4, bitcoin_signet, etc.
```

> Run it from the repo root — `generate.go` resolves `configs/` and `build/templates/`
> relative to the current directory. Output is written
> to `build/pkg-defs/`. Running `blockbookgen.exe` with no arguments prints the list of
> available coins.
