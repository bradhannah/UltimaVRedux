# UI Widget System Architecture

Explain UI package boundaries and shared lifecycle. Use [Widget Components](WIDGET_COMPONENTS.md)
to choose an existing component and find its integration contracts. Construction,
stack/input ownership, and cleanup are covered by [Modal Dialog System](MODAL_DIALOG_SYSTEM.md).

## Package Boundary

Reusable Ebitengine presentation components live in `internal/ui/widgets`. They may depend on rendering, display, text, and input packages. Core gameplay packages must not import them; gameplay requests UI through `SystemCallbacks` and presentation adapters in `cmd/ultimav`.

`GameScene` owns the principal `DialogStack`, main `text.Output`, keyboard, fixed screen panels, and feature-specific presentation components. It creates concrete widgets in response to gameplay callbacks.

Player-facing Ebitengine widgets are keyboard-only and must not read mouse,
wheel, or touch input. The browser-based Debug Workbench is separate developer
tooling and is explicitly exempt from this package rule.

## Core Interfaces

```go
type Widget interface {
    Draw(screen *ebiten.Image)
    Update()
}

type DismissibleDialog interface {
    Widget
    Show()
    ShowWithoutAnimation()
    Hide()
    HideWithoutAnimation()
    IsVisible() bool
    IsDismissed() bool
}
```

`Widget` is the stack contract. There is no shared result-bearing dialog contract; each dialog reports its result through its own callback or polling API, and `DialogStack` never polls one. `DismissibleDialog` is implemented by `MessageDialog` and `OverlayDialog` for shared visibility control.

Not every utility in the package is a `Widget`. `ScrollableListState` has a purpose-specific state API.

## Layout And Display

Most widgets use `sprites.PercentBasedPlacement`. The canonical display manager provides the current logical render dimensions through `GetCurrentSize`; widgets must not use its window, monitor, or container dimensions for layout. The game loop, presentation widgets, and resolution callbacks share the `display.GetManager()` identity. Configuration contains plain values and an instance-owned preference source, not the display manager; scene adapters apply display requests. Fonts use the shared scaling helpers in `internal/text` and `internal/ui/widgets/typography.go`; production spacing constants live in `dialog_constants.go`.

Text uses four roles from `typography.go`, sized for a 1920-wide screen and scaled to the window: `FontSizeTitle` (26) for the one name at the top of each window; `FontSizeBody` (23) for all readable dialog text, including prompts, conversation, footers and section labels in the heading colour; `FontSizeCompact` (20) only for dense tables measured not to fit at body size, such as Ready's gear, party-picker rows and Dungeon View labels; and `FontSizeHUD` (18) for fixed main-screen indicators. A warning may borrow the title size, as the provisions strip's food-loss heading does. Any other size is a named, measured local exception, listed below.

### Text-size exceptions

| Where | Size | Why |
| --- | --- | --- |
| Dungeon View axis tags | 16 | Four-character tags must fit eight adjacent columns. |
| Provisions strip `FOOD -N` heading | Title (26) | A warning borrowing the title size. |
| Dungeon debug coordinate overlay | 12 | Not player-facing. |
| Dungeon View size floor | 10, line space 11 | A lower bound that supported window sizes never reach. |
| Guard card | Role size, at least 8, stepping down a point at a time to 6 | Steps down until every row and wrapped line fits the card. |
| Journal moonstone rows and heading | Body (rows) and Title (heading), stepping down a pixel at a time | Shrinks until every phase line fits its pane. |
| Party status label in combat | Scaled down per label | Long labels such as `Possessed - 8/12 turns` are fitted to the status cell. |
| Character creation prompt | 10 on a 320x200 canvas | The canvas is then scaled to the window; the bitmap identity text is 8 on that canvas. |
| Title-screen menu | Fraction of screen height | Choices are about 0.0273 x height, the notice line about 0.0239 x height, sub-menu rows 0.9 x the choice size and the `Esc Back` hint 0.65 x. |
| Title-screen version line | About 0.012 x width | 0.8 x (6 x width / 400), to sit inside the frame's blue band. |
| Game-file setup screens | Title-screen choice size | Heading, body and footer all use it. |
| Runic, IBM and sky bitmap glyphs | Line spacing / 8 | They scale with the line spacing of the text beside them, not its font size, so runes are a little larger than the Latin text and the factor differs by window. The sky strip uses its band height / 8; sign text uses a whole-number scale. |
| Banner, finale and dungeon graffiti bitmap text | 8 on a fixed canvas | Fixed bitmap size, not a role. |
| Cast and Mix headers | Title (26) second line | The window draws a second Title-size line under its title: Cast shows the caster's `Level / MP`, Mix shows `Choose a spell to mix` or the result. An exception to one title per window. |

