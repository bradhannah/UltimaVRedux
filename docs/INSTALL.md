# Installing and starting Ultima V Redux

> **Public preview.** Prerelease builds are on the [releases page](https://github.com/bradhannah/UltimaVRedux/releases),
> and `@main` installs the moving development line. See
> [platform status](#platform-status) for what has been verified.

## What you need

- Your own copy of the original **MS-DOS PC** release of Ultima V, such as the GOG
  edition. Copy the whole game folder: setup checks every required file, in
  upper or lower case, and the Amiga, Atari ST, Commodore 64 and Apple II
  versions use different files.
- A supported desktop platform with a working graphics and audio environment.
- Go 1.27 or newer, using the latest patch. The module declares `go 1.27.0` and selects
  `toolchain go1.27.1`; allow Go to obtain that toolchain or provision it yourself.
- Network access for Go to download the public module and its dependencies.

Go is needed only for building or installing from source. Prebuilt release
downloads do not require Go; see [downloaded binaries](#downloaded-binaries).

Supported targets, and what has been verified on each, are listed under
[platform status](#platform-status). The public module installation requires
neither Make, Node, an AI tool, a C compiler, nor a source checkout.

## Install Go

Use Go **1.27's latest patch or newer** for source installation. These steps install
the compiler; the game installation is a separate command below.

### Windows

1. Open the [official Go downloads](https://go.dev/dl/) and download the Windows
   **amd64 MSI installer** for Go 1.27 or newer.
2. Run the installer and follow its prompts.
3. Close and reopen PowerShell, then run `go version`.
4. Confirm the version is at least 1.27, then continue to
   [install the preview](#install-the-current-development-preview).

### macOS

1. Open the [official Go downloads](https://go.dev/dl/) and download the macOS
   **arm64 PKG installer** for Apple Silicon, using Go 1.27 or newer.
   Intel Macs require the amd64 installer, but Intel game acceptance is separate.
2. Open the package and follow its prompts.
3. Close and reopen Terminal, then run `go version`.
4. Confirm the version is at least 1.27, then continue to
   [install the preview](#install-the-current-development-preview).

### Linux (Debian/Ubuntu with apt)

Open a terminal and run:

```sh
sudo apt update
sudo apt install golang-go
go version
```

**Check the version before continuing.** Distribution packages can be older than
the game requires: for example, Ubuntu 24.04's
[`golang-go` package](https://packages.ubuntu.com/noble/golang-go) selects Go 1.22.
If yours is below 1.27, use the [official Linux installation instructions](https://go.dev/doc/install)
with a current Linux **amd64** or **arm64** archive matching your computer
(`uname -m` reports `x86_64` or `aarch64`, respectively). Those instructions explain
installing a fresh tree under `/usr/local/go`; do not extract over an older Go tree.
Put `/usr/local/go/bin` before the apt compiler on your PATH:

```sh
export PATH="/usr/local/go/bin:$PATH"
go version
```

Keep that PATH line in your shell startup file (usually `~/.bashrc` for Bash),
open a fresh terminal, and verify `go version` again. Then continue below.
The official [Go installation guide](https://go.dev/doc/install) covers all three
operating systems if the compiler is not found after installation.

## Downloaded binaries

Prerelease archives are on the [releases page](https://github.com/bradhannah/UltimaVRedux/releases).
Choose the archive matching your OS and architecture. Extract the whole archive; Go, Node,
Make, and a source checkout are not required. You still need your original game
files and a desktop graphics/audio runtime.

Check the archive against the release's `SHA256SUMS` file before extracting:
use `shasum -a 256 <archive>` on macOS, `sha256sum <archive>` on Linux, or
`Get-FileHash <archive> -Algorithm SHA256` in PowerShell, then compare the hash
for that exact filename. Checksums detect a damaged or different download; they
are not a code signature.

Open a terminal in the extracted folder and launch that copy explicitly:

```sh
./ultimav
```

Windows PowerShell:

```powershell
.\ultimav.exe
```

Keep `START-HERE.txt`, `release.json`, credits, and notices with the executable.
`release.json` identifies its version, source commit, and target. The initial
portable archives are unsigned: macOS Gatekeeper or Windows security checks may
require you to review the download before allowing it to open. Use the operating
system's per-app review controls only for the intended trusted download; do not
disable system-wide protection. These archives are command-line-launched binaries,
not signed installers or macOS app bundles. See [platform status](#platform-status).

## Build the local source

From an existing application checkout:

```sh
go build -o bin/ultimav ./cmd/ultimav
./bin/ultimav
```

For Windows, build with `-o bin/ultimav.exe` and launch `.\bin\ultimav.exe`.
Alternatively, `go run ./cmd/ultimav` builds and runs the local source. Dependency
and toolchain downloads need network access unless already cached.

Required embedded/generated assets are included in the public module. A
production build does not require a frontend rebuild. Bundled assets and fixtures
are not a complete original game installation and do not replace the external
files described below. Contributors changing the frontend or running validation
should use the separate [development guide](DEVELOPMENT_GUIDE.md).

## Install the current development preview

Install directly from `main`:

```sh
go install github.com/bradhannah/UltimaVRedux/cmd/ultimav@main
```

The command downloads and builds the game and places the executable in Go's
executable directory. It does not install or download original Ultima V data.
`main` is the moving development line; tagged builds are on the
[releases page](https://github.com/bradhannah/UltimaVRedux/releases).

## Start the game

First identify where Go installed the new executable:

```sh
go env GOBIN
go env GOPATH
```

Go installs executables into `GOBIN` when set; otherwise it uses `bin` under the
first `GOPATH` entry. With Go's default settings, launch directly as follows.

macOS or Linux:

```sh
"$HOME/go/bin/ultimav"
```

Windows PowerShell:

```powershell
& "$HOME\go\bin\ultimav.exe"
```

Use your actual executable directory if you customized Go's environment. For an
automatic lookup in macOS/Linux shells:

```sh
game_bin="$(go env GOBIN)"
if [ -z "$game_bin" ]; then
  game_path="$(go env GOPATH)"
  game_bin="${game_path%%:*}/bin"
fi
"$game_bin/ultimav"
```

Windows PowerShell:

```powershell
$gameBin = go env GOBIN
if ([string]::IsNullOrWhiteSpace($gameBin)) {
    $gameBin = Join-Path ((go env GOPATH) -split ';')[0] 'bin'
}
& (Join-Path $gameBin 'ultimav.exe')
```

Once the intended executable directory is first on `PATH`, `ultimav` is also
convenient. If it starts an unexpected program or an old version, inspect
`type -a ultimav` on macOS/Linux or `Get-Command ultimav -All` in PowerShell.
Compare that result with the Go destination above. A successful installation does
not replace an unrelated executable elsewhere on `PATH` or a shell alias/function.
Launch by explicit path to bypass that ambiguity. For default Go paths, prepend
`export PATH="$HOME/go/bin:$PATH"` after other PATH configuration in `~/.zshrc`
(zsh) or `~/.bashrc` (Bash), then open a new terminal. On Windows, adjust the user
Path through **Edit environment variables for your account**, then reopen
PowerShell and verify resolution; explicit paths work even if another executable
still takes precedence. Preserve older executables unless you intend to remove them.

The installed game must work from an unrelated directory; running from a source
checkout is not part of the player setup contract.

## Locate the original game data

### Use an existing installation

Fresh configuration leaves the data location unset. With an unset or unusable
data location, setup checks supported GOG/Heroic
installation records and known installation locations, plus the home-folder
route below. Confirm a verified result: even a single result needs your explicit
confirmation. When multiple valid copies are found, choose the intended one.

An already valid configured location is retained without discovery on every start,
including a historical `./OLD` value that still resolves to a valid installation.
Fresh discovery does not select the working directory or an implicit `./OLD`.
Discovery is not an exhaustive scan of your computer; a copy may exist elsewhere.

### Use the recommended personal folder

Setup shows a recommended folder directly under your resolved home directory:

```text
<home>/
  UltimaV/
    DATA.OVL
    ...the other required original game files...
```

Copy the original game files into the displayed folder, then choose **Check
Again**, then confirm the verified installation. Supplying only `DATA.OVL` is not
sufficient. Use the exact location shown by the game: `$HOME/UltimaV` on macOS or
Linux, or the displayed home-directory equivalent on Windows. This is not a
Documents-folder lookup.

This is the simple fallback; a verified installation elsewhere does not need to
be moved. Custom locations use the external configuration editor below. Setup has
no in-game folder-path field.

### Edit the configuration in your text editor

Choose **Open Settings File** to request your external editor on a best-effort
basis. Setup shows the exact configuration filename and
the data-path setting to edit; look for the existing `DataFilePath` setting, whose
serialized key may appear as `datafilepath`.

1. Replace the existing setting's value with the folder containing your original
   game files. Do not add a duplicate setting.
2. Use an absolute folder path, not the path to `DATA.OVL` itself.
3. Preserve other settings and save the file.
4. Return to the game and choose **Check Saved Settings**. It rereads the actual
   saved file and validates its selected installation.
5. During first-run setup, confirm the freshly checked location before continuing.
   From the title menu, follow the next-launch notice instead.

You can paste the path in your operating system's normal text editor. In-game
clipboard support is not required. If the editor cannot be launched, open the
displayed file yourself and use the same recheck action.

For YAML paths, single quotes avoid treating Windows backslashes as escape
sequences. Setup displays a quoted example. Malformed or unreadable settings are
reported and are not silently rewritten; correct them in the editor and recheck.

The existing setup contract distinguishes first-run recovery from changing a
working installation: first-run selection continues startup; changing the data
folder from the title menu (**Configure**, then **Set Data Directory**) takes
effect on the next launch. Follow the displayed
restart instructions.

## Import an original save

**Import from Legacy**, under **New Game** on the title menu, reads the exact
`SAVED.GAM`/`SAVED.OOL`
pair in the **active game-data directory**. It does not search other providers,
previous locations, or a pending data-folder change. If the pair is missing or
unusable, the recovery menu offers **Check Again** and **Back**. There is no
in-game legacy-path field.

After preparation, choose a native slot and its optional description, then
explicitly confirm any overwrite. The original pair is preserved. To select a
pair stored elsewhere or create a standalone native snapshot, follow the
[save converter procedure](details/SAVE_CONVERTER.md).

## Files and folders

The game keeps its settings and saves in a `.ultima_v_redux` folder in your home
directory: `~/.ultima_v_redux` on macOS and Linux, `%USERPROFILE%\.ultima_v_redux`
on Windows.

| Path | Contents |
| --- | --- |
| `config.yaml` | Settings, including the original-data location. |
| `saves/` | Save slots (`slot-01.json` to `slot-08.json`) and their `excerpts/` notebooks. Start the game with `--player-save-dir <folder>` to use another folder. |
| `crash.log`, `redux.log` | Crash details and recent warnings; attach both to a bug report. |

Back up `saves/` to keep your playthroughs. Saves made by `v0.1.0-preview.1` or
later load in current builds; saves from earlier development builds do not.

## Troubleshooting

### Window size and desktop scaling

Configuration → Display offers Windowed, Maximized Window, and Fullscreen.
Window sizes are image/output pixels, converted automatically for Windows desktop
scaling and macOS Retina displays. For example, 1920×1080 pixels at 150% scaling
uses 1280×720 logical window units. `About` marks fractional-scale pixel rounding;
manually resizing produces a Custom size. Saved window geometry remains logical,
so moving to another display can change its output-pixel dimensions.

Presets reserve desktop headroom. Use Maximized for the available desktop area,
or Fullscreen to fill the desktop output; a full-monitor client image cannot also
leave room for a title bar/taskbar. The game keeps a 16:9 shape with black bars
where needed. Rendering automatically follows display density with a 720p minimum
canvas and a 4K cap. Backing/output pixels on scaled macOS desktops do not always
correspond one-to-one to physical panel pixels.

Press Enter to apply, then Keep or Revert; unconfirmed display changes revert
after 12 seconds. Review the window size again after moving between differently
scaled displays. Report OS scaling, selected mode/size, and observed behavior for
display problems. Automated scale/layout checks do not establish native Windows
or mixed-display macOS acceptance; see [platform status](#platform-status).

### Common problems

| Problem | First step |
| --- | --- |
| `ultimav` is not found | Check `GOBIN`/`GOPATH` and use the executable's full path. |
| `ultimav` launches an old or unexpected program | Compare command resolution with `GOBIN`/`GOPATH`; launch the newly installed executable by its explicit path. |
| No original installation is detected | Use the displayed home/UltimaV folder or edit the configuration. |
| Files are missing or unusable | Confirm that the full original data set is present and readable; inspect the setup details. |
| A custom data path is needed | Edit it in the external settings file, then choose Check Saved Settings and confirm. |
| A configured folder stopped working | Use setup recovery to select or configure a valid folder. |
| Build or startup fails | Report the release, OS/architecture, Go version, command, and relevant error text. |

Before attaching diagnostics, remove personal paths or other information you do
not intend to publish. Do not upload original game assets. See
[reporting guidance](../CONTRIBUTING.md).

## Update

Repeat the same command to update to the current preview:

```sh
go install github.com/bradhannah/UltimaVRedux/cmd/ultimav@main
```

Saves keep loading across updates, as described in [files and folders](#files-and-folders).
To stay on a tagged build instead, download it from the releases page.

## Platform status

| Target | Status |
| --- | --- |
| macOS Apple Silicon (arm64) | Built and tested on macOS. |
| macOS Intel (amd64) | Built; not tested natively. |
| Windows x64 (amd64) | Built; not tested natively. |
| Linux x64 and ARM64 | Built; community-tested. |

On macOS arm64, anonymous installation of `ultimav@main` and `save-convert@main`
and a bounded startup were verified from a clean environment without a C
compiler. Compiling for a target does not establish its native graphics, audio,
input, setup or save/load behavior.

The game uses the original Avatar tile as its icon. Windows executables embed it
for Explorer, shortcuts and the taskbar; macOS shows it in the Dock while the game
runs, because the download is not an app bundle; Linux window managers that read
X11 window icons show it in their task lists. GNOME Shell instead takes icons from
installed desktop entries, which the portable archives do not provide, so it shows
a generic icon. The Windows icon has been verified in built executables, not yet
on a native Windows desktop.

Linux needs X11 (or XWayland), OpenGL and ALSA runtime libraries, which desktop
installations normally include. On Debian and Ubuntu they come from `libgl1
libx11-6 libxcursor1 libxext6 libxi6 libxinerama1 libxrandr2 libxrender1
libasound2`.
