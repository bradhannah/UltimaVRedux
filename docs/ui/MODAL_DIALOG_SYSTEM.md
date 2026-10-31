# Modal Dialog System

Choose and construct dialogs, then establish one input and cleanup owner.
`internal/ui/widgets/dialog_stack.go` is the stack implementation; concrete
components and their source entry points are in [Widget Components](WIDGET_COMPONENTS.md).

## Selection Guide

| Need | Use |
|---|---|
| Ordinary map/journal update | Main `text.Output`, with normal History; no popup or acknowledgement. |
| Ammunition threshold/depletion warning | Nonblocking `ShowActionFeedback` with a main-output copy. |
| Short feedback that must not block gameplay | `SystemCallbacks.UI.ShowActionFeedback` |
| Combat pickup result | Structured combat report entry; no transient map dialog. |
| Chest discovery / departure totals | `CombatLootDialog` / `NewCombatSummaryDialog`; see [combat loot feedback](GAME_UI_INTEGRATION.md#combat-loot-feedback). |
| Modal status with a scene-owned deadline | `ShowTimedOverlayMessage` |
| Acknowledged popup | `OverlayDialog`, `AsConfirmation`, `WithUserDismissed` |
| Staged prompt/response transcript | Workflow-owned `InteractionTranscript` |
| Mutable multiline narrative, optionally with a prompt | `MessageDialog` |
| Grammar, regex, yes/no, or numeric input | `InputBox` or its prefabs |
| Optional checkpoint description and native text entry | `SaveDescriptionEntryModal` |
| Saves, party, items, equipment, spells, or map selection | Existing domain dialog; see the component guide |
| Personal atlas note authoring / deletion | Scene-owned `AtlasNoteEditor` child inside the atlas pause; core validates the captured draft. |

## Construction

### OverlayDialog

`NewOverlayDialog(...).Build()` requires sizing, position, effects, vertical
content alignment, horizontal text alignment, and exactly one dismissal mode.
`AsConfirmation` and `AsOverlayFeedback` supply appearance only.

- `WithExternallyManagedDuration(ms)` makes a timed dialog; the widget never
  expires it, so its owner must expire and remove the instance. GameScene's
  timed-overlay installer uses it.
- `WithUserDismissed(keyboard)` accepts Enter/Escape and supplies the shared
  acknowledgement footer, regardless of appearance template.

```go
dialog := widgets.NewOverlayDialog("Quest updated!").
    AsConfirmation().
    WithUserDismissed(keyboard).
    AutoPopFromStack(&stack).
    Build()
dialog.ShowWithoutAnimation()
stack.PushModalDialog(dialog)
```

Here the dialog owns exact-instance removal. For cancellation-safe workflow
ownership, omit auto-pop and use the [workflow pattern](#event-workflow-pattern).
Never assign both cleanup owners. Timed overlays have no acknowledgement footer
and do not auto-remove merely by hiding.

### MessageDialog

Use `NewMessageDialog` for mutable styled content. Prompt methods require
`WithSeparatePromptArea` or `WithInputDismissal`; missing prompt capacity panics.
`WithInputDismissal` supplies the shared Close hint. A workflow accepting any key
instead reserves the prompt area, sets `DialogAnyKeyHint` when ready, and clears
it before hiding; that prompt does not transfer input ownership to the widget.

```go
dialog := widgets.NewMessageDialog("Setting up camp...").
    WithTextLines(6).
    WithCharsPerLine(40).
    WithSeparatePromptArea().
    WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
    WithVerticalPosition(widgets.DialogDefaultVerticalPos).
    Build()
dialog.AppendLineBuilder().White("Found: ").Yellow("50 gold").Done()
```

Always commit line builders with `Done`. Ordinary dissolve transitions use a
wall clock; transient feedback injects its scene clock before `Show` so movement
protection and reveal/hold/hide remain coordinated. Keep a hiding dialog updating
until its fade completes, then remove the exact instance. The camping and action
feedback prefabs have distinct owners; do not substitute an action-feedback
dialog for a modal confirmation.

Message layout retains frame rectangles and wrapped output without constructing
legacy border/background textures. Rendering draws its classic frame directly
into the dialog-owned composition buffer. Resize releases the old composition,
mask buffer, and dissolve resources before replacement. These full-size buffers
use standalone storage rather than sharing an atlas with persistent small glyphs.
`OverlayDialog.PrepareLayout` can resolve wrapping and bounds without drawing,
consuming input, advancing duration, or invoking callbacks.

### InputBox And Transcript

`InputBox` supports grammar commands or regex validation, not both. Its builder
validates required callbacks. `WithResultCallback` communicates a choice to the
owner; `NewYesNoInputBox` provides an established configuration. For example:

```go
dialog := widgets.NewInputBox("Name thy wish:", nil, keyboard).
    WithRegexValidation(`^[A-Za-z ]*$`).
    WithResultCallback(onResult).
    AutoCloseAndPop(&stack).
    Build()
stack.PushModalDialog(dialog)
```

`InteractionTranscript` captures raw responses and displays explicit Close or
Continue hints only while accepting input. Its workflow owns sequencing,
authoritative mutation, time, and cleanup. Keep hints separate from narrative
text and match the keys actually accepted.

### Popup Placement

Map-relative MessageDialog/OverlayDialog popups use active combat battlefield
bounds, excluding Turn Order. Explicit screen/numeric anchors bypass this.
Viewport changes rewrap a copy of original styled content,
preserving Runic/color segments without changing focus, timing, or ownership.
InputBox and large selection/workspace dialogs do not use this policy.

Action/Search feedback additionally opts into scene-owned full-content
measurement, translation, clipping, and a bounded stack. Do not reuse those
opt-in lifecycle seams for ordinary modal content.

## Stack Model

`DialogStack` stores private entries containing a `Widget` and a modal flag:

```go
type dialogEntry struct {
    widget  Widget
    isModal bool
}
```

Push APIs are explicit:

```go
stack.PushModalDialog(dialog)
stack.PushNonBlockingDialog(dialog)
```

Modal dialogs require exclusive user interaction. Non-blocking dialogs render and update without preventing gameplay input; transient action feedback uses this lane.

Pushing nil or the same widget instance twice panics. A stack entry is an ownership record, not merely a visibility flag.

Retain and remove exact instances instead of blindly removing whatever happens to be on top.

## Input Ownership

Outside the [Journal pause](../systems/TIME_SYSTEM.md#journal-pause),
`GameScene.Update` advances event queues and non-blocking dialogs before modal routing. It then calls `updateDialogs`, which updates only `PeekTopModalDialog`. If a modal exists, main gameplay input does not run that frame.

The effective ownership order is:

1. Event workflows advance and may own the global input block.
2. Non-blocking dialogs advance without consuming modal input.
3. The highest modal dialog receives one `Update` call.
4. If no modal exists and global input is not blocked, secondary then primary game input may run.

Lower modal dialogs remain in the stack and are drawn, but they do not receive `Update` until dialogs above them are removed.

Modal widgets that own platform text focus are notified when they become or
cease to be the highest modal. This private stack contract lets
`SaveDescriptionEntryModal` focus Ebitengine's native text field only while it owns
input; non-blocking dialogs do not disturb that focus. Failed validation retains
text and focus for correction. Ordinary native editing and IME composition remain
supported. Redux supplies no clipboard adapter, paste handler, or paste hint;
that does not assert that the operating system's text service cannot paste.
Data locations use external settings editing, and title-menu legacy import uses
the active data directory; neither has an in-game path-entry dialog.

`HasOpenDialog` includes non-blocking entries. Use `HasModalDialog` to answer whether the stack owns gameplay input.

## Drawing

`DialogStack.Draw` renders every entry from oldest to newest, producing bottom-to-top visual order. Modal and non-blocking entries share this draw order; modal status controls input, not z-order.

`Dialogs()` returns an allocated snapshot of widget references for diagnostics or exceptional iteration. Normal drawing uses `Draw` directly.

## Removal

Prefer pointer identity:

```go
if !stack.RemoveWidgetInstance(dialog) {
    // The owner may treat an already-removed transient as idempotent cleanup.
}
```

Use `MustRemoveWidgetInstance` when loss of stack ownership is a programming error. It panics if the instance is absent and is used by strict auto-pop contracts.

## Visibility Versus Ownership

`MessageDialog.Hide` may begin a fade while the dialog remains visible and must continue receiving updates. Remove it only after the fade interval or in guaranteed cleanup.

User-dismissed overlays can use `AutoPopFromStack`; workflow-owned overlays should be removed by the workflow's `Finally` callback.

Timed overlays use `WithExternallyManagedDuration`; an `OverlayDialog` never times itself out. `OverlayDialog.Update` does not advance those dialogs: GameScene installs one exact instance, tracks its deadline with `GameState.ElapsedMs`, and removes it even when another modal covers it. Replacement, expiry, and scene teardown share the same exact-instance cleanup.

Non-blocking action feedback has separate GameScene ownership. The owner retains stable exact instances, reconciles protected lifecycle and movement before the stack's one non-blocking update, and promptly retires an instance removed elsewhere. Capacity eviction, expiry, scene replacement, and disposal remove only owned feedback and tolerate idempotent cleanup.

Combat chest contents retire all scene-owned informational action-feedback cards
immediately before modal admission: Search, Look, Jimmy, failures and completed
actions alike, without fade overlap. Other workflow-owned choices/acknowledgements,
persistent history and the current trap notice inside contents survive. Do not
clear the whole stack or match text/type. Starting Open is not admission:
failed/empty attempts or earlier cancellation do not purge unrelated cards.
Finalization remains chest-scoped so newer feedback survives. Expired/evicted
handles are harmless. Open removes its chest before admission, so its exact
pending reveal bridges that interval; cancellation retires only those handles.
Removed owners without that pending snapshot (including empty Open results) retire
on reconciliation, even beneath a modal. Identical readmission cannot inherit them.
See [combat loot feedback](GAME_UI_INTEGRATION.md#combat-loot-feedback).

Maximum Action Popups bounds the shared action/Search collection to 0–3 cards,
default 3. Off admits no card; every affected result retains exactly one ordinary
output/history owner. Successful Apply reconciles a lower capacity on the next
scene update even beneath a modal or without a new message. Oldest exact instances
are removed immediately, including associations and stack ownership; raising the
limit cannot restore retired cards. This preference does not suppress workflow-owned
choices, acknowledgements or chest-content reveals.

Ordinary entry/exit notices use nonblocking action feedback and one output copy,
so their results remain readable with action popups Off. Required fades/map changes
still own input; fresh movement works afterward while the notice remains visible. Falls
likewise retain authoritative cascade ownership but no extra post-step dwell.
Other timed overlays and special acknowledgements keep their existing ownership.

## Event Workflow Pattern

```go
var dialog *widgets.OverlayDialog

manager.Workflow("acknowledgement").
    Priority(event_queue.PriorityMedium).
    Sequential().
    BlockInput().
    First(func() {
        dialog = widgets.NewOverlayDialog("Continue?").
            AsConfirmation().
            WithUserDismissed(keyboard).
            Build()
        dialog.ShowWithoutAnimation()
        stack.PushModalDialog(dialog)
    }).
    While(func() bool { return !dialog.IsDismissed() }, func() {}, 16).
    Finally(func() {
        if dialog != nil {
            stack.RemoveWidgetInstance(dialog)
        }
    }).
    Start()
```

Do not use `BlockInput` on feedback. Do not start an unrelated workflow and assume the parent waits for it; insert owned steps with `ThenSequence`.

## Input Debouncing

Dialog transitions must prevent the key that submitted one state from immediately dismissing the next. Use `Keyboard.PrepareForTransition`, advance release state with the keyboard acceptance methods, and accept only a fresh press. Linear conversation and event-queue helpers contain current examples.

The five [merchant quantity forms](WIDGET_COMPONENTS.md#merchant-quantity-controls)
also listen for top-modal focus changes. Cover/return/removal resets horizontal
repeat and suppresses held quantity and confirmation keys even when the covered
widget received no Update. Missing updates alone are not evidence of release.
The scene's shared keyboard suspension protects window-focus return. An Enter
pressed during an adjustment cannot become a delayed purchase after the arrow
releases; dismissal cannot leak that hold into gameplay or the next modal.

Atlas note editors keep the parent mounted and frozen. Only the top child accepts
text; successful commit or cancellation removes that exact child and release-gates
return to the atlas. Parent teardown removes the child first and discards its
single-use draft. Failed validation retains text and focus. See
[Vessels and personal notes](WIDGET_COMPONENTS.md#vessels-and-personal-notes).

Signs retain their own any-key acknowledgement and release gate. Their hidden
maintainer translation view is local to one reading and resets when the passive
Runic reference closes; its chord must be consumed before raw any-key polling.
It never becomes a second modal or an extra command Help entry. See
[Sign reading](../gamefacts/SIGNS_AND_FONTS.md#sign-0008).

## Configuration Transaction

`ConfigurationDialog` is a keyboard-only modal available from both the title
menu and the in-game Escape menu. It keeps detached original and staged copies
of Sound, Display, and Gameplay settings. The title menu's Configure sub-menu
opens it on Display, Sound or Gameplay; the Escape-menu route opens on Sound. Sound changes may preview through the
audio manager, but no persisted configuration model changes before Apply. Tab
and Shift+Tab change section in either direction; Up/Down changes the active row;
Left/Right changes stepped Gameplay values, Sound values or Display choices;
Space toggles a selected Gameplay checkbox. Numeric boundaries clamp without
wrapping and do not require entering a separate editor.
Gameplay includes **Flexible Sextant Use**, an off-by-default Redux option for
daytime and indoor surface-settlement readings. It requires a sextant and excludes
dungeons and the Underworld. It stages the existing
`Gameplay.Navigation.FlexibleSextantUse` preference with the same Apply/Cancel
transaction as the other settings. The eleven measured Gameplay rows use a
clipped scrolling viewport that keeps the selected row visible and reserves top
and bottom frame clearance plus a scrollbar gutter. Each checkbox or numeric
value has a separate measured column; label
continuations align with the label. Complete label buffers and measured detail
heights prevent leading-line loss. Row gaps preserve normal typography.
**Remember Hidden Dungeon Discoveries** retains its name and preference: On keeps
discovered hidden doors/pits between visits; Off resets them on a fresh visit.
The responsive card uses the
complete host pane, equal-width tabs, and independently measured Display label
and value columns.

The additional Gameplay preferences are:

| Preference | Persisted key | Allowed values | Default |
|---|---|---|---|
| Combat Speed | `Gameplay.Combat.SpeedFactor` | 0.5x, 1x, 1.5x, 2x, 3x, 4x, 6x, 8x, 12x, 16x, 20x | 1x |
| Confirm Quick Save | `Gameplay.Saves.ConfirmQuickSave` | On / Off | On |
| Show Combat HP Bars | `Gameplay.Combat.ShowHealthBars` | On / Off | On |
| Maximum Action Popups | `Gameplay.Presentation.MaxVisibleActionPopups` | integer 0–3; 0 is Off | 3 |

The details pane uses short player-facing sentences and blank paragraph rows,
appended as separate `Output` lines with natural wrapping. Measurement uses the
same populated Output as drawing, including blank rows, at the established font
size. Numeric choices, adjustments and defaults derive from the shared specs:
for example, "Choose 0.5x, 1x, 1.5x, 2x, 3x, 4x, 6x, 8x, 12x, 16x or 20x." and
"Choose Off, 1, 2 or 3." Speeds omit a trailing `.0`, and the value column is
measured from the widest formatted choice. UI copy uses
font-safe ASCII punctuation rather than an en dash or compact technical range
syntax. The current value remains separately labeled. Combat speed shortens animations and action pauses while
preserving combat rules; different elapsed timelines need not produce identical
seeded outcomes. Popup Off retains messages in the lower-right output. Quick-save
confirmation Off still reports success. HP-bars Off adds general monster injury
descriptions to numeric reports and retains fleeing indicators.

`NumericSettingSpec` owns numeric bounds, step and default for both controls and
persistence. An optional ascending `Values` list narrows the grid to listed
choices; Left/Right then moves one listed choice and clamps at either end. Runtime fields remain typed, including an integer popup count.
Defaults apply only to absent source keys: explicit false and zero are retained.
Invalid explicit speed/count values fail loading and saving before mutation. A
saved speed that lies on the half-step grid but is no longer listed, such as a
former 2.5x or 3.5x, loads as the nearest listed choice with ties upward (3x and
4x) and a load diagnostic; the next gameplay save writes the listed value.
`config.DefaultGameplayConfiguration` holds the only gameplay defaults: loading
starts from it and replaces each value the configuration sets, so tests that
construct detached models from it get the defaults players get.
Nil configuration or a missing Gameplay section gives consumers legacy defaults.

The new combat-speed key takes precedence over the legacy Debug movement key.
When only a valid legacy speed exists, it moves to the nearest listed choice,
clamped to the list; exact midpoints resolve upward (0.25→0.5, 0.75→1, 1.25→1.5,
2.5→3). Normalization is diagnosed, and the canonical key is written on the
next successful save. F2 detail persistence and full saves never write the old
speed authority. Immediate Workbench gameplay preference setters reject writes
while this dialog owns a staged transaction; Apply/Cancel releases that reservation.

Display offers Windowed, Maximized Window, and Borderless Fullscreen. Windowed
mode offers output-pixel presets (including 1600×900), quantized to the nearest
integer DIP window request, and preserves custom dragged sizes. An `About` prefix
marks a preset whose observed pixel size differs because of rounding. Ordinary
presets fit within 90% of the current monitor dimensions; this reserves portable
headroom, not a measured work area. Custom sizes can use more of the monitor.
Use Maximize for OS-managed available desktop space or Fullscreen for the desktop
output. The other modes show the observed final output only when actually active;
a staged mode says Automatic after Apply. Window dimensions remain saved in DIPs.

The Display section has two padded control rows, near-label value alignment,
focus-dependent wrapped help, a selected-row highlight, blank-line spacing,
and a distinct Apply/Cancel hint line. Aspect/resizing information is prose, not
an apparent interactive control. Layout measures its own content while keeping
the card stable across tabs. Native mode changes are reflected until a mode is
explicitly staged. A scale change clears staged size intent and asks the player
to review it; a scale/mode/size change during confirmation triggers rollback.

Explicit `Display.Mode`/`WindowWidth`/`WindowHeight` form a complete validated
tuple and take precedence over legacy `Resolution`/`FullScreen`. Invalid/partial
tuples fall back to defaults. Legacy indices 0–4 keep their old DIP sizes and
ordering; the player pixel-preset list is separate. The absolute saved-geometry
floor is 320×180 DIPs. Startup fits oversized geometry to conservative monitor
bounds; automatic modes retain normal geometry separately. Rendering is automatic
at current pixel density with a 720p minimum canvas and 4K cap, not an independent
quality preference.

Applying a changed display
selection first requests the asynchronous platform transition. Once observed,
the dialog requires Keep or Revert confirmation and automatically reverts after
12 seconds. Transition timeout, explicit Revert, Escape, or persistence failure
requests restoration of the prior mode and normal window size.
Revert retains an already fitting custom size even when it exceeds the preset
headroom; restoration is fitted only if desktop bounds or minimums now require it.

`Enter` persists all settings through `SaveConfigurationSettings` after any
display confirmation. Persistence writes a complete candidate configuration to
a same-directory temporary file and atomically replaces the active file before
updating the existing Audio, Display, and Gameplay model pointers. A failure
leaves the models and Viper state unchanged, keeps the dialog open, and offers
retry or cancel after display rollback.

`Escape` writes nothing, restores previewed volumes, stops owned preview
playback, and dismisses the exact Configuration instance. Apply performs the
same preview cleanup after persistence succeeds. When opened in-game, the
underlying Escape menu remains stack-owned, so game audio stays disabled until
that menu closes. When opened from the title, dismissal restores title-menu
audio and returns input ownership to the title menu.

The section, row selection and first visible row survive section changes and
responsive relayout; relayout scrolls only as needed to keep selection visible.
Each update accepts at most one bound key, and dismissal is guarded so cleanup
cannot remove the same instance twice.

## Invariants

- One concrete owner removes each dialog instance.
- Only the highest modal processes input.
- Non-blocking dialogs never imply a gameplay input block.
- A hidden dialog may still be stack-owned.
- Required stack and keyboard dependencies are validated during construction; do not add silent nil fallbacks around mandatory integration.
- Cleanup that must survive cancellation belongs in `Finally`.

See [Game UI Integration](GAME_UI_INTEGRATION.md) for scene wiring and
`internal/ui/widgets/dialog_stack_test.go`, `overlay_dialog_test.go`, and
`message_dialog_test.go` for focused lifecycle/constructor coverage. Scene
ownership tests live in `cmd/ultimav/gamescene_dialog_lifecycle_test.go`.
