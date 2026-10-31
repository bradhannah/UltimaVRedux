# Widget Selection And Integration

Use this guide to reuse the correct component and preserve its non-obvious
behavior. Exact constructors, options, and result types live beside each widget
in `internal/ui/widgets`; inspect its focused tests before changing it.
[Widget architecture](UI_WIDGET_SYSTEM.md) owns common layout/text rules,
[modal guidance](MODAL_DIALOG_SYSTEM.md) owns construction and cleanup, and
[scene integration](GAME_UI_INTEGRATION.md) owns callbacks and update order.

## Component Map

Paths in this table are relative to `internal/ui/widgets/` unless stated.

| Need | Component / source | Integration boundary |
|---|---|---|
| Styled text | `internal/text/`: `text.Output` | Commit line builders with `Done`; rendering remains inside Output. |
| Thematic sprite chrome | `borders.go`: `Border` | Keep intentional blue/white chrome; do not globally recolor it to style transient dialogs. |
| Classic functional cards | `classic_pixel_panes.go`, `save_slot_browser.go` | Shared stepped silhouette, measured interiors, fill before content, frames last. |
| Bounded numeric setting | `slider.go`: `Slider` | Focused Left/Right changes integer values; no pointer/drag path. |
| Grammar or free-text editor | `textinput_core.go`, `textinput_update.go`, `textinput_autocomplete.go` | Input callbacks report choices; gameplay stays outside the widget. |
| Validated prompt | `input_box.go`, `input_box_prefabs.go` | Grammar and regex modes are exclusive; use the yes/no prefab. |
| Native checkpoint description input | `save_description_entry_modal.go`, `save_description_layout.go` | Preserve native focus/composition and owner-held draft/revision state. |
| Mutable narrative | `message_dialog_core.go`, `message_dialog_content.go` | Reserve prompt capacity before using prompt methods. |
| Timed/acknowledged popup | `overlay_dialog_builder.go`, `overlay_dialog_templates.go` | Exactly one duration mode and one removal owner. |
| Staged interaction | `interaction_transcript.go` | Captures raw responses; the workflow owns sequencing and authority. |
| Chest discovery / completed battle totals | `combat_loot_dialog.go` | Detached icon/text rows; discovery owns reveal/acknowledgement. Summary is a narrower bottom-anchored, upward-growing card with right-aligned totals and clipped overflow; constructor requires exact-instance dismissal ownership. |
| Keyboard menu | `button_list.go`: `ButtonListModal` | Repeated arrow navigation; Enter invokes once; Escape invokes owner close. |
| Scrolling selection | `scrollable_list_state.go` | List state is not a widget or input owner. |
| Player checkpoint selection | `save_slot_browser.go` | Detached views; no persistence or revision authority. |
| Journal reading | `quest_journal_dialog.go`, `quest_journal_search.go`, `quest_journal_excerpts.go` | Detached opening snapshot; scene owns pause, acknowledgements, and persistence. |
| Traveller's map / personal notes | `atlas_dialog.go`, `atlas_panels.go`, `atlas_view.go`, `atlas_render.go`, `atlas_note_editor.go` | Detached point authorities and separate supplied artwork; scene owns pause/preferences and note intents; application lends one geography texture. |
| Commands / status reference | `command_help_dialog.go` | Detached command and visible-status snapshots; Tab switches pages without gameplay actions. |
| Party selection/reordering | `character_selection_dialog.go`, `character_roster_dialog.go`, `new_order_dialog.go` | Inspectable and actionable rows differ; preserve stable character identity. |
| Use selection | `item_selection_dialog.go` | Emits a detached item; workflow revalidates inventory and context. |
| Ready / Cast / Mix | `ready_equipment_dialog.go`, `spell_casting_dialog.go`, `reagent_mixing_dialog.go` | Inspect the corresponding scene workflow for commitment and cancellation. |
| Read-only views | `ztats_dossier_dialog.go`, `astronomy_dialog.go`, `sign_dialog.go`, `gem_view_dialog.go`, `dungeon_view_dialog.go` | Reuse domain-specific layout, Runic, and view behavior rather than a generic popup. |
| Merchant workspace / approval | `merchant_dialog_shell.go`, `merchant_modal_frame.go`, family `*_shop_dialog.go` and approval dialogs | Detached quote → intent → reviewed request → authoritative result and complete refresh. |

## Dungeon View

`dungeon_view_dialog.go` is a glance-and-close observation of the **current
dungeon floor**. Its detached model comes from live authoritative dungeon state.
Gem View, prepared In Quas Wis, and the In Quas Wis scroll share the scene's modal
presentation; the widget never consumes resources, advances time, changes facing,
or owns dungeon command completion. Their existing costs, cancellation,
exact-instance cleanup, shutdown handling and repeated-open input restoration
remain the invoking workflow's responsibility.

### Solid-wall presentation (A)

