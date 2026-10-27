# Convert an original Ultima V save

`save-convert` converts one original `SAVED.GAM`/`SAVED.OOL` pair into a native
Redux save without opening the game window. It leaves the originals unchanged.
This procedure owns command-line use; [Save Snapshot Schema](SAVE_SNAPSHOT_SCHEMA.md)
owns the native format and [the legacy crosswalk](LEGACY_TO_NATIVE_SAVE_CROSSWALK.md)
explains conversion dispositions and omissions.

## Before converting

- Use the converter from the same Redux version or checkout as the game that will
  load the result. See [installation prerequisites](../INSTALL.md#what-you-need).
- Supply a **complete original installation** with `--data-dir`, even if the save
  pair lives elsewhere. The converter loads real game references, hydrates the
  converted state, then recaptures a canonical snapshot and validates compatibility.
  A save pair alone is not enough.
- Supply `--source` as either the folder containing the pair or its `SAVED.GAM`
  file. `SAVED.OOL` must be the exact sibling in that folder; there is no search
  of other installations or fallback pair.
- Choose an explicit output outside both the source-save and original-data trees.
  Use a separate scratch destination when trying a conversion for the first time.
- The converter does not inherit your HOME configuration or launch any UI.
  Shell-expanded paths such as `"$HOME/UltimaV"` work; a quoted literal `~` is not
  expanded by the converter. Substitute your real paths in the examples.

Install the converter from the public development preview and inspect its flags:

```sh
go install github.com/bradhannah/UltimaVRedux/cmd/save-convert@main
save-convert --help
```

Repeat the installation command to update alongside the game. Anonymous macOS
installation and installed help have been verified; see
[platform status](../INSTALL.md#platform-status) for scope and
platform limits. Go places this executable in the same `GOBIN`/`GOPATH` location
described in [Start the game](../INSTALL.md#start-the-game); use its full path if needed.

From an existing application checkout, use `go run ./cmd/save-convert --help`
to inspect the local version. You can also build a local executable with
`go build -o bin/save-convert ./cmd/save-convert` and use `./bin/save-convert`
in place of `save-convert` below. On Windows, build with
`-o bin/save-convert.exe` and launch `.\bin\save-convert.exe`.
The examples below use macOS/Linux shell syntax; in PowerShell, supply
the same flags with your actual paths, using `$HOME` for the home directory.

## Choose exactly one output mode

### Standalone snapshot

The output's parent directory must already exist. For example, first create a
separate directory, then write a named snapshot:

```sh
mkdir -p "$HOME/UltimaV-converted-snapshots"
save-convert \
  --data-dir "$HOME/UltimaV" \
  --source "/path/to/original-save/SAVED.GAM" \
  --output "$HOME/UltimaV-converted-snapshots/imported-adventure.json" \
  --description "Imported adventure"
```

`--output` must use the lowercase `.json` extension, not `config.json` (in any
letter case). Names beginning with `slot-` (case-insensitive) are reserved: use
the player-slot mode below instead. For Workbench loading, use a lowercase slug
filename such as `imported-adventure.json`: lowercase
letters/digits with optional interior hyphens, starting and ending with a letter
or digit. An arbitrary converter-accepted filename is not necessarily a valid
Workbench ID.

### Player slot

Use both `--save-dir` and `--slot`. The root is explicit; it is not inferred from
your normal game configuration. The slot must be `slot-01` through `slot-08`.

```sh
save-convert \
  --data-dir "$HOME/UltimaV" \
  --source "/path/to/original-save" \
  --save-dir "$HOME/UltimaV-converted-slots" \
  --slot slot-07 \
  --description "Imported adventure"
```

The final save-directory component may be absent and will be created; its parent
must already exist. The root cannot itself be a symlink or file. In addition to
remaining outside both input trees, the save root cannot be an ancestor of either
input directory. Player slots use the ordinary repository's validation, locking,
revision checks, and private storage permissions.

Do not combine `--output` with `--save-dir` or `--slot`. There are no positional
arguments. `--description` is optional (blank is valid), single-line, and limited
to 80 Unicode runes; invalid descriptions are rejected rather than truncated.

## Existing destinations and failures

Both modes refuse to replace an existing destination unless you explicitly add
`--overwrite`. Inspect the exact file or slot first and preserve any checkpoint
you need. The flag authorizes replacement only of the destination revision
inspected before conversion; a change during conversion is refused and requires
fresh inspection. Symlink and nonregular destinations are rejected.

Conversion warnings go to **stderr** with their field path, code, and message.
Read every warning: a completed conversion can still omit or normalize legacy
state as described in the crosswalk. Success prints a receipt to **stdout**:
`Converted save: "<path>"`, the new `Playthrough ID`, the `Format` with envelope
and snapshot versions, and the `Warnings` count. Keep that receipt with the
warnings when reviewing the result. A nonzero exit means the command did not
complete successfully.

An error does not always mean no output was written. Directory synchronization,
slot persistence, or success-reporting failures can occur after publication.
If the error says to inspect the destination, do so before retrying: a complete
new file may already exist. Do not blindly retry with `--overwrite` or assume the
previous slot is still present. A reported successful conversion establishes
conversion and persistence, not an interactive gameplay load test.

## Load the result

### Load a player slot in the game

Select the same save root when starting the game:

```sh
ultimav --player-save-dir "$HOME/UltimaV-converted-slots"
```

Use **Journey Onward** on the title menu or **Load** during gameplay and select
the converted slot. Complete ordinary original-data setup if necessary. The game
still needs the compatible original installation; the slot does not bundle it.

### Load a standalone snapshot in the Debug Workbench

Keep the lowercase-slug `.json` file directly in the selected personal snapshot
directory and start:

```sh
ultimav --debug-workbench \
  --snapshot-personal-dir "$HOME/UltimaV-converted-snapshots"
```

1. Start or load ordinary gameplay and reach a stable, save-eligible boundary.
   Workbench loading first captures an automatic rollback of the current game;
   the title screen or an unsupported transient state cannot supply that rollback.
2. Open `http://127.0.0.1:8585` and navigate to **Saves**.
3. Choose **Refresh Library** and select the entry with ID `imported-adventure`
   under **Personal / Local** (its display name is **Imported original save**).
   Its inspected reference is `personal:imported-adventure`. Inspect its
   metadata and compatibility, and review the earlier CLI conversion warnings.
   Choose **Review & Load**, then **Capture Rollback & Load**. Loading is
   conditional on the reviewed revision and successful rollback persistence.
4. Check the restored party, location, and subsequent gameplay. Distinguish this
   load test from the earlier conversion result when reporting a problem.

The converter does not add a native-save startup flag. See
[Workbench snapshots](../systems/DEBUG_WORKBENCH.md#save-snapshots) for repository
and rollback behavior. No original binary export is provided, and conversion does
not change the native schema version.

## Title-menu import instead

For a pair already in the active game-data directory, use **Import from Legacy**
under **New Game** on the title menu. It reads only that active directory, then
offers native slot,
description, and overwrite review. Missing-pair recovery offers **Check Again**
and **Back**. A pending data-folder change takes effect next launch, so it does
not redirect an import in the current session. Use this converter when an
explicit separate source pair or standalone output is needed.