The game font is an 8x8-cell pixel font and text scales by window width / 1920 with no rounding, so a role lands on whole pixels only when its scaled size is a multiple of 8. At the supported window sizes (1280, 1366, 1920, 2560 and 3840 wide) Title 26 and Body 23 never do, Compact 20 does only at 3840 and HUD 18 only at 2560. This is accepted.

Line spacing is chosen per window, not by role. Values in use: 1.0 plain HUD and lists; 1.05 arms and healer shops; 1.06 inn, tavern and ship; 1.08 reagent, guild and horse; about 1.09 the message window; 1.1 party picker and Dungeon View; 1.15 merchant shell, item pickers, Journal and Map; 1.2 most dialogs such as Z-stats, Cast, Mix, Save/Load and Help; 1.3 Talk and credits; about 1.59 main-screen summaries. There is no single standard yet.

Percent placement is the dominant convention, but cached pixel geometry and scaled text resources still require invalidation. There is no shared flex/grid layout or resolution-scaler API. Active responsive dialogs use dimension-remembering layout at construction and update/draw boundaries. A size change rebuilds only derived borders, fonts, outputs, and viewport capacity while preserving selection, workflow, input, pending-result, visibility, and dismissal state. `MessageDialog` refreshes those resources automatically during draw; `Refresh` remains available for explicit reinitialization.

High-DPI conversion is already applied to the published render canvas: do not multiply
widget coordinates or fonts by device scale a second time. OS window DIPs,
canvas pixels, and final-output pixels are distinct. There is no independent
player UI-scale setting.

`Border` captures logical dimensions and placement during construction, but creates
its background and shared frame only when their draw methods are used. Bounds and
interior measurement do not allocate textures. Deferred drawing retains those
captured dimensions rather than consulting a later display size. `Dispose` releases
the border's private background; it must not release the shared cached frame.

The shared frame cache retains at most 128 entries and 32 MiB of image pixel
payload including the pinned engine's minimum and power-of-two texture rounding.
Eviction drops the cache reference without deallocating artwork still
borrowed by a scene. This is a cache-held payload budget, not a native-memory cap.
Composed frames and cached text use standalone storage so small persistent images
do not pin oversized atlases grown by transient render targets.

Transient dialogs use opaque black classic cards with white `DialogRuleColor` frames. The legacy blue/white `Border` sprite remains a thematic-chrome primitive and must not be recolored globally; external code may still use `NewBorder` or `NewOpaqueBorder` where that chrome is intentional.