- The map is one north-up 8×8 floor with dungeon name, level, party position and
  authoritative facing. The **22×22 display-space reveal model remains intact**;
  the renderer collapses its visible occurrences by wrapped source coordinate.
  [VIEW-0003](../gamefacts/VIEW_SYSTEM.md#view-0003) owns that flood geometry.
- Solid filled blocks are walls, dark dotted cells are passages, and diagonal
  hatching means **Not revealed** by this observation. Shape/fill supplement colour.
  This is not an exploration journal, and visible does not mean reachable or safe.
- Stalactites, skeletons and cave-ins use the ordinary wall artwork, retaining
  their blocked terrain without a decorative special-wall category. Secret doors
  retain a solid wall background and a closed inset panel; they are not drawn as
  open passages.
- `dungeon_view_symbols.go` supplies the same opaque background and feature
  artwork to map and key, with nearest-neighbour integer scaling. Ladders have
  rails, rungs and directional heads; doorways, room entrances, chest, fountain,
  wall writing and hazards have distinct geometry rather than letter placeholders.
  Ceiling openings occupy the upper part of a symbol; floor pits occupy the lower
  part, so open/hidden pits combined with a ceiling opening remain explainable.
- A bright backed party frame and filled facing arrow sit at the cell perimeter.
  They preserve the central feature artwork for all four facings. The key's party
  entry uses the same orientation and explicitly names the facing direction.
- The consistently ordered key starts with party, solid wall, passage and Not
  revealed. Additional entries appear only for symbols in the **revealed,
  collapsed** plan: doorways/secret doors, rooms, ladders, ceiling openings and pit
  variants, traps, fields, chest, fountain and writing. Its rows, columns and
  measured label space depend on that list and the available interior; it does
  not reserve a fixed eighteen-entry legend. Key icons scale with key typography
  rather than map cells, with compact row spacing so a dense key does not force
  the primary map to shrink merely to accommodate oversized legend icons.
- Plain paired `W-E1`…`W-E8` and `N-S1`…`N-S8` labels outside opposite map edges
  identify known west/east or north/south continuations **on the same level**.
  A pair requires adjacent revealed display occurrences with the expected wrapped
  coordinates and non-wall endpoints; seeing endpoints separately is insufficient.
  Hidden endpoints never supply labels. These are neither ladders nor exits, and
  a labelled seam can still contain a hazard. The footer explains same-level
  wrapping and makes no safe-route promise. The ordinary edge-label font is
  independently scaled and measured; north/south tags are centered within their
  own column, with separation even when all eight columns have known seams.

Any fresh key closes the view, after invoking keys have been released, matching
the surface Gem view. Arrow keys do not move the party or inspect cells. Resizing rebuilds presentation resources
while preserving dismissal and input-release state.

Focused coverage separates classification/reveal boundaries, small exact-RGBA
map/key checks, all-facing feature preservation, four-level greyscale distinctions,
and measured layout at 720p and larger sizes. The opt-in
`dungeon_view_wireframe_test.go` generator (`dungeonviewwireframe` build tag and
`DUNGEON_VIEW_SAMPLE_DIR`) writes synthetic simple, dense, partly revealed and
both-axis wrapping samples (including all eight seams on each axis) at 1280×720
and 1920×1080, plus party-over-feature
facings and reduced-colour versions. These samples support maintainer visual
review; generating them or passing geometry checks does not establish visual
acceptance or native gameplay/input acceptance.

### Considered alternative C: optional inspection

Keyboard inspection is **not implemented**. The considered C design would retain
the A map and add an arrow-key inspection cursor visibly distinct from the party,
with a plain-language side panel for the selected revealed cell. An unknown cell
would say only that it is not shown by the current view. Inspection would have no
gameplay effects or additional information authority, and closing would stay
straightforward.

Reconsider C only after visual testing A: if players understand terrain and
orientation but still need explanations of uncommon features, optional inspection
may help. If basic walls, passages or facing remain confusing, improve A first.
C is a documented alternative, not a shipped control or required follow-on.

## Traveller's Map

The keyboard-only atlas opens/toggles with exact `Shift+M` (either Shift, no
Ctrl/Alt/Meta). Its ten visible category tabs are All, Settlements, Keeps,
Castles, Shrines, Lighthouses, Dungeons, Moonstones, Vessels, and My Notes. `Tab` advances the category
and `Shift+Tab` reverses it, wrapping at either end. Fresh released taps are
accepted immediately without gameplay repeat timing; holding Tab selects once.
There is no filter submenu. Opening starts in Places mode, where Up/Down selects.
`P` toggles Pan mode: plain arrows then pan a zoomed map, without macOS-reserved
Control-arrow chords. `P` again returns to place selection; mode changes block
held navigation until release. The header visibly says `PAN MODE` and the two-row
footer describes the current arrow action. Tab/Shift+Tab still changes category
in either mode. Pan mode is local to the open dialog and resets on reopening.
`0` (number row or keypad) restores canonical north-up 1×; `+` / `-` (including keypad Add/Subtract)
choose 1× / 2× / 4×, spanning 256 / 128 / 64 tiles. `Enter` focuses an authorised
selected point; `A` (shown as `[A] Avatar` only when a fix is available) focuses
the current valid sextant fix. Both focus actions raise 1× to 2× and retain an
already closer zoom. Neither can use an
unmeasured private site. Escape closes; Space (or PgUp/PgDn) pages details only when
their content overflows. `L` shows/hides the single selected-place callout; there is no
automatic label-culling mode. In a category overview, `L` instead shows/hides all
authorised in-view labels. `G` toggles the chart grid.

First opening per scene is whole-world, All selected, selected-place callout on,
grid off. Manually chosen camera, zoom, category and selection survive reopening
within the scene. Selection/category changes do not move the camera. Current
Avatar visibility is automatic and independent of categories. The selected tab
uses the shared yellow text highlight, matching Journal and Z-stats. The All list
is stably grouped in tab order, with Buccaneer's Den beside the other townes.
Principle
strongholds and discovered keeps are in Keeps; discovered huts and villages are
in Settlements. The remaining icon types have their own category. Supplied-map
and personally measured entries both pass the selected type filter. Known
unmeasured rows are included automatically, marked as unmeasured and without an
interactive pin or focus action. There are no source switches, mixed checkbox/radio controls,
or undiscovered placeholder rows. Discovery-only keeps (including Bordermarch and
Stonegate), huts and villages remain absent until personally observed. Entering
records the presented name even without a sextant; the known but unmeasured
entry has no interactive map pin. For landmarks, moonstone destinations and
Guardians, `AtlasEntry.PlotPosition` accepts only a recorded survey. Personal
notes and registered parked vessels use their own explicit point authorities;
none use supplied artwork as a fallback. The detached entry has no alternate
`ChartPosition` field. Enter on an unmeasured row explains
`Measure this place to locate it from the list.` without
changing camera or zoom. A later eligible local reading earns the interactive pin.

The 34-row supplied directory is distinct from the **27 static supplied symbols**:
eight virtue towns, three principle strongholds, Lord British's Castle, seven
physical shrines and eight moonstone origins. `AtlasDossier.Symbols` carries only
this finite decorative layer. It excludes Buccaneer's Den, Blackthorn's Castle,
all lighthouses, Guardians and discovery-only sites. Symbols are light gray,
category-filtered and never generate names, numeric coordinates, brackets,
leaders, Enter focus, discovery colour, or mapped counts. An earned pin with the
same identity and position coalesces visually with its supplied symbol. Printed
moonstone-origin artwork remains fixed after pickup/relocation; current personal
destination markers and retired regional notes retain their separate rules.

The Places heading shows `N total, M mapped` and a `Coordinates recorded` legend;
only entries accepted by `PlotPosition` get the row's coordinate stamp. Each
specific category, including Vessels and My Notes, ends with `All mapped (N)`.
All has no synthetic overview row. Overview is presentation-only, excluded from
counts and persistence, has no selected-point brackets or Enter-centering, and
preserves camera/zoom. With labels enabled it lays out every authorised in-view
nameplate deterministically, using bounded collision search and a crowded fallback
rather than silently dropping labels. Offscreen, empty and unmapped states are
explained in text. Selecting an ordinary row restores the single-place label.

The one acquisition exception is a real absent-to-owned sextant grant inside a
valid Greyhaven floor, after its entry has legitimately supplied the name. It
records only Greyhaven's canonical exterior, including daytime/strict mode,
preserving naming evidence and truthful `sextant-receipt` provenance. Other
visits are never backfilled by possession, setting changes, readers or loading.
The sextant cannot be lost in normal gameplay. Recorded surveys remain durable
across save/load and flexible-use changes; a stored Greyhaven point does not
enable a strict-mode interior Avatar fix.

The footer has exactly two measured physical help rows at supported sizes, with
normal body typography and complete key/action segments.
Current position and transient status live in the right pane, never enlarging
the footer. A fixed measured status block takes space from the list/details, so
changing fix eligibility never changes the map size. `YOUR POSITION` heads a
coordinate line and, when supplied by runtime, a separate settlement context.
`POSITION UNAVAILABLE` heads the complete word-wrapped reason; transient feedback
also wraps without ellipses or reduced typography. Trying to pan the whole world
explains that zoom is needed first.
The compact title, measured category rows and fixed footer retain a large
geography pane. Category layout wraps full
labels instead of hiding them or using abbreviated names.
Single-page details have no page indicator or reserved paging row. Landmark details do not
repeat the selected name/type or explain the supplied map. Before genuine
personal discovery landmark details say `Not yet visited.` and show no coordinate pair. A
discovered but unmeasured site says `Coordinates: Not measured`; only a genuine
sextant survey supplies their numeric coordinates or interactive map geometry.
Supplied references never serve as an entry plotting/focus fallback, including original moon sites. Phase
names and original regional notes remain reference text without exact points. A printed name or
symbol does not count as personal discovery. Only relevant current-placement
observation and real measurement dates add further rows.

Player text uses font-supported punctuation, including `...` for measured name
truncation. Supplied townes and principle keeps begin as Towne/Keep plus a stable
neutral number, including Buccaneer's Den's supplied position as Towne 9.
Lighthouses and shrines likewise start numbered; Blackthorn's castle is unnamed.
Lord British's Castle, the Guardians and moon-phase symbols are explicit supplied
names. Other site names require genuine entry/introduction evidence. Look, sextant ownership,
surveying or merely being present in a reference catalog do not reveal names.
Guardians remains the single `chart-25` row. A successful exterior Look at either
verified angel artwork can record observation and an eligible normal survey of
the checkpoint, without fabricating a learned-name event.
Successful entry replaces the generic label with the actual revealed name,
independently of sextant eligibility, and save/load retains that knowledge. Older
saves without visit/name evidence do not fabricate it. Castles, principle strongholds, discovered keeps, settlements
and huts have distinct compact five-cell pixel stamps shared by the map and Places rows:
a single house, battlement tower, twin-tower stronghold, wide castle, stepped
shrine diamond, beacon tower and cave arch. Artwork stays two `ClassicUIPixelUnit`
widths across. Its dark outline is approximately one of the five artwork cells
wide, rounded to the nearest screen pixel, rather than a full message-box border
unit. Outline thickness does not resize the coloured silhouette. Artwork uses a separate
fine pixel grid with integer-rounded edges and no smoothing; multiplying each
art cell by the border unit would incorrectly enlarge the whole marker again.
At 1280x720 the body is 8 pixels across, 12 including the 2-pixel outline. Sizes
remain independent of map zoom. Tiny custom resolutions preserve at least one
screen pixel per source cell so distinct phases do not disappear.
Moonstones use eight distinct compact phase stamps on the same five-cell grid; their
phase names omit the original-site suffix. The current Avatar is a chunky red
plus whose body grows to 100% / 150% / 200% at successive zoom steps, with both
arm length and stroke width increasing while the outline thickness stays fixed.
At 1280x720 its body is 8 / 12 / 16 screen pixels across at 1x / 2x / 4x, with
2 / 3 / 4-pixel strokes (12 / 16 / 20 including the outline).
Callout placement accounts for those larger
bounds. The cross is drawn last and clipped to the map pane; other place markers
retain their zoom-independent size.

Landmark list symbols and names are light gray until personally visited/discovered,
then green, independently of name knowledge. Interactive landmark symbols additionally
require measurement, so unmeasured discovery colour never identifies supplied
artwork as an earned point. Directory selection
uses the standard white inversion bar with black text and no yellow border;
its symbol uses dark gray or dark green to retain discovery contrast. Rows reserve
the full symbol cell height. Map selection uses chunky, integer-snapped yellow
two-cell corner brackets with a half-unit dark backing and close clearance, preserving the symbol's discovery
colour. Explicit status text and measurement availability remain in the details
rather than relying on colour alone.

Texture, markers, grid and labels share the same positive-modulo transform.
Seam-crossing regional views draw at most four rectangles of the retained
texture. Directory rows remain individually selectable at overlapping points.
Ordinary selection shows at most one place-name callout; category overview shows
all authorised in-view callouts. Each yellow classic stepped frame and
interior padding scale with `ClassicUIPixelUnit`. A short integer-segment elbow
connects its edge to the selected marker's exact centre, with a dark contrast
underlay at the same resolution-scaled thickness. Markers render above the line.
Placement avoids other markers and selection geometry where space permits;
the bounded crowded fallback retains authorised labels. Offscreen selection stays in the list and
can be centred with Enter, without a fictitious edge marker. Region names always
remain subdued, unboxed area text in every category and L mode, yielding when
they overlap the selected plate. Multiword names use measured, balanced two-line
blocks (for example `Valorian / Isles` and `Isle of / the Avatar`). The complete
block is positioned inside map margins at each zoom and seam; clamping decoration
never changes the world camera or makes it a precise site. They never add directory rows, point symbols,
leaders, or counts. Outputs, wrapping, detail paging and footer space use logical
display dimensions and measured font metrics.

### Vessels And Personal Notes

Vessels requires sextant possession, but automatic fleet bookkeeping does not
require the ordinary nighttime/outdoor reading gate. A successful boat-use
boundary registers a frigate/skiff only while a sextant is carried, allocating
one stable `VesselID`. There is no pre-sextant history or acquisition/load backfill.
The same identity follows boarding and parking; the list projects only registered,
visible parked Britannia-surface boats, omitting the boarded vessel, pending
deliveries, destroyed frigates and removed units. It reads current authoritative
vehicle positions rather than keeping a second location cache or stale trail.

`N` marks the **current eligible sextant fix**, never the selection or map cursor.
Flexible settlement fixes capture the canonical settlement entrance with an
explicit explanation. A note at that exact position opens for editing instead of
creating a duplicate. `E` edits a selected note; `D` opens deletion confirmation.

From safe primary surface-overworld exploration, exact `Ctrl+N` (either Control,
no Shift/Alt/Meta) opens the same atlas and immediately invokes its Mark Current
Position intent. It reuses the current-fix draft and same-position edit behavior,
independently of camera position/zoom. A carried sextant must be usable under the
shared nighttime or flexible daytime rule; an unavailable fix opens the atlas's
explanation without a child editor. The shortcut's narrower context and held-N
release guard belong to [input routing](../systems/INPUT_HANDLING.md#travellers-map).

Creating a note needs a valid current fix; editing/deleting an existing note does
not. Personal-note map markers use the existing paper silhouette in green, with
its dark outline and independent yellow selection brackets. Existing notes need
no migration. Places-row text and vessel colours retain their existing roles;
green note ink grants no discovery. GameState owns single-use, checkpoint-bound
drafts and revalidates before commit. Editing preserves the original point and
date. There are at most 256
notes, with a required trimmed label of 1–32 printable ASCII characters and an
optional description of at most 500 characters (printable ASCII plus LF).
Creation refusals explain unavailable fixes or capacity in the atlas. Validation
or stale-commit errors preserve an open editor rather than truncating or evicting
notes. Stable IDs and their allocation watermark belong to checkpoint
state, as do the Journal's kept excerpts.

`AtlasNoteEditor` is an exact-instance scene-owned child within the paused atlas.
Tab/Shift+Tab changes fields, Enter adds a description paragraph, Ctrl+Enter
commits, and Escape discards. Its cursor, measured wrapping and scrolling retain
the complete draft through resize. Deletion uses Enter to confirm and Escape to
keep. Successful commit refreshes the detached atlas; failed commit keeps focus.
Save and cancel remove only the child and return to the underlying atlas, with
key-release protection. This includes Ctrl+N entry: the atlas pause lasts through
the editor and ends only after the parent closes and its closing keys release.
The shortcut/editor add no gameplay time, RNG draws or resource costs.
Parent close, disposal and GameState replacement remove the child and invalidate
the draft. Widgets neither choose authoritative coordinates nor mutate gameplay.

Both fields use owned native text sessions compatible with save-description and
Tavern editing. Only the top modal starts a session; changing fields or losing
focus releases it while preserving accepted text and selection. The scene calls
`SuspendTextInput` before skipping a native child during an unfocused Atlas
frame. Composition precedes control keys, and candidate-confirming controls
remain owned until release. Committed text retains the existing ASCII and length
policies; rejected replacements preserve the untouched suffix. Native failures
keep the draft and expose cancellation. `atlas_note_editor_native_test.go`
exercises the handoff, field switching, selection, composition and cleanup.

Both editor fields use `ClassicUIPixelUnit` for their classic stepped border
thickness and corners, with matching scaled interior padding and caret width.
`internal/ui/widgets/atlas_note_editor_layout.go` supplies one measured interior
for text wrapping, clipping, visible-row capacity, scrolling and caret placement,
reserving one classic pixel unit after the text for an end-of-line caret. The
single visible **Map label** line is vertically centred within that interior;
the multiline description starts at its top padding. Caret height follows the
same measured text height in both fields.

### Shared map and journal notices

`gamescene_journal_notice.go` queues semantic `UI.AtlasUpdated` deltas and journal
updates for the ordinary main Output. Same-site changes coalesce; different map
sites retain FIFO order ahead of the coalesced journal notice. Both frame-entry
and frame-exit blockers must be clear before the pending batch flushes. Secondary
prompts, arrival presentation, modals, workflows, readers and fades defer it.

`Map updated:` reports legitimate discovery/name/measurement changes, with
measured/not-measured status but no exact coordinate pair; `Journal updated.`
uses the same output channel. Separate `[Shift+M] View map` / `[Shift+J] View
journal` lines use semantic key/hint colours; labels and values are likewise
distinct. Normal Output wrapping and History retain these notices. There is no
top-centre card, visible-time countdown, fade, focus, or dialog-stack entry.
The queue neither opens/selects readers nor restricts saving, and is cleared on
scene replacement/disposal. Hydration and browsing do not replay it.

### Geography and cache

`internal/atlasterrain` composes on CPU from the loaded immutable
`OverworldLargeMapReference` and `sprites.TileAtlasPNG()` artwork. Its versioned
finite policy checks 19 exact source guards and 69 feature substitutions on the
reviewed 74-ID map. Natural cells retain source pixel shapes and geography;
unsupported inputs return a recoverable diagnostic. There is no gameplay tile,
visibility, quest-state, sprite GPU readback, or original binary decoder here.

The cache is `~/.ultima_v_redux/cache/atlas/<identity>.png` plus validation
metadata: native 4096×4096 terrain only, no runtime points, labels, grid or Avatar.
Identity hashes terrain, artwork and the dimension/palette/substitution policy
version. Hits validate bounded encoded size, metadata/hash and dimensions before
image decode, skipping composition/encoding. Malformed entries regenerate;
temporary publication and atomic rename prevent valid partial hits. Write
failure logs a wrapped diagnostic and retains generated pixels in memory.

`Game.atlasTexture` owns one GPU texture across modal and scene lifetimes;
dialogs borrow it. Owner replacement/shutdown releases it. CPU image and encoding
buffers become unreachable after upload submission. A native RGBA image is
64 MiB CPU plus at least 64 MiB GPU, excluding staging/encoding/driver overhead.
Central logs report resolved PNG path, cold compose/encode, warm decode and
upload-submission timing. Submission is not a GPU-completion or peak-memory
measurement; hardware profiling must record those separately. Generation and
cache I/O never run in Draw.

Focused coverage lives in `atlas_test.go` in widgets/journal/game state,
`internal/atlasterrain/cache_test.go`, `internal/references/atlas_test.go`, and
`cmd/ultimav/gamescene_atlas_test.go`, `gamescene_atlas_mark_shortcut_test.go`,
`gamescene_atlas_notes_test.go` and `atlas_texture_test.go`. Real-data tests use
temporary cache roots; derived geography must not be committed.

## Sign Reading

`SignDialog` normally presents the original encoded inscription. The passive
Ctrl+R alphabet remains separate from its acknowledgement/input owner. A hidden
maintainer-only Ctrl+T view can translate that exact sign while its alphabet is
open; it is absent from player Help and visible hints, resets with reference
closure or dismissal, and never records translated History or knowledge.
The English view replaces understood Runic lettering with yellow IBM bitmap
lettering inside the source sign. Brown artwork and white IBM glyphs remain
unchanged, as do the source rows, glyph height and sign/dialog geometry.
Compound runes fit within bounded translatable spans rather than reflowing the
inscription or removing artwork.
See [SIGN-0008](../gamefacts/SIGNS_AND_FONTS.md#sign-0008)
for the bounded decoding and release-guard contract.

## Shared Presentation Contracts

All player-facing widgets are keyboard-only and use `text.Output`. Constructors
validate mandatory wiring; optional configuration is not permission to ignore a
missing keyboard, callback, or required state. Results are feature-specific:
DialogStack does not automatically poll a result interface.

Classic panes share a logical 320x200 stepped silhouette. Fill all panes before
content, then draw white `DialogRuleColor` frames last. Connected frame edges
overlap by one classic pixel. Buffered single-view cards use the matching alpha
mask. Use shared typography and `dialog_constants.go`, not copied measurements.

Responsive widgets rebuild derived geometry/fonts/output capacities when logical
display dimensions change, preserving selection, input, visibility, pending
result, and workflow state. There is no generic flex/grid or resolution-scaler
framework. Focus helpers are owner-driven presentation, not an alternative input
router or a claim of accessibility compliance.

## Shared List Navigation

Player-facing lists use `pkg/input.ListNavigation`. Widgets embed the local
`navigation` focus adapter; the spell browser owns its repeat state and its
containing dialogs forward focus changes. Poll once per focused Update, before
text input or any competing arrow handler. A held navigation key owns the frame
even when a repeat is not due. Resolve Shift before registering the arrow, so a
page chord cannot become a single-row move or trigger both actions.

The adapter delegates its owner-local countdown and captured-key barrier to Berad
Input; page meanings, key sets, Shift resolution and `DialogStack` ownership remain
Redux policy. All widgets in one scene share the scene keyboard's consumption and
strong suppression. Do not create a private keyboard to restart list repeat or
clear a parent's release gate. Cover/return/removal notifications remain necessary
even when a covered widget receives no Update. The supplied callback remains live;
the extraction does not require each widget to capture another edge sample.

- Up/Down selects one row and repeats while held.
- Shift+Up/Down aliases Page Up/Down, including both physical Shift keys.
- First movement and fresh taps are immediate; holding waits 300 ms, then repeats
  every nominal 120 ms, quantized to update ticks. Gameplay repeat settings are
  independent. Changing Shift during a hold changes the next scheduled action.
- Opposing/simultaneous navigation keys produce no movement. Focus changes require
  releasing held navigation keys. Input-mode changes reset the repeat state.
- Activation, dismissal, spell letters, numeric shortcuts and tab selection
  remain separate, one-shot policies. Merchant quantity forms have their own
  [horizontal-repeat policy](#merchant-quantity-controls); native text editors
  retain their text-selection semantics.
- Advertised Enter acceptance supports both Return and keypad Enter. Tab/Shift+Tab
  traverses sections/casters in opposite directions where cycling is supported.
- Compact footers prioritize Shift+Up/Down; physical Page Up/Down remains available
  wherever paging is supported. Command Help displays both forms.

Paging and boundary exceptions are explicit:

| Surface | Page action / boundary policy |
|---|---|
| Z-Stats ordinary lists, Use, Ready, Mix ingredients, Arms catalog | Half a visible list; clamp at first/last item. |
| Shared spell finder (Z-Stats, Cast, Mix), ordinary Journal lists/search | Full visible list; clamp. |
| Journal Last Conversation / Excerpts | No Page-key or Shift+arrow action; Space/Shift+Space pages within an exchange. Arrows select exchanges. |
| Talk history / Tavern transcript / Command Help | Page the reader's text using its existing measured page boundaries. Talk's Shift+Up also opens history. |
| Button menus | Page the selected description; arrows retain wraparound selection. |
| Party selectors / New Order | No paging; repeating arrows retain wraparound selection and unavailable-row inspection. |
| Intro menu / Configuration tabs | No paging; repeating arrows select bounded rows. On the title menu Right opens and Left closes a sub-menu; Left/Right setting adjustment remains one-shot. |
| Save slots; compact Guild, Reagent, Healer, Inn, and Tavern choice lists | No list paging; repeating arrows retain existing selection rules. Unsupported page chords do not fall through to row movement. |

Combat HUD history remains on gameplay input routing, outside this modal-list
policy. Space details, Left/Right, letter shortcuts, and tab cycling are
surface-specific; do not infer a global binding from one dialog.

Regression entry points: `pkg/input/list_navigation_test.go`,
`internal/ui/widgets/list_navigation_test.go`, the Journal held-search tests,
and the existing selector/merchant/reader navigation and layout suites. Physical
keyboard verification must also exercise both Shift keys, held repeats, search,
and return from a nested modal in the running game.

## Save And Native Text Entry

`SaveDescriptionEntryModal` is the concrete checkpoint-description editor. It
uses platform text input for punctuation, spaces, selection, and IME composition.
Required `OnResult(text, submitted)` reports the user's choice; validation keeps
invalid input open with an error. Redux has no clipboard adapter, custom paste
handler, or paste hint; OS text-service behavior remains separate.
There are no in-game data or legacy-save path fields: custom data locations use
external settings editing, while title-menu import reads the active data folder.

The caller retains the reviewed revision and draft and removes the exact editor
and browser before gameplay capture. `OnResult` runs before automatic stack
removal. Invalid/oversized input preserves the prior value rather than truncating;
native input errors retain text and show a failure instead of silently cancelling.
Ctrl+U clears without submitting, recognizing either `u` or `U` accompanying the
physical chord, including at the rune limit and with a selection. Native
composition remains the owner until its controls release. Backspace has delayed repeat, and native
composition edits must not replay as local deletion. Opening Enter is release-
gated; focus loss/dismissal resets repeat and native text focus follows top-modal
ownership.

The native description editor and the default save browser use normal body
typography. Focused editors show the visible selection with inverted text on a
high-contrast background, including backward selections and composition ranges.
An empty Load browser explains Create/Import and exposes only dismissal guidance.

`SaveSlotBrowser` consumes detached views and emits primary/cancel callbacks.
Each slot row shows the slot name with the save's description beneath it, both at
body size. The player's description editor accepts at most
`SaveSlotRowDescriptionMaxRunes` (22) characters, the most that fits whole on a
row in the narrow in-game layout at every supported size. Longer notes from earlier
saves or imports end in an ellipsis on the row; saving over one pre-fills its
first 22 characters for review. The selected save's description also leads its
details, in full, directly under the slot name.
The application chooses the newest marker independently of cursor position:
Save uses the active playthrough, Load the overall newest ready checkpoint.
Load skips empty slots; Save retains them. Require explicit selection when there
is no appropriate empty/matching destination. A corrupt regular file can be
offered for deliberate replacement only with an inspected raw-byte revision;
Load stays disabled and the player browser offers no Delete.

Up/Down navigation repeats deterministically; fresh physical taps bypass the
shared delay. Enter invokes once, Escape cancels, and Space (or Left/Right) pages
details when needed. Page Up/Page Down has no slot action. Review labels
are opt-in; ordinary browsers do not inherit extra review geometry.

See [player slots](../details/SAVE_SNAPSHOT_SCHEMA.md#player-slots) for persistence
authority. Tests include `save_slot_browser_test.go`,
`save_description_submit_input_test.go`, `save_description_lifecycle_test.go`,
and `save_description_native_input_test.go`.

### Tavern Subject Native Session

`TavernShopDialog` uses the same native field protocol as the save description
editor. A native save session can leave macOS character delivery outside
`AppendInputChars` after blur; the Tavern therefore acquires its own native
session rather than relying on raw character input being restored. The shared
`native_text_field.go` contains only the field interface and factory.

The Subject field is the single text authority and focuses only while the visible
Tavern is the top modal, the window is focused, and an eligible customer is in
Conversation/Browse. Quote, approval, result, notices, Order, session errors,
Hide, dismissal and loss of focus release it. Returning to Browse or from a
nested modal retains the committed draft and requires held transition controls
to release. The Tavern focus listener also preserves the existing list-navigation
reset. Workflows still own exact stack removal and all merchant authority.

When the scene skips input Updates, it must call
`TavernShopDialog.SuspendTextInput()` from its suspended-dialog path. The hook
does not poll keys or change modal ownership: it releases native focus, resets
local control/navigation holds, and leaves the next allowed Update release-gated.
It is safe under suspended keyboard polling and repeated calls. Blur preserves a
bounded committed draft and byte selection captured before release; native commits
flushed by Blur and uncommitted preedit cannot leak into a resumed draft, cancelled
subject or already emitted intent.

Native text is pumped before speech paging or control routing. Composition owns
Enter, Escape, Backspace and navigation through physical release; accepting a
candidate cannot submit a subject, leave Conversation, or page history. Committed
input retains at most 15 printable Unicode runes, including whitespace verbatim;
replacement capacity is applied to inserted text without truncating an untouched
suffix. Preedit remains visible without premature truncation. Selection uses native byte
offsets, with a measured visible caret and candidate rectangle. Left/Right and
Home/End edit the eligible native Subject; Ctrl/Cmd+A selects it. Shift+Left/Right
retain merchant speech paging outside composition. Tab/Escape return to Order.
Native errors retain the draft, release focus and display an acknowledged failure.

Leave `TavernShopDialogConfig.AppendInputChars` nil in production. A supplied
function explicitly selects the detached rune adapter, with no native field or
second authoritative buffer. `IsKeyPressed` should use the scene keyboard's
`IsKeyHeld` adapter so control release checks and routed keys share one source.
Regression coverage is in `tavern_native_input_test.go`: simulated save-session
handoff with raw delivery still empty, modal/phase ownership, Unicode selection,
preedit control precedence and bounded candidate placement. This simulation does
not establish native OS runtime acceptance.

## QuestJournalDialog

The Journal is a modal over a detached opening snapshot, with Chronicle, Virtues,
Shadowlords, Dungeons, Last Conversation, Excerpts, and Moonstones tabs. The scene
owns the [Journal pause](../systems/TIME_SYSTEM.md#journal-pause), exact removal,
visit acknowledgements, and excerpt persistence. Browsing never mutates gameplay.

### Navigation And New Entries

- Curated tabs retain independent selections. Up/Down clamps; Page Up/Page Down
  moves a full visible-item screen; Home/End selects first/last.
- Tab/Shift+Tab cycles tabs; Space pages measured details. Selection/tab changes
  reset detail paging; resize preserves valid selection and pages.
- Escape or Shift+J closes, except Escape from search returns to browsing.
- Stars mean new since the last visit to a curated tab, not individually unread.
  They stay frozen during browsing/search/resize. `VisitedTabs` records actual
  visits, including opening Chronicle. Dismissal acknowledges only visited tabs,
  only through the opening revision. Do not repurpose
  spell Unread or Chronicle Acknowledged state for these baselines.

### Search And Discovery

`/` searches display-safe title/detail values in readable entries,
Last Conversation, and visible kept Excerpts. Archived excerpts are
excluded unless Show Archived is on; Last Conversation remains searchable
independently of notebook archive state. Every case-insensitive query word
must match, in any order. Hidden IDs, unlearned values, and Runic transliterations
are not indexed. Stable destination identities survive query changes. Enter
opens a result; searching alone does not visit its tab. Empty/unmatched queries
show explicit instructions/no-match state.

Results use three measured rows: title/category, known speaker/location where
available, and a matching snippet. Query terms are highlighted with contrast on
both normal and selected rows. Snippets elide Runic passages rather than reveal
their transliteration; complete captured source remains unchanged in the reader.
Enter reveals the first rendered text match inside the selected exchange/message
or curated detail pages; metadata-only matches open the entry normally. Match
emphasis is transient and clears when choosing another entry. Paging and held
navigation use the same three-row result pitch as rendering.

Dungeons lists discovered identities only, with independent observed-entrance
state and explicitly learned words. It does not expose unknown rows or completion
counters and does not reinterpret curated visit stars. Once the Codex word is
learned, a final **Final Gate** row presents it as a gate clue rather than a
dungeon: heading, Word of Power, source and the disclosed clue text, with no
entrance or location fields. The clue text is the word-only summary until final
Codex acknowledgement, then the same four paragraphs as the Chronicle entry, one
paragraph per detail line for measured paging. Both views derive from the same
knowledge and completion state, so searches for the word, source or clue text
legitimately reach both tabs.

After the final Codex page is acknowledged, the existing word-only Chronicle
entry becomes **Codex revelation: VERAMOCOR**, containing all four clue paragraphs
through the ordinary measured detail paging and visible-text search. Word
knowledge still records after the preceding word-bearing page; interruption before
final acknowledgement leaves that word-only entry intact. The first completion
updates the existing entry's visit revision once, preserving its original identity,
date and valid source provenance; repeated completion is silent. Reading/searching
grants nothing and reveals no unspoken Doom name, entrance association or
coordinates. See [Codex discovery boundaries](../gamefacts/WORDS_OF_POWER.md#word-0005)
and [V24 persistence](../details/SAVE_SNAPSHOT_SCHEMA.md#snapshot-24-personal-state-and-chest-lifetimes).

Moonstones always lists all eight phase names. Until personally retired, a phase
may show its original regional label from the supplied map; personal destinations
begin **Not personally confirmed**. Pickup/reburial retires that original-site
note and personal origin-marker authority, including return to the same
coordinates; static supplied origin artwork remains. The atlas has one
stable phase entry, not an original/history entry alongside its new destination.
Rows, search-result titles, and the persistent detail heading use the actual
`0x30 + phase` Runic glyph through mixed-font `text.Output`; full names are
measured to fit, never ellipsized or wrapped away in one-row buffers. Selected
glyphs use the selected text foreground, preserving the New Moon outline against
both backgrounds. Moonstones occupies the second measured header row.

Moonstone details distinguish carried, current last-observed placement, and
unmeasured states. Coordinates appear only from public current surveyed records,
with world/floor and independent observation/measurement dates. The glyph heading
remains above every detail page. Search includes chart labels, phase names, safe
sources, status, dates, and current measured facts; it excludes unknown placeholders,
glyph bytes, retired origins and stale prior-placement coordinates. Pickup shows
Carried with no destination; an unmeasured reburial has no pin or old-coordinate
fallback. Neither widgets nor search receive private recognition sites or observer contexts. See
[moonstone knowledge](../gamefacts/MOONSTONE_LOCATIONS.md#moon-0006) and
[surveys](../gamefacts/MOONSTONE_LOCATIONS.md#moon-0008).

### Main Output Compatibility

History is no longer a Journal reader, tab or search source. The bounded
[plain main-output source](../systems/MESSAGE_OUTPUT_SYSTEM.md#journal-history)
remains necessary for restoring the live message rail from supported saves.
Its canonical collection, validation and snapshot field are retained; this UI
removal requires no save-schema change. Curated visit IDs remain 0, 1 and 2.
Talk's own history, Last Conversation and save-owned Excerpts remain separate.

### Conversation Readers

Last Conversation displays the complete retained transcript with exchange
selection. The selected Avatar input and reply invert, including Runic passages;
attribution and neighboring exchanges do not. Long exchanges start at the top;
short ones can retain context. Up/Down selects exchanges; Space/Shift+Space pages
within the selection and clamps without choosing another exchange. These readers
have no Page Up/Page Down or Ctrl+arrow paging aliases. Hints/page indicators
appear only when needed.

K keeps the complete selected exchange, including offscreen pages, without moving
the selection or reading position. Excerpts shows saved source text normally,
without capture inversion, summarization, or whitespace collapse. Delete or D
requests permanent removal; Enter confirms and Escape cancels. A archives the
selected active excerpt or restores an archived one. H toggles Show Archived,
initially off each time Journal opens. The mode is shown in the keyboard hint;
archived rows are prefixed `[Archived]` and use semantic yellow, inverted to dark
text on yellow when selected. The Excerpts footer reserves an extra row for these
controls. Selection retains its identity when visible, otherwise clamps to a
remaining row. Keeping an already archived exchange restores it without
duplicating or shortening its captured speech. Scene callbacks persist changes
in the active game and report errors in the reader. Normal saving embeds the
collection and archive flags in that save; loading another save replaces them,
even within one playthrough. The scene never reads a notebook while browsing.
No free-form editor is provided. Kept exchanges
are marked in the last conversation list.

For a selected exchange containing Runic text, Ctrl+R shows the existing alphabet
in the list pane, leaving the recorded passage readable. Ctrl+R or Escape closes
the reference without losing the selected exchange or page. Tab/search/selection
changes close it; it neither translates the passage nor changes journal knowledge.

Use friendly location labels and the font-supported ASCII ` - ` separator.
New conversation records use the NPC's semantic name for attribution without
requiring a NAME question, including capture from Last Conversation after Talk
closes. This labels the source without capturing unseen dialogue or learning clues.
For older records, once an explicit name is presented, earlier exchanges in that
conversation and matching saved excerpts receive the detached attribution correction without modifying
captured speech. The original Shadowlord sprite is integer-scaled without
invented per-lord palettes. Shared footers distinguish keys from descriptions and
keep full `Shift+` spellings.

Verification: `quest_journal_dialog_test.go`, `quest_journal_feature_test.go`, `quest_journal_excerpts_test.go`,
and the scene-owned Journal tests. Conversation capture semantics are in
[the conversation system](../systems/LINEAR_CONVERSATION_SYSTEM.md).

## Merchant Presentation

Merchant integration proceeds from immutable family references to authoritative
domain operations, a live session/workflow, detached dialog previews and explicit
commitment, then typed Workbench inspection. UI code must not duplicate pricing,
eligibility, theft or inventory mutation. Revalidate at commitment, clean up the
session on every exit, and test cancellation and partial-capacity outcomes.
The family rules live in [Merchant Services](../gamefacts/MERCHANT_SERVICES.md),
especially [MERCH-0003](../gamefacts/MERCHANT_SERVICES.md#merch-0003) for transactions.

`MerchantDialogShell` owns common presentation geometry, detached header/gold,
portrait, latest exchange, and footer—not input or transaction state. The latest
exchange is one replaceable value, not a transcript. Draw its background before
family content and common frames/regions afterward. Use semantic merchant
background/key colors across parent and nested surfaces.

The card uses 80% of the screen width and gives one third of the body to the
merchant, preserving the trade workspace while widening speech. The portrait/name
uses one quarter of that column's height. Latest speech is retained in full;
overflow uses measured pages rather than ellipsis. Family input owners reserve
Shift+Left/Right for previous/next speech page, with a page/key hint inside the
speech pane only when needed. Holding a key does not skip pages. A changed
utterance resets to its beginning; refreshing the same text preserves the page,
and resize clamps it. Paging owns no transaction, session, or gameplay time.

`merchantModalFrame` shares placement, bounds, insets, classic pixel geometry,
and resize caching for nested approvals. `newOutputs` builds their title-size
header, body and footer outputs from a per-dialog budget (a body either fits its
area or keeps a fixed line count), and `drawOutputs` draws them. It is not a
Widget and owns no text content, input, callbacks, requests, Preview state, or
lifecycle. Each concrete dialog uses its change-reporting layout method to
rebuild derived resources and populates its own text. Shop windows draw their
light-framed panes with `drawMerchantPane`.

### Transaction Pattern

1. Supply complete detached family state and finalized prose from authority.
2. The parent emits one typed intent and blocks duplicate input.
3. The workflow opens an exact owned approval/quantity modal. Cancellation emits
   no commit request; release-debounce before returning input to the parent.
4. Confirmation emits the reviewed request. Authority revalidates current state;
   widget previews never calculate prices, placement, capacity, or effects.
5. Install a complete refreshed state and finalized result/refusal. Retain stable
   selection where appropriate; refreshing only gold leaves stale quotes.
6. Preview is visibly labeled before confirmation and stops before transaction
   authority. Workflow `Finally` owns exact dialog/session cleanup.

`cmd/ultimav/gamescene_merchant_talk.go` is the integration entry point. Start
with an existing family's scene workflow, domain `*_shop.go`, and matching
widget tests rather than designing a parallel transaction framework.

### Family-Specific Traps

| Family / widget sources | Preserve when extending |
|---|---|
| Arms: `arms_shop_dialog.go`, `arms_trade_dialog.go` | Carried inventory is sale authority; READY counts are informational. Buy/Sell retain independent selection/scroll/sort. M selects the Buy maximum or the gold-safe Sell all (A remains a Sell alias); ammo is a fixed refill. Used-ammo refusal ends the session; zero-price/headroom refusals return to Sell. Capacity refusal returns to Buy; insufficient Buy funds terminate after acknowledgement. |
| Healer: `healer_shop_dialog.go`, `healer_treatment_dialog.go` | Services focus does not silently select a recipient. Individual and All intents require approval. Preserve distinct eligibility, funds blockers, and authoritative effect projections; refresh all recipients/prices after outcomes. |
| Horse: `horse_shop_dialog.go`, `horse_purchase_dialog.go` | Placement/ownership/terrain details stay out of shopping UI. Approval Escape returns to Browse (a documented Redux deviation); success/funds/stable-closure outcomes remain one-shot terminal acknowledgements. Remote Preview does not claim Live placement availability. |
| Ships: `ship_shop_dialog.go`, `ship_purchase_dialog.go`, `ship_dispatch_dialog.go`, `ship_travel_dialog.go` | The selected card uses the shared white selection bar. Parent shows trusted berth/delivery projections. Frigate loading uses aggregate quantities; standalone delivery is one. Refresh after purchase/stale refusal. First vessel creation can open dispatch; reviewed harbour escort is separate from boarding. Preview stops before purchase or travel. |
| Inn: `inn_shop_dialog.go`, `inn_action_dialog.go` | Rest/Leave/Pick Up retain mode. Ineligible rows remain inspectable with reasons; poison death and insufficient-funds guard consequences are visible before approval. Last-living-member Leave remains reviewable with warning. Approval Escape returns to Browse. Refresh complete roster/charge/result data; clear stale receipts on navigation. |
| Guild: `guild_shop_dialog.go`, `guild_purchase_dialog.go` | Browsing does not replace latest speech; pitch appears in quantity approval. Cancellation retains exchange. Maximum comes from authority and permits a saturating final bundle. Zero maximum emits no request. Session errors disable purchase until dismissal. |

All approval dialogs consume requests once. Do not normalize distinct family
refusal/termination policies into a generic modal rule. Inspect the family tests
for navigation, terminal/nonterminal refusals, duplicate suppression, and refresh.

### Merchant Quantity Controls

`quantity_input.go` shares input ownership across five approval forms; each form
retains its existing quote-derived bounds and request type:

| Form | Repeating adjustment | Existing one-shot shortcuts / limits |
|---|---|---|
| `ArmsTradeDialog` | Left/Right item count | Up/Down decrement/increment once; M selects the maximum (Sell all), with A as a Sell alias; ammunition remains a fixed refill. |
| `GuildPurchaseDialog` | Left/Right bundle count | Up/Down decrement/increment once; M selects the quoted maximum, including the existing saturating final bundle. |
| `ReagentPurchaseDialog` | Left/Right bundle count | Up/Down decrement/increment once; M selects the maximum supplied approval view. |
| `ShipPurchaseDialog` | Left/Right loaded-skiff count | Up/Down decrement/increment once; M selects the maximum supplied approval view. Standalone vessel delivery remains one. |
| `TavernActionDialog` | Left/Right ration-pack count | Up/Down decrement/increment once; M selects MaximumUsefulPacks; other Tavern approvals keep their existing controls. |

A fresh plain Left/Right press steps immediately, then waits 300 ms before nominal
120 ms repeats, quantized to Update ticks. Release stops repetition; rapid taps
and switching direction are immediate. Bounds clamp without wrapping. Shift,
Control, Alt or Meta blocks quantity adjustment rather than invoking list paging.
Conflicting quantity keys are suppressed until release; releasing only a modifier
or one conflicting key cannot replay the remaining hold. Up/Down aliases never
inherit list-navigation repeat or Shift paging.

A throttled hold still owns the update. Enter/keypad Enter pressed during quantity
adjustment must release before a fresh confirmation; holding an arrow never emits
a transaction. Escape keeps cancellation. Modal focus notifications reset repeat
and suppress held adjustment, shortcut and confirmation controls on cover, return,
dismissal or removal, even when no Update ran while covered. Application focus
suspension uses the shared keyboard's release protection. Resize preserves the
quantity and current repeat cadence. The helper changes neither pricing nor
authority: confirmation emits one existing reviewed request, and workflow cleanup
returns control with release protection.

`quantity_input_test.go` exercises injected physical-held input, cadence, aliases,
conflicts, focus suspension, removal and resize across these forms. Family dialog
tests retain bounds, fixed-quantity and single-request assertions. These tests do
not substitute for a live keyboard acceptance pass.

## Detached Selectors

`CharacterSelectionDialog` and `NewOrderDialog` share a roster/detail renderer
but keep separate input state machines. Every row is inspectable; Enter confirms
only actionable or warning rows. Status color describes the detached condition,
not eligibility or encounter possession. Owners supply action-specific facts and
availability. New Order keeps Avatar inspectable but immovable, previews both
exact positions, and returns from stage 2 to stage 1 on Escape. The footer packs
its control hints by measured width and reserves every packed row, so no hint is
dropped.

Healing and resurrection targets may supply detached `CharacterSelectionHealth`.
Those selectors use a wider roster with current/max HP, missing HP, and condition
columns for every candidate. Dead members show no recoverable-HP estimate; the
existing warning/availability rules and resource commitment remain authoritative.
The detail pane prioritizes the action explanation over a portrait in this mode.

Camping watch selection instead explicitly sets
`CharacterSelectionConfig.GuardResources`. `CharacterSelectionResources` carries
typed current/max HP (`uint16`) and MP (`byte`) values copied from the party's
getters. Construction and option access copy the resource value: an open selector
is a detached snapshot, refreshed only by reopening, and never parses free-form
Facts to obtain numbers. Supplying Resources alone does not select this layout;
the explicit flag also takes precedence over healing Health data.

The guard layout measures all names/identity markers, HP/MP fractions and
conditions, then centres a content-sized card inside the supplied map workspace.
Larger windows add space around the card, not between the resource columns.
Title/footer requirements may widen the card without spreading the column group.
Width is selected before wrapping the longest explanation, reserving stable
height across selection. Constrained screens retain the measured two-line
fallback and typography-fit policy. Every party member remains visible, including all six
members plus No One; No One has blank HP/MP/Condition cells. Complete numeric
values and condition colours survive selection and resize. Arrows/number keys
inspect and Enter confirms; there is no automatic guard choice or recovery
prediction. Poisoned remains a confirmable warning row whose scene-owned result
is cancellation; Good guards and explicit No One retain their separate meanings.
Healing selectors and New Order keep their own layouts. See
[guard confirmation](../systems/CAMPING_SYSTEM.md#guard-comparison-and-confirmation)
and `character_guard_resources_test.go` for the detached/layout contract.

`ItemSelectionDialog` receives detached `UseableItemInfo` values containing stable
identity, quantity, applicability, guidance, consumption, lore, and sprite. The
workflow revalidates inventory/context and exact combat ownership at commitment.
All owned Use-category items appear even when none can be activated. An explicit
`UnavailableReason` blocks activation independently of `AvailableNow` effect
applicability. Such rows are muted and marked `[No]` with an `Unavailable` legend;
selection keeps readable contrast. Enter retains the picker/selection and returns
to the first detail page's concrete restriction. Likely no-effect but permitted
items remain selectable with explicit warnings and expenditure text. The ordinary
view combines concise handling/current state with atmospheric lore. Long lore
fills remaining space and continues through measured paging without clipping or
shrinking text. Page counts/Space hints appear only for overflow. There is no
Action/Effect/Next/Now/Consumed form stack or separate "Why unavailable" layer.
Wearables say worn/not worn; names include item families and the artifact tab is
labelled Artifacts.

Owning/highlighting an item grants no knowledge of hidden powers, rituals, required
quest locations/objects or conversation-earned clues. This applies to lore,
restrictions, warnings and pre-use prompts. Without reliable earned-knowledge
tracking, Use-specific copy is conservative; no knowledge flags are added.
Technical catalog descriptions stay separate. Retained borrowed lore keeps its
attribution; rewritten original descriptions are not presented as quotations.
Actual deliberate-use feedback may describe what happened. Potion colour promises
no particular effect, and warning-only attempts retain their availability/costs.
Nonempty categories retain independent selection/scroll; item/category changes
reset details. Tabs/Left/Right wrap categories, arrows browse, page keys move half
a viewport, Space pages details, Enter selects, and Escape cancels. There are no
number shortcuts. See [Input Handling](../systems/INPUT_HANDLING.md) for resource
commitment and cancellation; the picker itself owns neither.

For specialized Ready, Cast, Mix, and read-only views, use the component map and
the corresponding scene workflow. Required constructor dependencies and result
polling differ by component; there is no generic result-bearing dialog type.

Ready deliberately retains its compact 20-size body. Its character name uses
the 26-size title; EQUIPPED, AVAILABLE and DETAILS are 23-size section labels. A
full-width character header shows the selected character's `GetKeySpriteIndex`
sprite at integer nearest-neighbour scale, name, and `CHAR N/count` in active-party
order (including dead members in the count). Its measured text bounds reserve the
portrait and position label. It labels the actual loadout as `Current equipment`,
with Strength use, capacity, and remaining allowance. Tab/Shift+Tab cycles living
present members with wraparound; Left/Right and 1–6 remain shortcuts. Combat locks
the acting character and omits character-switch hints. Footer hints wrap as
measured key/action pairs so a key never separates from its description. Hand
slots explicitly say
`Free` or `Occupied`; the second hand reserved by a two-handed weapon identifies
that weapon rather than appearing empty. The equipped and available panes start
at minimum shares of 27% and 38%, expanding as required by measured full identities
and row decorations; the comparison pane receives the remaining width. Do not
normalize its body to the larger main-screen font. Complete gear
rows are measured with bitmap-icon advances, marker and quantity space, and a
separate scroll gutter. Full identified equipment names remain visible using
measured content layout rather than catalog short-name fallback. Strength-budget
and replacement labels use the same full identities; two-handed status retains
its hand-status line. Arms-shop lists likewise reserve numeric columns before
wrapping complete names and scroll whole item rows. Details include range,
required ammunition and stock, catalog special properties,
and the slot or hands affected by the authoritative preview. Comparisons name all
removed items and use the authoritative resulting loadout, including invalid
Strength totals and shield Defense lost to a two-handed weapon.
Candidates show a compact `Strength budget`: character Strength, minus retained
other equipment, minus the selected item, and `Remaining` or `Short by`. Other
equipment excludes every displaced item, including both hands for two-handed
replacements. Zero-cost items show `None` in the cost column. A Strength refusal
is explained by the red shortfall alone; other refusals retain their own message.
Equipped-item inspection instead shows current Attack/Defense and `Uses N Strength`
or `No Strength required`, without removal deltas or a hypothetical budget. Availability
refusals appear once in details; the footer points to that explanation rather
than repeating it, including after a refused attempt. H switches the
one-handed target; B opens an equipment Strength breakdown. Enter equips an
empty slot or unreadies an equipped item; replacements first open a child review
card listing removed gear, resulting ratings, and inventory returns. Fresh Enter
confirms, Escape cancels the card, and held keys cannot leak into the parent.
The Ready modal owns its child through resize, Journal suspension, and disposal.
It emits a `ReadyEquipmentTransaction`; the scene workflow commits it and reports
the result through `ApplyTransactionResult`. Space (with `[`/`]` aliases) pages
overflow details without equipping, with a conditional page indicator beside DETAILS; changing
character/item or completing a transaction resets the detail page. Preview text
never substitutes for authoritative revalidation.

Ready retains one owned image of its current parent view, bounded to the union
of its panes. Unchanged frames composite that image without rebuilding text or
querying the shared text cache. Selection/scrolling, character, hand, page,
warnings, candidate refresh, and relevant live header/loadout/ammunition changes
invalidate the view. Resize and dismissal release the target; no historical
selection images are retained. The child review card is drawn separately and
does not by itself invalidate the parent. All content is still composed through
`text.Output`, and transaction revalidation remains scene/GameState-owned.

## Commands And Status Help

`CommandHelpDialog` accepts detached command entries and a `HelpStatusView`.
Tab/Shift+Tab switches Commands/Status while retaining independent scroll positions.
Status identifies only currently displayed worn items/effects using the same
artwork and tints as the rail, includes explicit empty states and current food,
and exposes no countdowns, undiscovered artifacts, or hidden actor identities.
Long text wraps and scrolls at normal typography. Scene-owned construction and
exact dismissal preserve Help's no-time/no-resource contract and nested menu focus.

## Title Acknowledgments

`CreditsDialog` accepts detached `text.CreditParagraph` content and an injected
intro-presentation millisecond source. The scene constructs the document once,
pushes one modal, and removes that exact instance on Escape. A new opening
creates fresh scrolling state. Enter/Numpad Enter do nothing, and transition
release debouncing prevents held opening/closing keys from leaking.

After a two-second reading pause, the document advances one rendered line per
two seconds. Space pauses/resumes; Up/Down moves one row; Page Up/Down and
Shift+Up/Down move a measured page; Home/End clamp to the start/end. Navigation
pauses. Short/empty documents rest at Complete; long documents stop with their
last row fully visible. Space at Complete does not restart; Home then Space does.
The fixed footer displays Scrolling, Paused or Complete and semantic key/hint
segments, separate from label/value styling.

Paragraph Outputs rewrap from retained content only when resolution changes.
Reflow preserves the current paragraph and reading fraction, or the final resting
position for a completed roll. A persistent body-sized canvas renders only the
visible rows plus edge rows, and is fractionally translated into a clipped
viewport. Header and footer remain outside that clip; Draw never advances time.
Dismissal releases the canvas. No gameplay clock, state, or audio manager is needed.

Credit-source authority and the intentional original-presentation difference
are recorded in [INTRO-0002](../gamefacts/STARTUP_MENU_AND_INTRODUCTIONS.md#intro-0002).

## Spell Browsing And Preparation

Cast, Mix's finder, and Z-stats Spells consume the same detached
`GameState.CurrentSpellCatalog` knowledge/inventory projection. Book-of-Lore
formulas are readable and selectable without a prior successful mix. Hidden
spells remain absent until learned; uncertain formulas retain their candidates.
Manual syllable experimentation in Mix does not reveal an undiscovered formula.

`internal/ui/widgets/spell_browser.go` owns shared filtering, two-line rows,
recipe vocabulary and measured/clipped rendering. Spell names and short effect
labels use the normal resolution-scaled 23-size body font; headings use 26.
Prepared counts occupy a separate labelled column. Cast adds current-caster
validation and a fixed action-status area; Z-stats retains portraits and
discovery provenance. Overflow details page instead of reducing font size.
Two-line rows centre their measured text block; rendering, paging and visible
capacity use the same row pitch. Cast/Mix allocate 40% to lists and 60% to details,
reserve a separate scroll-indicator gutter, and size the footer from its wrapped
hint content. Keys/actions and labels/values use separate semantic-colour
segments. Ingredient check marks have distinct normal and selected-row colours.

Z-stats keeps its separate title and eight full section labels in a fixed two-row,
four-column grid at the configured body font size. Tab/Shift+Tab traverses the
row-major order. The Spells title stays **Spells**; unread discoveries retain their
individual **New:** row markers without acknowledgement during browsing.
The standard dossier expands vertically to pay for both the larger header and
the full-width spell input strip, preserving usable list/detail space and page
capacity at every supported resolution. Explicit caller placement remains binding.
Its outer dimensions stay stable across sections and input modes; other sections
use the additional body space. Cast and Mix retain their own original placement.

Z-stats equipment and owned-item rows use full catalog identities with measured
wrapping. Bitmap-icon advances, first-line quantities and the scroll gutter are
reserved before name layout; subsequent physical lines retain the rest of the
name. Equipment rows are uncounted, with armory available/readied counts in details.
Refreshing a reordered owned-item projection retains selection by typed identity.

Carried moonstones supply explicit optional phase identity, including a present
phase-zero New Moon. A small phase glyph accompanies each Items row. Their detail
header uses a compact framed glyph beside the full name and category/quantity,
approximately two text lines high. It replaces the moonstone's large portrait
allocation rather than enlarging to fill available pane space. Whole-number pixel
scaling, nearest-neighbour rendering and the visible New Moon outline match the
shared journal glyph path. The header stays fixed while overflowing lore pages
below it. Other catalog and character portraits retain their normal layout.

The shared two-row input strip sits above both spell panes. Its input, filter, and
right-aligned **Matches: N** fields have independent measured bounds and Output
buffers. Long queries expose the latest characters and cursor with a leading
`...` marker; display clipping never edits the stored query. Empty results keep
the strip visible and replace selected details with an explicit no-match state.

Cast/Mix detail paging is measured from wrapped content and final pane bounds,
including headings, Cast action status/explanations, and the wrapped footer.
Layout first measures without a paging hint; genuine overflow then reserves the
indicator and conditional hint together. Multipage details show **Page X/Y** and
advertise **[Space] Next detail page** only in browse mode. Selection, refresh,
mode changes, and resize recompute this before drawing, keeping the page, hint,
and displayed rows synchronized without shrinking typography.

- Type syllable initials, or `/` to search names, effects and descriptions.
- `Ctrl+O` cycles All known, Prepared, and Mixable in either input mode, retaining
  the selected identity when it survives filtering. Mixable requires a confirmed
  recipe, ingredients for at least one mixture, and room below the 99-spell cap.
  The displayed mixing capacity includes stock headroom; the manual preparation
  maximum still follows reagent availability and previews discarded overflow.
  Cast list colours reflect authoritative caster readiness; Mix finder colours
  reflect mixability. Unavailable names are dimmed and marked `[No]`, matching Use
  and Ready. Details explain unavailable states in text. Escape leaves
  search before closing the owner. Navigation owns held arrows/page keys even
  while repeat-throttled. Space pages overflow details in syllable/browse mode;
  while searching it inserts a space into the query, so Escape returns to browse
  mode before paging. Leaving search retains the selected spell identity, clearing
  an incompatible older syllable prefix if necessary. Recoverable Cast prerequisite
  refusals stay in the browser with warning text; they do not spend resources or
  complete its command. Combat hides the unavailable Mix hint.
- `Ctrl+M` in Cast or Z-stats opens preparation for the selected identity.
  Widgets emit an intent and suspend input while the owning scene/workflow
  handles Mix. Refresh retains filter, selection and caster. Combat refuses Mix.
- Direct Mix opens the finder. Enter chooses a spell without consuming anything;
  Enter in preparation emits a `MixRequest`. Known formulas are preselected,
  but ingredient toggles remain available for experimentation.
- Preparation digits edit quantity. When maximum is below ten, each valid digit
  replaces the previous value. Otherwise two digits accumulate and a third
  starts a replacement. Over-limit digits are rejected, not silently clamped.
  Backspace/Delete restore one; zero maximum disables mixing. Both number rows
  work and separate rapid presses are accepted without movement-repeat delay.
  The preparation field displays its editable quantity, teaches typing digits
  before Enter, and labels confirmation with the actual amount (`[Enter] Mix 5`).
- `Ctrl+B` reopens the finder, preserving the unfinished preparation if cancelled.
  A successful standalone mix returns to the finder; a shortcut mix returns to
  its originating screen after the receipt. Mix previews ingredient cost and
  stock overflow but the authoritative operation revalidates before mutation.
- `Ctrl+F` in preparation fills the quantity with the maximum confirmed-recipe
  amount that fits both current ingredients and the remaining 99-spell stock
  capacity. It never mixes directly. Changed/unconfirmed recipes, missing
  ingredients, or full stock retain the draft and explain the unavailable action.
  Manual quantities and experimentation retain their existing rules.

`cmd/ultimav/gamescene_spell_browsing.go` owns the shared awaited Mix sequence,
receipts, trap resolution, exact modal cleanup, catalog refresh and command-cycle
handoff. See [Input Handling](../systems/INPUT_HANDLING.md#spell-browser-mix-handoffs)
for timing. Regression entry points are `spell_casting_search_test.go`,
`reagent_mixing_dialog_test.go`, `ztats_catalog_regression_test.go` under widgets,
and `cmd/ultimav/gamescene_spell_browsing_test.go`.