For map-relative message popups, `Game` scopes a presentation-only viewport
provider on the display manager during activation, Update, and Draw. The
provider resolves the current scene each time; `GameScene` supplies combat
battlefield bounds, excluding Turn Order, or no override outside combat. It is
restored on exit, so dormant scene construction and failed/replaced scenes do
not leave stale placement behind. Widgets receive geometry, not gameplay state.
`MessageDialog` retains its map/screen anchor intent and original layout/content;
`OverlayDialog` inherits the same policy through its wrapped message dialog.
See [Popup Placement](MODAL_DIALOG_SYSTEM.md#popup-placement)
for sizing, explicit overrides, and excluded widget families.

## Text Rendering

All widget text is rendered through `text.Output`. Fresh panel content follows this pattern:

```go
output.Reset()
output.AppendLine().Yellow("Label: ").White(value).Done()
output.DrawPanelContent(screen, point, width, etext.AlignStart)
```

Scrolling regions use `DrawScrollingBuffer` or `DrawScrollingBufferWithHeight`.
The unused direct `DrawText`, `DrawTextCenter`, and `DrawTextRightToLeft`
compatibility methods are retired; widgets use Output rather than bypassing its
layout and semantic text boundary.

Builders returned by `AppendLine` require `Done()` to commit the line. Panel code uses `Reset`; circular scrolling output normally keeps its existing index.

Keyboard hints use one grammar, `[Key] Action`, with three spaces between hints
and the spellings `Esc`, `Enter`, `Space`, `Tab`, `Shift+X`, `Ctrl+X`, `Up/Down`,
`Left/Right`, `PgUp/PgDn` and `Home/End`. Typed hints are `ControlHint` values;
`PackControlHints` colours keys with `DialogControlKeyColor` and actions with
`DialogControlHintColor`, wraps only between complete hints, and tightens the
separator only when that saves a row. `AppendControlHints` and
`ParseControlHints` read the same grammar from guidance strings; text outside
brackets stays prose. Where Space is otherwise free, Space pages detail panes.
Selected rows use the white `DialogSelectedBackground` bar (`fillSelectedRow`)
with `DialogSelectedText`; unchoosable Use, Ready and spell rows are dimmed with
`DialogUnavailableItemColor` and carry the `[No]` marker.

## Construction And Invariants

Create exported components through their `New...` constructors. Builders collect optional configuration and perform contract checks in `Build`.

Required dependencies are not soft-optional merely because they are pointers. Current constructors such as `NewCharacterSelectionDialog`, `NewReadyEquipmentDialog`, `MessageDialogBuilder.Build`, and `OverlayDialogBuilder.Build` panic on invalid mandatory wiring. `DialogStack` also panics when asked to push nil or the same instance twice.

Some older general-purpose controls still normalize invalid optional configuration and log warnings. Do not copy that behavior into new mandatory system wiring.

## Composition

Common composition patterns are:

- a border plus one or more `text.Output` regions for static panels
- a `TextInput` inside `InputBox` for validated user entry
- `ScrollableListState` for list selection and viewport offsets
- specialized dialogs for party, inventory, spells, reagents, equipment, astronomy, signs, and map views
- `OverlayDialog` over `MessageDialog` for explicitly configured popup messages
- `InteractionTranscript` for compact staged prompts and retained response history

Dialogs communicate results through callbacks or polled result fields. Authoritative gameplay mutation remains in `GameState` or a workflow callback, not in rendering methods.

## Timing

Widget timing is not uniform:

- `MessageDialog` dissolve transitions use wall-clock milliseconds by default; transient action feedback injects a scene-owned clock before `Show`.
- `OverlayDialog` has no timer of its own: `WithExternallyManagedDuration` leaves expiry to an owner such as `GameScene`, which uses one authoritative elapsed-time deadline and exact stack removal.
- `GameScene` transient action feedback uses `GameState.ElapsedMs` for movement, protected reveal/hold/hide progression, and exact expiration.

Do not infer deterministic timing from the common `Widget.Update` interface.

## Testing

Widget tests live in `internal/ui/widgets/*_test.go`. Prefer state and layout assertions over brittle screenshots. GameScene ownership and input-routing tests live in `cmd/ultimav`, including `gamescene_dialog_lifecycle_test.go`.

See also:

- [Widget Components](WIDGET_COMPONENTS.md)
- [Modal Dialog System](MODAL_DIALOG_SYSTEM.md)
- [Game UI Integration](GAME_UI_INTEGRATION.md)
- [Visual Effects System](../systems/VISUAL_EFFECTS_SYSTEM.md)
