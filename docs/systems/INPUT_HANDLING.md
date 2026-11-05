# Input Handling System

Gameplay input is routed by `GameScene.Update` through a primary/secondary state machine. Primary input chooses a command; secondary input owns subsequent ordinary direction, tactical-target, or specialized retained input until that input contract completes.

## Keyboard mechanism boundary

`pkg/input.Keyboard` remains the game-owned concrete adapter. It supplies
`ebiten.Key`, live physical state and explicit per-query elapsed milliseconds to
[Berad Input](https://github.com/bradhannah/berad-sdk/tree/891cf44f456a/modules/berad-input).
The SDK owns consumption/suppression, release barriers, captured edges, ordered
binding inspection and cadence state. It does not import Ebitengine or the game.
Redux retains command routing, arrows/keypad/semantic conversions, Caps Lock
exclusion, modifier/chord policy, native text/IME and the historical simulated-key
compatibility path. No mouse input is introduced.

There are four independent scene keyboards: GameScene (165 ms), Intro and setup
(250 ms), and character creation (0 ms). Widgets borrow their scene keyboard;
list/quantity owners retain separate repeat state. Release cleanup still runs
before modal early returns in all four owners. GameScene alone captures a detached
edge sample at entry, before quickload, readers and queues. Capture consumes
nothing and does not turn later queries into snapshot-only polling. Scene
activation barriers inspect live holds before the incoming scene's first Update.

This extraction preserves timing rather than normalizing it: legacy acceptance
uses strictly greater elapsed time, a different key bypasses the shared interval,
and force-next never bypasses the all-key deadline or physical gates. The input
clock stays independent of paused Journal/Atlas gameplay time. List repeats use
focused-update ticks (300/120 ms), while exploration Pass retains its scene clock,
eligibility and no-catch-up policy. First-held binding selection is distinct from
first-eligible selection; ordinary consumption does not silently change priority.
Intro uses a copied SDK binding registration; existing slice-based callers retain
their facade entry point.

One-shot consumption clears only after observed physical release. Stronger
suppression also blocks repeats and survives transition reset. Masked input is not
release evidence; restoring focus protects all current holds. Status reads never
advance release state. Existing dummy `TryAcceptKey` calls still service release,
update the shared timestamp and apply a pending post-accept delay; ordinary
registration does not apply that delay. Synthetic input retains its historical
method-specific timing/gate bypasses and is not a physical edge sample.

Regression ownership: SDK public/independent-consumer tests establish mechanisms;
`pkg/input/compatibility_test.go` protects legacy facade behavior, and
`cmd/ultimav/keyboard_owner_test.go` exercises preactivation and modal early-return
release servicing in all four scenes. Reader-clock, focus, quantity, list,
quickload and combat-buffer suites retain game policy coverage. This is an
implementation-boundary change, with no intended player command/debounce change;
physical keyboard/IME and ordinary published-dependency installation remain
separate acceptance from injected-source candidate testing.

## Routing Order

Closing the game window during play opens the Exit to DOS choice instead of quitting; a second close while that choice is open,
or any close outside play or after the game has ended, quits at once.

Caps Lock is transparent to game-control polling: the shared Keyboard adapter
excludes it before native/injected polling, including nil-receiver fallback. It
cannot acknowledge all-key prompts, count as intro activity, hold release gates,
or enter focus-return suppression. Real held controls still require release.
Native committed text and composition keep their existing case policies.
**Shift+M/Shift+J require physically held left or right Shift**, with existing
context/other-modifier rules. While a text box has focus, Shift+J types a capital J
instead of opening the Journal. Caps Lock alone routes M/J to Mix/Jimmy; Caps Lock
plus physical Shift opens readers even if native text is lowercase. Native
save-description Ctrl+U accepts either ASCII shortcut insertion (`u`/`U`) without
changing composition ownership. Injected held-key and production reader round-trip
tests cover these contracts; physical-keyboard/OS acceptance is separate.

The entry-frame physical sample reserves modified Q. A fresh eligible Ctrl+Q
reload returns immediately, before clocks/queues or deferred update-tail work;
an unavailable chord is suppressed through release. Its ownership policy is
described in [Q quick save and Ctrl+Q quick load](#q-quick-save-and-ctrlq-quick-load).

An active Traveller's Map session has the earliest frozen-reader path, before
all gameplay updates below. Fresh exact Shift+M and Ctrl+N requests are checked
against the safe openers described in [Traveller's Map](#travellers-map).

After released-key consumption cleanup, Ctrl+R is reserved for the passive Runic
reference (including release protection). An existing scene-owned Journal session
is then checked before finale input, wind/effects, event queues, automatic
doors, or gameplay dispatch. Only modal UI and audio presentation update;
even the dismissal frame returns without advancing play. The shared map/journal
notice owner samples entry/exit blockers, including early atlas returns, so
pending ordinary Output notices wait until command and reader ownership ends.
Search, tab changes, and loss of top-widget focus do not release the session's
[full gameplay pause](TIME_SYSTEM.md#journal-pause).

Outside that session, each frame follows this ownership order:

1. Capture one detached physical-key edge sample before any owner advances;
   after reader/shortcut precedence, admit an eligible early all-party reset
   before capturing waiting combat intent.
   Then update event queues, effects, and non-blocking UI.
2. Let an active auto-open animation or rising-moongate lock own the frame.
3. Let the top modal dialog consume input exclusively. Admit a sleeping-only
   camping Space request after rechecking its exact owner, then honor the workflow
   block for all ordinary commands.
4. Present pending ordinary battle summaries after exit/recovery workflows finish;
    then resolve pending overworld/dungeon encounters, combat turn transitions, and NPC auto-talk before player input.
5. Run `ResumeWaterfallContinuation` after contact/scheduler/auto-talk handling.
   It preserves owned tactical input but resumes a retained surface cascade before
   any ordinary direction, movement or Pass. See
   [waterfall ownership](EVENT_QUEUE_SYSTEM.md#retained-waterfall-cascade).
6. Accept a still-valid pending combat verb through its normal handler before
    ordinary movement; if `secondaryKeyState != PrimaryInput`, route only to the
    current map's secondary handler and return.
7. Otherwise read and debounce a bound key, intercept `/` (also shifted `/`, `?`)
   Help, F4 last-battle review, and global Shift+J journal routing, process active-player number keys, and call the large-, small-,
   dungeon-, or combat-map primary handler.

In combat, selecting an active player enables solo mode without removing any
combatant from the authoritative DEX queue. Non-selected player-loyal turns
still resolve their timing, duration, ring, field-expiration, and standing-field
effects, but the scheduler scans past their generic auto-passes synchronously.
The scan stops for the selected actor or a hostile turn. Condition-driven
non-actionable turns, such as sleep, retain their dedicated presentation and
feedback.

AI-controlled bats and mongbats use a scene-only movement profile: at 1x, 75 ms
no-target anticipation and 175 ms walking interpolation. Both durations capture
the validated movement-speed factor together when the workflow starts. Targeted
attack anticipation, projectile/signature feedback, ordinary enemies, player and
controlled-monster movement retain their own timings. Actual resolved movement
determines interpolation; idle/blocked/teleport outcomes do not receive a walking
tween. Exact token completion remains after presentation. The pacing profile adds
no gameplay RNG draws and changes no AI, DEX-order, possession-counter, or calendar
cost rules. Isolated queue tests compare outcomes with controlled RNG inputs;
production wind runs before queues and shares the gameplay RNG. Different elapsed
waits can therefore admit different wind draws and change subsequent outcomes.
See [elapsed-time and shared RNG scope](TIME_SYSTEM.md#elapsed-time-and-shared-rng-scope).

Dungeon primary input implements traversal, facing, turnaround, contextual
rejection, and the production Attack, Cast, Get, Hole Up, Ignite, Jimmy, Klimb,
Look, Mix, New Order, Open, Quit, Ready, Search, Use, View, and Yell workflows.

## Native Tavern Subject Ownership

The Tavern's eligible Conversation/Browse Subject owns a native text session,
compatible with the save-slot description editor. Ending the save session does
not guarantee that macOS resumes `AppendInputChars` delivery. Production Tavern
construction leaves `AppendInputChars` nil and uses its own native field; physical
letter keys are never translated into text. This preserves Unicode, dead keys and
IME input independently of purchase shortcuts.

`DialogStack` grants only its top modal focus. The Tavern listener retains list
navigation's release reset and releases native focus when covered, hidden,
dismissed, window-unfocused, in Order, ineligible, or awaiting quote/approval/result
or error acknowledgement. Each eligible Update pumps native input before merchant
speech paging, mode controls, local editing and topic submission. Composition and
handled native control presses remain owned through release, including Enter
confirming a candidate and Escape/Backspace editing preedit. Bounds and selection
are derived from the displayed Subject, including multibyte and long preedit text.

If the scene omits the Tavern Update while input is suspended,
`suspendNativeDialogInput` calls `TavernShopDialog.SuspendTextInput()` before
any reader-owned early return.
The hook retains top-modal ownership, releases the native session without pumping
input, preserves the committed draft/byte selection against late Blur commits,
and resets local holds. A returning held control must release before editing can
resume; no second resume hook is needed. Calling it under masked keyboard polling
must not reopen focus or rearm controls through another presentation transition.

Subject submissions retain the existing 15-printable-rune and whitespace rules;
the workflow receives the exact committed text. Text-service errors visibly end
editing without emitting a topic intent. Detached tests may explicitly supply
`AppendInputChars`; that adapter is a per-frame snapshot, not a drainable queue.
Use the same `Keyboard.IsKeyHeld` source for the Tavern's `IsKeyPressed` release
checks. See the [widget contract](../ui/WIDGET_COMPONENTS.md#tavern-subject-native-session)
for lifecycle, editing and regression entry points.

Atlas note fields and character-name entry also own native sessions rather than
depending on raw character delivery returning after a save or Tavern session.
Atlas keeps its 32-character ASCII label and 500-character multiline description
policies. Each field retains its draft and selection across Tab changes; the
top-modal editor releases native input before an unfocused reader frame returns.
Character naming keeps its eight-ASCII-letter policy, pumps composition before
controls, and releases native ownership on phase change, cancellation and scene
replacement. Later creation choices use their ordinary physical-key routes.
Native-session handoff tests model the separate character channel; they do not
replace native platform keyboard/IME acceptance.

## Input States

### Merchant quantity adjustment

Merchant parent and child release gates use the scene keyboard's `IsKeyHeld`
source, including Ship, Inn and Tavern approvals. Native gameplay therefore
shares one physical-key owner, while detached fixtures can exercise release and
modifier boundaries without consulting the host keyboard.

The Arms trade, Guild purchase, Reagent purchase, Ship purchase and Tavern ration
approval forms share plain Left/Right quantity input: one immediate step, a
300 ms initial hold delay, then nominal 120 ms repeats quantized to widget updates.
Release stops repetition; fresh taps and direction changes do not inherit gameplay
movement delay. Modified or conflicting quantity keys are suppressed through
release. Guild, Reagent and Ship retain Up/Down as one-shot aliases; maximum/All
shortcuts also remain one-shot. Quantity holds never confirm a purchase, including
Enter pressed during an adjustment. Covering, returning to or removing a modal,
and application focus suspension, require held controls to release before use by
the next owner. Quote bounds and transaction commitment remain family-owned. See
[merchant quantity controls](../ui/WIDGET_COMPONENTS.md#merchant-quantity-controls).

### Exploration Pass repeat

Space passes immediately, then repeats after 300 ms and every 120 ms while held
at idle exploration input: towns, overworld, Underworld and first-person dungeon
corridors. Each due key follows the same ordinary map handler and completion
owner as a single press. Combat never repeats Pass. The injected scene elapsed
clock controls cadence; one Update emits at most one Pass, with no backlog after
a hitch. Release stops immediately. Readers, modals, blocking workflows, focus
loss and location/floor changes cancel the hold and require a fresh press.
Space cannot bleed from a repeat into another prompt or command owner.

### Sleeping Space

Outdoor and dungeon Hole Up retain their workflow input block. Only a fresh
unmodified Space edge sampled during logical sleeping can request acceleration;
setup/Bard holds are suppressed through release. The post-queue check rejects a
request if the same Update completed or interrupted sleep. Acceleration is silent:
it adds no hint/status box, message, sound, or acknowledgement.
Additional fast-sleep presses are consumed,
and raw any-key waits keep their release-before-press contract. No command cycle,
calendar charge or RNG draw belongs to this input intent. See
[camping acceleration](CAMPING_SYSTEM.md#on-demand-sleeping-acceleration).

### One pending solo combat verb

While the selected ordinary party character waits for intervening enemy turns,
fresh **G/O/J/P/S** may retain one Get/Open/Jimmy/Push/Search verb. Further
supported edges replace it immediately, even while an earlier key remains held.
Capture and replacement are silent: there is no pending-status popup, map overlay,
queue hint or acknowledgement. Acceptance opens the ordinary command prompt once
the selected character is ready. Waiting neither reveals enemy information nor
pauses their workflows. This scope excludes all-party mode, controlled monsters,
ambiguous clone selection, committed player-action presentation, retained targeting,
readers and modals.
Modified shortcuts retain precedence; Shift+J opens Journal and clears intent.

Escape clears without cost and writes one `Pending command cleared.` line to
ordinary Output. A plain unsupported gameplay letter, Space/Pass or Tab/Auto
clears with one ordinary Output explanation and is consumed through release.
These one-off cancellation/refusal messages are not persistent queue feedback.
Directions entered while waiting retain the verb but are suppressed through
physical release, including cardinal keypad keys and the repeatable movement
path. Clearing or invalidating the verb does not release an old arrow. With no
pending intention, unrelated input keeps its existing behavior.

One edge sample precedes wind/queues. After the early all-party reset has had
precedence, buffer ties have no chronological ordering: Escape wins, then an
unsupported action, then the alphabetically first supported verb. All losing
action edges are consumed. A blocked-entry arrow
stays early even if that Update finishes the last enemy. At an already-ready
boundary a fresh direction reaches the installed secondary prompt in the same
Update, without an extra release-all delay. Normal Escape cancellation and paid
Space Pass apply after acceptance; the buffer is already empty.

`ProjectSoloCombatInput` reads exact encounter, queue entry and an entry-local
dispatch generation. Waiting binds to that entry's next generation; ready input
matches the same generation after actual dispatch. The authoritative queue
increments it before turn-start rules, including skipped opportunities. Input
never forecasts countdowns, plans future actions or claims a scheduler review.
Enemy removal, DEX changes and Time Stop suppression of intervening enemies do
not expire the selected character's pending opportunity. Death, departure, sleep,
captivity, possession, ambiguous clone selection, selection/control changes and
encounter replacement still invalidate; a skipped selected opportunity cannot
replay on its later recovery. Scene teardown/load and application focus
loss clear intent. Production remains runnable without focus:
sky synchronization, wind/queues, committed automatic-door phases, effects and
audio service continue on the ordinary update path outside retained reader pauses.
The keyboard is temporarily masked and raw acknowledgement waits retain a real
release-before-press requirement across focus return. Animated Message/Overlay
dialogs continue their presentation without accepting keys; other modal input
updates remain suspended. Journal and Atlas keep their full gameplay pause.
These fields and release guards are transient and never saved.

The two combat presentation workflows use a narrow release guard that permits
handoff over keys already suppressed until physical release. This does not alter
raw `WaitForAnyKey` or camping release-before-unblock behavior.

### Early all-party combat reset

A fresh unmodified **top-row 0** can clear solo selection during a scheduled
enemy turn or condition-driven auto-pass, even when the selected party member is
sleeping, captive or possessed. `captureCombatSoloReset` runs before pending-verb
capture and wind/queue advancement. It validates the exact scheduler boundary and
its scene owner: an unclaimed scheduled step with no competing workflow, or only
`monster_turn_delay` / `combat_pc_auto_pass` after presentation claims it.

The reset clears the future solo filter and pending G/O/J/P/S verb. It preserves
the current actor, completion token, target, countdowns and in-flight action;
that owner still resolves and completes once. Future actors follow the existing
DEX schedule, including intervening enemies. It grants no extra action, refunds
no completed skip, and itself charges no time, resources or RNG. Original
out-of-turn keyboard admission is not established; this Redux convenience is
scoped in [COMBAT-0003](../gamefacts/COMBAT.md#combat-0003).

Zero wins over buffered verbs in the same sample. Losing verbs and early held
directions remain suppressed until physical release; held 0 cannot delay the
workflow's release. Already-all-party reset is silent and idempotent. Readers,
modals/text editors, retained targeting, focus barriers and player-owned committed
action presentation retain their input. This adds neither early 1–6 selection nor
keypad-0 admission. There is no delayed reset request to replay at a later boundary.

### Automatic sailing interruption

The automatic sailing owner checks input before each 55 ms cadence phase; normal
large-map dispatch also catches short non-direction commands between phases.
Space stops with `Sheets in irons!` and is consumed. Other commands stop automatic
travel and retain their own normal dispatch and completion. Furl stops sailing
before its feedback. Tactical entry stops and furls; successful outdoor enemy
ranged hits stop automatic travel while preserving a surviving frigate's sails
and facing. Sinking stops before fallback replacement or drowning.

`AutomaticSailingStopEpoch` lets the scene clear the sailing camera and suppress
held arrow/cardinal-keypad movement before input and after queue updates, including
projectile preemption. Movement must release and be pressed afresh; Fire, Yell and
other fresh commands remain usable once their ordinary input boundary is available.
A live or finalizing `automatic_sailing` name prevents a second start. Stopping
does not discard already owed actor work or projectile presentation. See
[sailing timing](TIME_SYSTEM.md#automatic-sailing-boundaries) and
[SHIP-0001](../gamefacts/SHIP_MECHANICS.md#ship-0001) /
[SHIP-0005](../gamefacts/SHIP_MECHANICS.md#ship-0005).

### Traveller's Map

Live `UI.AtlasUpdated` feedback never owns input or changes atlas selection. One
scene FIFO coalesces pending updates for the same public site and keeps pending
map updates ahead of the coalesced journal notice. At an unblocked boundary it
flushes into the ordinary main Output, with styled Shift+M / Shift+J reader hints
and normal History retention. Secondary prompts, readers, modals, workflows,
arrival presentation and fades defer the flush; both entry and exit of the frame
must be clear. There is no timed map overlay, countdown, or notice dialog.
Notices add no save restriction, game time, RNG work or persisted queue. Scene
replacement/disposal clears pending notices; hydration does not replay them. See
`gamescene_journal_notice.go` and `gamescene_map_notice_test.go`.

`gamescene_atlas.go` admits exact Shift+M only at idle PrimaryInput on actual
large, small or dungeon maps, including read-only surface-chart access from the
Underworld. Combat, camping, virtual scenes, workflows, modals, pending battle
summaries/encounters, auto-open transitions and input locks reject opening. NPC
auto-talk retains precedence. The accepted route precedes camera clearing,
smashed-command interception, ordinary command dispatch and dungeon charging.
An unavailable exact chord is consumed through M release without falling into
Mix or secondary command input; other modifiers retain existing routing.

Exact `Ctrl+N` (either Control, no Shift/Alt/Meta) uses the same safe-primary
opener, restricted to Britannia's surface overworld. It opens the existing atlas
and immediately requests Mark Current Position through its ordinary note intent
and `BeginAtlasNoteDraft(0)`. The draft captures the current eligible sextant fix,
never the camera or selected place. A note already at that position opens for
editing. A missing sextant or unusable fix leaves the atlas open with the
existing explanation and no editor. The shared rule requires a carried sextant
at 19:00–05:59, or daytime with Flexible sextant use enabled; that preference does
not expand the shortcut beyond the surface overworld. An unavailable request is
not deferred. N stays reserved until release even if Ctrl releases first, so it
cannot fall into New Order or secondary input. Fresh plain N retains New Order
in exploration.
Command Help describes the shortcut and its surface/fix requirements. See
`cmd/ultimav/gamescene_atlas_mark_shortcut.go` and its `_test.go` coverage.

The scene owns the exact AtlasDialog instance and freezes elapsed time, calendar,
RNG, wind, effects and all event queue lanes. Only the top modal updates while
the session exists. It refreshes detached public evidence and current sextant
eligibility without adjusting the manually chosen camera. Opening/closing keys
must be released; both dismissal and final release frames return without a
gameplay tick. Resumption resets elapsed/presentation bases, so paused wall time
does not catch up. Scene disposal removes the exact atlas and releases its pause;
the application-owned geography texture survives. Atlas reading never changes
Journal acknowledgements. Controls and rendering ownership are documented in
[Widget Components](../ui/WIDGET_COMPONENTS.md#travellers-map).

After opener release, Up/Down selects in the visible Places list. Tab and
Shift+Tab change the visible place category (with release-based single-tap
handling), not input focus. `P` enters/exits the explicit local Pan mode, in which
plain arrows pan while preserving place selection and the header/footer identify
the mode. Category cycling remains available in Pan mode; held arrows must be
released across a mode switch. Opening resets to Places. There is no Control-arrow
requirement, filter submenu or Space selection mode. Number-row/keypad `0` resets
the world view; no Home key is required.

Each specific category (not All) has a selectable `All mapped (N)` overview row,
independent of individual selection. `L` shows/hides all authorised labels in overview or the
single selected label in ordinary selection. `N` requests a note at the current
eligible sextant fix; `E` edits a selected note and `D` requests its deletion.
The exact scene-owned child editor remains within the atlas pause. Tab/Shift+Tab
changes its label/description field, Enter inserts a description paragraph,
Ctrl+Enter saves, and Escape discards. Delete confirmation uses Enter/Escape.
`GameState` validates the captured draft and owns the mutation; cancel, teardown,
or state replacement discards the draft and removes the exact child before its
parent. Save or cancel returns to the underlying atlas, including after Ctrl+N;
held child-dismissal keys cannot also dismiss the parent. Only final atlas close
and key release resume play. Opening, editing and closing add no gameplay time,
RNG draws or resource costs. Neither editing nor deletion invents another current fix.

### Combat report and completed battle review

At primary combat input, F3 reviews the oldest pending pickup/discovery/trap
result, or the latest noteworthy result when none are pending. PgUp/PgDn/Home/End
retain their history navigation; End returns live. F4 at primary input opens the
last completed battle summary in the current scene/session. These controls spend
no action, resources, gameplay time, or RNG, and never intercept retained targeting
or a modal's input. The automatic departure summary uses the same bounded reader
and requires fresh Enter/keypad Enter/Escape to continue. See
[combat loot feedback](../ui/GAME_UI_INTEGRATION.md#combat-loot-feedback) for history,
attention, accounting, and lifecycle contracts.

### Runic reference

`Ctrl+R` toggles the full right-rail alphabet during ordinary surface/dungeon
exploration, Sign Look, and semantic NPC Talk. The source remains in its existing
left-side presentation; the chart pairs the existing Runic font with Latin A-Z
and the authored TH, EE, NG, and ST forms. It does not decode the current text.

The panel is passive, never takes dialog focus, and permits conversation typing,
progression, and exploration while open. The toggle spends no gameplay time or
resources and does not start a dungeon command cycle. Visibility is transient,
initially closed, and not saved. The exact sign/Talk dialog becomes its lifetime
owner, even if the chart was opened before that dialog; removing the owner closes
the chart. Other modal interfaces temporarily hide it. Combat, camping, cinematic
layouts, View, and Journal close the scene-owned right-rail chart.

The Journal handles Ctrl+R locally when reading a retained Runic exchange. It
draws the same passive alphabet in its list pane beside the passage. Escape or
Ctrl+R closes that reference first without changing the selected exchange/page;
tab, search, or exchange changes close it. No gameplay time or knowledge changes.

Outside the Journal, central routing reserves Ctrl+R before modal or primary/
secondary input, including when the reference is unavailable. The chord remains intercepted until R is
released, so releasing Ctrl first cannot type R, Ready equipment, or dismiss a
raw press-any-key prompt. Escape retains the reading workflow's normal behavior.
The shortcut is listed in command Help.

The sign-specific maintainer translation chord and its release guard are
documented in [SIGN-0008](../gamefacts/SIGNS_AND_FONTS.md#sign-0008).
It is not part of player Help or a conversation translation facility.

Implementation: `cmd/ultimav/gamescene_runic_reference.go` and
`internal/ui/widgets/runic_reference_panel.go`. Injected-input and responsive
layout coverage live in the corresponding `_test.go` files; physical-keyboard
and visual acceptance remain separate from those automated assertions.

### Command help and retained prompts

Combat status Help explains enemy possession's own-completed-turn resistance,
intelligence adjustment, guaranteed twelfth-turn endpoint, and An Xen Ex or
controller-removal alternatives. Sleep/captivity and temporal suppression still
count at the scheduled boundary. Help and report browsing never advance that
counter or draw recovery RNG; the exact actor/episode completion ticket belongs
to GameState. Chaos Sword control remains untimed. See
[COMBAT-0005](../gamefacts/COMBAT.md#combat-0005).

Help also appears in Escape. It lists context-specific command purposes, next
inputs, unavailable reasons, and Shift+J without puzzle answers. It is a detached
keyboard-only modal: opening/browsing/closing adds no gameplay time, RNG draw,
or combat action and does not begin a dungeon cycle. It cannot intercept a
secondary prompt or an input-owning workflow. Physical-keyboard acceptance
remains separate from injected-input coverage.

`InputState` is defined in `cmd/ultimav/gamescene.go`.

Standard directional states cover Open, Jimmy, Klimb, Push, Get, Look, Talk,
Search, Attack, and Fire. Use has no directional state; item workflows own Use
input. Combat adds
persistent target states for physical attacks, square-target spells, cone spells,
and adjacent spells, plus `CombatKlimbLadderDirectionInput` for the original dual-ladder
`U/D` choice. Specialized non-directional contracts can also retain secondary
routing, including `DungeonQuitInput` and `SurfaceQuitInput`.

Cast, Yell, Use menus, Ready, and other modal workflows do not require new directional states when their dialog or event workflow already owns input. Popup-based directional workflows use `thenShowDirectionPrompt` instead of the command state machine.

Hole Up's guard selector returns cancellation separately from the explicit
**No One** row. Escape cancels setup and removes the selector without starting
camping; No One confirms an unguarded camp. Surface cancellation has no ordinary
completion cycle. Dungeon cancellation retains its existing precharge and exact
`DungeonFullEndPlayer` cleanup, without constructing a camping plan or consuming
camp-setup RNG. See [Camping entry](CAMPING_SYSTEM.md#entry).

The guard selector explicitly opts into `GuardResources`, showing detached
current/max HP and MP for every party row and blank resource cells for No One.
Arrows and number keys inspect; Enter confirms. It does not automatically choose
a guard from those values. A Poisoned row is confirmable with the warning that it
**camps without a watch**; the scene rechecks Good status and turns a non-Good
selection into No One, following the original rule in
[CAMP-0001](../gamefacts/CAMPING_MECHANICS.md#camp-0001). Escape cancels setup,
a recorded deviation from that rule.

Cast gathers required targets/directions before final confirmation spends MP and
one mixture. Cancellation before that boundary preserves resources, gameplay
time, combat action, and gameplay RNG, including party-member target selection.
Later effect failure does not refund committed costs.

Interaction transcript text prompts normally use Escape to cancel. Blackthorn
alone configures `ClearTextOnEscape`: `[Esc] Clear text` clears the editor without
submitting an answer, advancing the attempt, or releasing interrogation input
(the interrogation belongs to [CRIME-0003](../gamefacts/CRIME_PRISON.md#crime-0003)).
Return, including an empty Return, still submits an answer.
Shrine meditation empty submissions or cancellation skip remaining prompts and
unfocused feedback, then perform normal walk-out/visit cleanup. Nonempty wrong
mantras retain all three prompts; the existing early rejection of a nonempty
wrong virtue is unchanged.

## Dungeon Commands

Dungeon exploration currently accepts these primary keys:

| Key | Action | Command-cycle completion |
|---|---|---|
| Up | Advance one wrapped cell | Full end-player processing, including blocked attempts |
| Down | Back up one wrapped cell | Full end-player processing, including blocked attempts |
| Left / Right | Turn left / right | Command-only, including refused doorway turns |
| Return / `.` | Turn around | Command-only |
| Space | Pass | Full end-player processing |
| `A` | Attack the wrapped facing cell | Command-only or retained tactical combat handoff |
| `B` / `E` / `F` / `T` / `X` | Source-defined contextual response | Full end-player processing |
| `D` / `W` | `D-What?` / `W-What?` | Command-only |
| `C` | Retained prepared-spell workflow | Route-dependent full completion |
| `G` | Take an open current-cell chest | Full end-player processing |
| `H` | Retained exact-cycle camping workflow | Full completion after rest or ambush restoration |
| `I` | Ignite | Full end-player processing on success or no-torch failure |
| `J` | Retained current-cell chest Jimmy workflow | Full completion after actor selection and resolution |
| `K` | Klimb from the current cell | Route-dependent; see below |
| `L` / `S` | Retained actor and relative-direction Look / Search | Full completion after workflow cleanup |
| `M` / `N` / `R` | Mix / New Order / Ready modal workflow | Full end-player processing after workflow cleanup |
| `O` | Open the current packed cell | Full completion after optional actor selection |
| `P` | `Push` / `Not here!` | Command-only |
| `Q` | Save to the active player slot; optional Save game? Yes/No prompt | Command-only before capture, after the answer when confirmation is On |
| `U` | Retained item workflow | Full completion; supported Sceptre/View routes commit authoritatively |
| `V` | Gem symbolic View | Retained through dialog dismissal, then full completion |
| `Y` | Retained editable Yell input | Full end-player processing after input cleanup |

Each accepted key begins one exact-once dungeon command cycle. Ordinary commands
apply their precharge before the action. Cast and Use reserve the cycle but defer
its charge until final confirmation, making their rejection and cancellation
free. Charged cycles use one gameplay minute; Quickness alternates charged and
free cycles, and Time Stop suppresses the charge. Completion never calls
`FinishTurn` or callback-owned time, so the charge is not duplicated.

Klimb starts one cycle and precharges once. A missing route or grapple-only
opening without a grapple completes command-only. A single route starts an
input-blocking floor workflow. A dual route retains the same token in
`KlimbDirectionInput`: Up/`U` selects ascent, Down/`D` selects descent, Space
passes, and unrelated keys leave the prompt and token active without another
charge. Pass, a blocked selected route, and successful internal travel use full
end-player completion. Boundary exits consume the token without dungeon
end-player processing and transition to the route-selected world plane.

Remaining unsupported bound keys that reach the dungeon primary handler report
`What?` and do not begin a command cycle or charge its
minute. Party-selection numbers and Escape retain their global routing, while
`Ctrl+M` toggles music without a dungeon cycle. A stale Look, Search, Klimb, or
Quit secondary state is cleared without action or charge. Open, Jimmy, Search,
and Look retain their command token through character and directional input.
Mix, New Order, and Ready likewise retain one token through success, rejection,
and cancellation. Immediate validation rejection completes synchronously;
started workflow finalizers own exact-once full completion after cleanup.

Shift+J is intercepted before queues, modal input, and primary handlers in
exploration, conversations, retained prompts, workflows, dungeons, and combat
including targeting. Terminal finale presentation is excluded, and so is a
focused text box (typed Yell, conversation keyword or text prompt), where Shift+J
types a capital J. It opens the detached Quest
Journal without beginning a dungeon command cycle, calling `FinishTurn`, or
finishing a combat action. Plain J continues to reach the existing Jimmy path.
The top modal isolates Tab/Shift+Tab, repeatable Up/Down and Page Up/Page Down,
Home/End, Space detail paging, `/` Journal search, Escape, and Shift+J.
Curated page keys move selection a full viewport. The seven-section Journal no
longer exposes main-output History through tabs, navigation or search.
Search owns text input, Enter opens a matching Journal destination, and Escape
returns to the previous tab without closing Journal. Shift+J closes from either
mode. `PrepareForTransition` prevents the opening or closing chord from bleeding
into the next input owner. The pause lasts until closing keys are released, so
raw workflow any-key waits cannot consume a held dismissal key. Covered message
fades rebase their wall-clock start on resume. See the
[widget contract](../ui/WIDGET_COMPONENTS.md#questjournaldialog).

During semantic NPC Talk, Tab keeps the presented exchange before key waits,
dismissal, or text entry handle input. In Talk history, Up/Down selects an
exchange and Tab keeps it; page keys read its text. Last Conversation uses K,
leaving Tab/Shift+Tab for Journal navigation. Neither capture path submits partial
input, grants knowledge, flushes unseen output, or advances Talk. Combat Tab
retains its meaning. Excerpts uses Delete or D then Enter confirmation; Escape
cancels. A archives/restores the selected excerpt, and H toggles Show Archived
(off on opening Journal, affecting both the excerpt list and Journal search).
These letter bindings only act while browsing Excerpts; search keeps text input.

In Journal's Last Conversation and Excerpts readers, Up/Down selects an exchange.
Space/Shift+Space pages forward/backward within that selection, clamping at the
first and last pages. There are no Page Up/Page Down or Ctrl+arrow paging aliases
in these readers. Search and the other Journal sections retain their own existing
navigation. K captures the complete highlighted exchange without moving its page.

The Use item modal similarly owns keyboard input while open. Tab/Shift+Tab and
Left/Right select non-empty categories; Up/Down and Page Up/Page Down browse;
Space pages overflowing details and lore; Enter selects; Escape cancels. The
modal has no number-key shortcuts or mouse handling. It emits a detached item
identity only, and the owning workflow revalidates current inventory before
mutation. All contexts show every owned identity in the existing Use categories,
including an inventory containing only unavailable items. `UnavailableReason`
blocks activation independently of effect-applicability warnings. Unavailable
rows use muted text and a `[No]` marker with an explicit legend; selected rows
retain contrast. Enter shows the specific restriction on the first detail page
without dismissing or dispatching. Warning-only consumables remain selectable.
Normal details combine concise handling/current state with atmospheric lore;
there are no separate mechanical pages before lore. Page hints/counts appear
only for overflow. Ownership does not reveal mysterious powers, ritual recipes
or conversation-earned clues, including through browsing warnings or pre-use
prompts. Actual deliberate-use feedback may describe the observed result.
Cancellation before final confirmation preserves resources, surface time,
dungeon charge, combat action, and gameplay RNG. Surface Use transfers a
confirmed asynchronous item to an exact-once completion callback; selection
cleanup precedes the item workflow's final surface cycle, including shutdown.
A dungeon menu cancel completes its deferred token without charging it; inspecting
or rejecting an unavailable row keeps the menu and token open without charge.
Confirmed consumable attempts consume their item and cycle once,
including attempts whose effect later fails.

Combat Crown, Amulet and Black Badge wear/remove retain inventory and spend one
action. Deferred Use confirmations retain location, stock, and exact encounter,
actor/clone, and dispatch ownership. The workflow completes only its own committed
attempt; another operation changing the global commit count cannot charge a
cancelled or stale Use workflow.

### Q quick save and Ctrl+Q quick load

Surface and dungeon **Q** save to the active player slot: the slot and reviewed
revision established by the last successful player-slot save/load in this scene.
They preserve the committed checkpoint description and optimistic revision checks;
no slot picker or default-slot preference is involved. Without active provenance,
the player must save or load a player slot first. Ordinary combat Q still reports
`Quit-Not here` without saving or consuming the actor's action.

**Confirm Quick Save** defaults On. On retains the map-relative `Save game?`
overlay: secondary input owns the question, `Y` saves, `N`/`Escape` cancels, and
other keys are ignored. The exact overlay is removed before capture, on
cancellation, and on disposal. A successful save then needs acknowledgement.
Off saves on the accepted Q press and writes the slot-specific success to the
normal output, without a question, secondary input, or success acknowledgement.
Both settings report failures in output and an acknowledged error overlay.
Failed capture/write retains active provenance. Held Q/Y/acknowledgement keys
cannot repeat the operation or leak into the next input owner; Caps Lock is neutral.

Surface Q runs after ordinary command preprocessing (including intoxication and
sailing handling) but before acquiring surface command routing. Neither answer
nor direct save charges a surface cycle. Dungeon Q retains the accepted-command
precharge, including its Quickness phase and Time Stop rules. With confirmation
On the token remains owned until the answer; Off completes it immediately.
Both routes restore primary input and complete exactly one **command-only** token
before capture. The snapshot and resumed scene therefore share the same boundary,
without a full dungeon end-player pass or surface completion. Save eligibility
checks remain unchanged; a success message alone does not prohibit another save.

**Ctrl+Q** is a Redux-only direct active-slot reload, independent of Confirm Quick
Save. Either physical Control key works, without Shift/Alt/Meta; Caps Lock has no
effect. It has no confirmation or slot selection. It uses the stored reviewed
revision, never Latest or a newly adopted revision. Externally changed, deleted,
corrupt, or incompatible slots require ordinary reviewed Load; failure retains
the current scene, generation, provenance and source input, and reports a
load-specific error in normal output without installing a new modal owner.

The main scene samples modified Q before readers or queues can change ownership.
Only a fresh exact chord at normal primary input is admitted: town, world,
Underworld, dungeon, or a player-controlled combat turn (including a controlled
monster). Enemy/scheduled turns, targeting, modal dialogs, readers and action
workflows retain ownership. Harmless floating damage labels and nonblocking
action cards do not prevent loading. A blocked chord is discarded, with raw
reader/any-key polling masked until Q and its modifiers are released; it cannot
become plain Q or a buffered combat command. It never waits to execute when an
enemy turn or prompt ends. A fresh eligible quickload precedes pending buffered
command activation and dungeon precharge.

Quickload uses canonical native loading with a dormant candidate and no outgoing
snapshot, save, or rollback capture. It consumes no action, calendar minute,
resource or gameplay RNG draw. Construction/preflight/activation failure preserves
the outgoing scene. Successful activation retires old scene-owned queues/effects
before publishing the candidate, so shutdown finalizers still resolve their old
owner. Success is reported on the replacement scene, with held-key transition
protection. The outgoing Update returns immediately, before clocks or deferred
update-tail work can mutate it.

Dungeon Yell retains its token in the `dungeon_yell` workflow;
submitted nonempty text reports `No effect!`, while empty input reports
`Nothing`. Escape clears Yell text without completing the command; Return
submits the current input.

## Combat Party Spell Recipients

Combat Mani, An Nox, An Zu and post-victory Vas Mani resolve to
`TargetCombatPartyMember`, using the existing `CombatSpellTargetInput` state.
The spell browser transfers ownership to battlefield targeting with exact caster
focus. Arrows/keypad move freely; Tab cycles visible PC entries in source-slot
order, including clones and shared-square occupants. Enter confirms the exact
reviewed recipient; Escape cancels freely. Invalid confirmation retains the
selector without payment. Preview and confirmation share authoritative presence
and terrain-concealment rules. The HUD explains identity, HP/status, shared
effects, costs and no-benefit casts. An Zu previews the original first sleeping
copy independently of the highlighted copy. A hidden waking position is never
published. One presentation workflow completes each committed combat action;
Journal suspension retains the selector and no selection state is saved.

## Combat Door And Disarm Spells

An Sanct, An Ex Por and In Ex Por retain `CombatAdjacentSpellTargetInput` while
using the familiar full-size white attack cursor. Arrows and cardinal keypad
8/2/4/6 select only the caster's four neighbouring squares; repeated directions
never extend range, diagonals do nothing, and edges do not wrap. The initial
square is the first in-bounds neighbour in north/east/south/west order, without
inspecting doors or chests. Aim text and cursor colour disclose no lock/trap
eligibility. These three spells suppress the generic adjacent underlay and
direction arrows; other adjacent/cone previews retain their own contracts.

Enter, keypad Enter, C or Space confirms. Escape cancels freely; invalid targets
retain selection without MP, mixture, action or RNG cost. Final confirmation
revalidates resources and retains a single-use `CombatUtilitySpellCommit` for the
exact encounter, PC/clone, dispatch, origin, maps and target state. The paid cast
remains owned through effect resolution and finalization. Duplicate callbacks
cannot spend, resolve or finish twice. Shutdown settles an already-paid cast only
for its still-current owner; stale ownership cannot finish a replacement action.
A changed target after payment suppresses success but still completes the owned
paid action.

Successful door changes synchronize the combat reference and effective layered
terrain, removing obsolete overrides. Ordinary Open then sees the unlocked door
and changes both views to floor; Jimmy preserves its physical-unlock/key rules.
Spell semantics and the original/Redux commitment distinction belong to
[SPELL-0001](../gamefacts/MAGIC_SPELLS.md#spell-0001),
[SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002) and
[SPELL-0006](../gamefacts/MAGIC_SPELLS.md#spell-0006).
Core and scene `combat_door_spell_test.go` / `gamescene_combat_door_spell_test.go`
cover these ownership, routing and terrain boundaries.

## Spell Browser Mix Handoffs

Cast and Z-stats Spells reserve `Ctrl+M` locally for preparing the selected spell;
outside those modals the existing music shortcut remains unchanged. Ctrl chords
are consumed before syllable/free-text input and held keys cannot bleed into the
next owner. The shared finder also uses `/` search, `Ctrl+O` to cycle All known /
Prepared / Mixable filtering,
and Space detail paging outside search. While searching, Space enters text;
Escape returns to browse mode for paging while retaining the selected spell.
An incompatible older syllable prefix is cleared rather than selecting a different
spell. Recoverable prerequisite refusals remain visible in the same Cast browser,
with no resource, command-time, or combat-action commitment. No function keys are
required.

Cast awaits the reusable Mix sequence inside its own workflow. The underlying
selector is suspended until Mix's receipt/trap steps and command completion
finish. On the surface, Mix pays one ordinary command cycle; a later cancelled
Cast adds none. In a dungeon, the uncharged Cast token completes before Mix
starts its normal precharged full cycle. Resuming Cast reserves a fresh deferred
token after Mix completion. A map change, combat transition or total party death
prevents returning to an obsolete selector. Z-stats invokes the same sequence in
a scene-owned workflow and refreshes its detached dossier after completion.
Combat Mix requests are refused without spending a combat action.

Opening a spell finder, choosing a recipe and editing quantity never consumes
resources. Enter in preparation transfers a typed request to `ExecuteMixing`,
which validates quantity and current selected-reagent stocks before deduction.
Incorrect recipes still spend the confirmed ingredients and resolve the normal
trap sequence before returning. Workflow finalizers remove the exact owned Mix
and receipt instances, including shutdown while a parent Cast remains mounted.

## Primary To Secondary Transition

A directional command starts its transcript prompt, records the secondary state, and permits the following key to register immediately:

```go
g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Open-")
g.secondaryKeyState = OpenDirectionInput
g.keyboard.SetAllowKeyPressImmediately()
```

`SetAllowKeyPressImmediately` is important when the player presses the command and direction in quick succession. New directional transitions should use it unless the command deliberately requires release or a modal handoff.

Surface primary and secondary handlers return a typed completion result:
completed, rejected, cancelled, deferred, or still active. Completed, rejected,
and cancelled results consume one fixed command cycle by default;
`ConsumesNormalCycle` records command-specific free exceptions. Deferred and
still-active results cannot consume the normal cycle, and map transitions bypass
surface completion. Action methods use the routing ownership guard so direct
callers retain a cycle without production dispatch duplicating it.

Open and Jimmy enter directional input with immediate key registration on both
surface map types. Like the other reachable uncommitted surface direction
prompts, they permit free Escape cancellation before the action starts.
Deferred surface Open/Jimmy character selection re-enters surface routing for
the authoritative mutation, then its completion callback consumes the cycle
after dialog cleanup. Both map types prompt when no eligible actor is active and
multiple eligible party members exist.

Large-map `E` skips routed `FinishTurn` only when the current world coordinate
identifies a dungeon admission attempt. Rejection adds no outdoor turn, and
successful dungeon entry adds no time in its transition workflow; the first
accepted dungeon command owns the next precharge. Other Enter contexts retain
their existing turn ownership.

Outdoor movement and relocation use explicit gameplay-window boundaries:
`MovePlayerOneStep` owns accepted cardinal steps, while `InitializeOutdoorWindow`
owns fresh outdoor entry and successful Blink. Both forced waterfall southward
steps use the step boundary individually. Generic position assignment is not a
substitute for choosing that lifecycle. Push/Pull preserves window origins and
tracks its temporary terrain separately. Saving alone preserves the live window;
actual hydration initializes it from the loaded outdoor position and plane. The
runtime 32×32 window, quadrant-local Push lifetime and load-reset contract are maintained once in
[SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002); the scene's camera projection
and ordinary command completion do not define Blink range.

## Secondary Resolution

The surface handlers first intercept Escape for Get, Push, Open, Jimmy, Klimb,
Look, Talk, Search, Attack, and large-map Fire. `cancelSurfaceDirectionInput`
reports `Cancelled`, restores primary input, and returns a free result without
calling an action, consuming time/resources, advancing actors, or drawing gameplay
RNG. Registration consumes Escape through release without delaying the next fresh
key. This is the deliberate [surface deviation](../gamefacts/COMMANDS_REFERENCE.md#cmd-0003)
from original ordinary direction input, confined to the precommit prompt. It does not cancel an
already committed action, item/spell workflow, combat reader, or dungeon token.

All ordinary surface direction prompts keep their short command text, such as
`Attack-` or `Open-`, without an inline direction/Pass/Cancel legend.
`commandHelpEntries` in `cmd/ultimav/gamescene_command_help.go` describes cardinal
arrow/keypad input, free Escape cancellation, and Space's normal command cost,
including the free outdoor Attack Pass exception.

Combat adds its own scoped `cancelCombatDirectionInput` before the shared reader
for Klimb, Open, Jimmy, Push, Get, and Search. It reports cancellation and restores
primary input for the exact same actor/action without gameplay effects. This also
covers controlled-monster Push and a failed ordinary Klimb retry. Escape is
consumed through release and a fresh command can follow immediately. Dedicated
ladder/tactical selectors and item workflows keep their own rules.

After those context-specific interceptions, `ordinaryDirectionInput` retains the
shared original ordinary/spell direction-reader behavior:

- Cardinal arrows and physical keypad `2`/`4`/`6`/`8` append and return the
  equivalent direction.
- Space appends `Pass`, returns a passed outcome, and terminates the prompt.
- Escape, other invalid keys, and keypad diagonals return an active outcome;
  they do not clear state, consume a cycle/action, or swallow the next key.

The six ordinary combat prompts show `Arrows; Space: Pass; Esc: Cancel`;
combat Help distinguishes free cancellation from paid Pass.
Cast details use the resolved context targeting profile rather than base spell
metadata. Cast guidance describes final-confirmation MP/mixture commitment and
free cancellation beforehand, including preservation of the combat action.
Ready labels come from the authoritative preview: Equip, Unready,
no change, or the blocking reason.

After a selected direction, the map-specific handler calls the relevant
`GameState.Action*` method or UI workflow and normally returns to `PrimaryInput`.
After Pass (distinct from Escape), ordinary surface commands consume their fixed
map cycle without calling the action. Outdoor Attack is the source exception and consumes no
cycle. Combat Open, Jimmy, Push, Get, Search, and ordinary rock-direction Klimb
Pass consume one action. Combat Push also exposes `P` and the ordinary direction
route to a controlled monster; trap, rejection, pull, and push outcomes all
complete through the same exact-once action owner. A failed non-rock combat Klimb direction reports
`What?`, reprompts `Klimb-`, and leaves `KlimbDirectionInput` active for the
exact same actor without consuming the action. A successful rock climb consumes
exactly one action. Single-route combat ladders resolve directly from `K`;
generated dual ladders use `CombatKlimbLadderDirectionInput`, accept Up/`U` or
Down/`D`, and ignore Space rather than applying ordinary-direction Pass
semantics. A successful selected ladder route consumes one action. `Stay with
ship!`, an incompatible established dungeon-room exit, or another selected-route
failure restores primary combat input for the exact same actor without consuming
the action.

Before normal-PC or controlled-monster command dispatch, combat rejects the
original blocked matrix through one shared guard: `Board what?`,
`Enter-Not here`, `Fire-Not here`, `Hole up-Not here`,
`Ignite torch-Not here`, `Look-Not here`, `Mix-Not here`,
`New order-Not here`, `Quit-Not here`, `Talk-Funny, no response!`,
`View-Not here`, and `X-it what?`. Rejection restores the same actor's inner
prompt without entering a secondary state, completing an action, consuming a
resource, advancing time or RNG, or mutating the battlefield. Other unknown
keys retain the generic `What?` inner reprompt. Combat Fire, Look, and Talk do
not enter their surface secondary routes.

Primary cardinal numpad movement is normalized to the same route as arrows
after harpsichord and top-row number handling. Keypad diagonals have no ordinary
movement meaning.

Combat target states are intentionally distinct and longer lived. Physical and
square-target spell crosshairs accept all eight keypad directions; the
[door/disarm cursor](#combat-door-and-disarm-spells) selects only four cardinal
neighbours. Cursor movement keeps the state active, and Escape cancels aiming.
Terminal cancellation also clears the command prompt. Do not apply ordinary
direction semantics to tactical cursors.

## Combat Physical Attack Ownership

`combatMapInputHandler` routes an accepted Tab press through the pure
`PlanCombatAutoAction` decision: one available-source attack before the existing
one-step movement fallback. Eligible enemies are ordered by integer distance,
then current HP, with deterministic path and monster-index fallbacks.
`resolveCombatAutoAttack` begins authoritative per-source targeting with an exact
victim and owns the complete automatic sequence in one input-blocking workflow.
It calls `ContinueCombatAutoAttack` between sources to revalidate the same victim,
range, obstruction, readied state, and ammunition; it does not call the
adjacent-only attack executor or retarget after a kill. See
[Tab semantics](../gamefacts/COMBAT.md#combat-0012).

Manual aiming and Tab share `combatPhysicalProjectileSequence` in
`cmd/ultimav/gamescene_input_combat.go`. A ranged source commits through the core
confirmation path, resolves damage exactly once through
`ResolvePendingCombatAttack` at impact, then awaits any return launch and clears
the projectile before continuation. Neither the next source nor action completion
may run while that travel remains pending. These are workflow steps, not
presentation-feedback mutations.

Manual `startCombatProjectileAttack` resumes `finishCombatAttackTargetAction`
after the shared sequence; usable later sources return to aiming. The automatic
owner instead revalidates later sources without player prompts and waits for
melee impact presentation between sources. Finalizers clear projectile/input
state, resolve an already committed pending impact if interrupted, and avoid
replaying it. Automatic cleanup cancels uncommitted sources and finishes once
only while its original combat/actor still owns the action. Manual initial
cancellation remains free; later source cancellation retains its existing
per-source completion rules. Physical aiming prompts show `Attack 1/2: Spiked
Helm`, then `Attack 2/2: Morning Star`, with the full current equipment name.
The controls advertise Space to skip that item at the origin and confirm away
from it. Initial Escape cancels the sequence; later Escape skips the current
item. Source order, range, resources and completion ownership are unchanged.

The readout keeps full equipment `(RN)`, target `(DN)`, and compact
`Attack` status, including `Ally`/`Enemy` for a disclosed actor. The accompanying
prompt explains R/range and D/distance and gives the target's full name, side
and known Charmed/Summoned/Possessed provenance. At the origin it says `Choose
target`; otherwise an unnamed square says `No one seen`. This deliberately
describes disclosure rather than occupancy, so an invisible actor cannot be
detected by comparing an empty-tile label. It uses the core exact-actor
projection rather than renderer-side occupancy rules. The fixed 45% turn pane
and normal font size remain unchanged. Complete labels wrap to the measured
pane width; their full physical height is reserved before placing NEXT.

Core and scene regression assertions include resource deferral, impact,
return-before-next-source, and shutdown cleanup in
`internal/game_state/combat_attack_preview_test.go` and
`cmd/ultimav/gamescene_combat_auto_attack_test.go`.
Live keyboard acceptance is separate from these injected-input assertions;
record its results in the relevant issue.

## Keyboard Debouncing

`pkg/input.Keyboard` provides three related mechanisms:

- `TryToRegisterKeyPress` consumes a non-repeatable key until physical release and enforces the repeat delay.
- `TryToRegisterRepeatableKeyPress` and
  `IsKeyPressedAndRegisterRepeatable` support held movement and tactical cursor
  keys through physical, injected, and simulated input.
- `PrepareForTransition` and `TryAcceptKey` wait for already-held keys to release before a new dialog state accepts input.

Arrow polling uses the same injected key source and one-shot simulated input as
other bound keys while preserving the original Down, Up, Right, Left priority.
Detection does not consume simulated input; successful registration owns that
boundary.

`ClearFrameConsumption` runs at the start of every update and removes only keys that have been released. `ResetReleaseDebouncing` is used during final dialog/workflow cleanup; it is not a substitute for returning `secondaryKeyState` to `PrimaryInput`.

Use `SetAllowKeyPressImmediately` for command-to-direction continuity. Use `PrepareForTransition` for modal dismissal, KeyWait, and workflow handoffs where the initiating key must not bleed into the next state.

Sign dismissal retains a release gate through workflow cleanup, including held
movement, Space, Escape, and item-command keys. The closing press cannot also
move, Pass, open Escape, or ignite a torch; a released/fresh press acts normally.
Open signs reflow on size changes while retaining Runic display mode.

Small-map cardinal movement that displays `Blocked!` also calls
`PrepareForTransition(0)`, requiring release of the held movement input before
another attempt. The rejected attempt still advances exactly one normal
small-map turn, including NPC AI and eligible NPC-initiated Talk. Successful
held movement remains repeatable; large-map, dungeon, combat, and declined
small-map boundary-exit input retain their existing behavior. Original command
and timing evidence belongs in
[Commands Reference](../gamefacts/COMMANDS_REFERENCE.md) and
[Time Costs](../gamefacts/TIME_COSTS.md).

## Turn And Feedback Ownership

- Input handlers display command prompts and select state.
- `GameState` owns gameplay validation and authoritative mutation.
- Feedback from core actions uses `GameState.SystemCallbacks`; scene handlers do
  not repeat action-owned result text.
- Event workflows own their own input blocking and final turn completion where explicitly designed.
- Failed or cancelled input must follow that command's authentic time-cost rule; a boolean return is not a universal instruction to advance or skip time.

### Choosing The Completion Owner

| Context | Owner | Rule for new or changed commands |
|---|---|---|
| Britannia / Underworld | `completeSurfaceCommand` and typed surface results | Ordinary commands own one two-minute cycle; free exceptions and movement costs are explicit. |
| Town / small map | Same surface completion boundary | Ordinary commands own one one-minute cycle, including blocked movement; transitions suppress duplicate origin completion. |
| Retained surface workflow | Deferred completion callback | Finish only after prompt/dialog cleanup, including cancellation and shutdown paths. |
| Dungeon | `DungeonCommandCycle` | Retain the exact token; choose command-only, full end-player, or transition completion. Cast/Use defer charge until confirmation. Never add surface `FinishTurn`. |
| Tactical combat | `FinishCombatAction` or its deferred token | Preserve exact encounter/actor identity; a free inner reprompt cannot consume an action, time, resources, or RNG. |
| Application UI | Scene/modal owner | Do not infer a gameplay cost merely from opening or dismissing a dialog. |

Two easy-to-miss exceptions: combat Ready dismissal, including Escape without
equipment changes, completes one action **after cleanup**; explicit small-map
Attack can retain its origin turn until tactical combat returns. Do not complete
either by observing that a dialog or combat scene merely opened.

### Verification Entry Points

Use `cmd/ultimav/gamescene_command_context_routing_test.go` for bounded primary
dispatch, `gamescene_direction_input_test.go` for ordinary versus specialized
directions, and `gamescene_surface_completion_test.go` for immediate/deferred
surface ownership. `gamescene_input_dungeon_test.go` covers retained dungeon
tokens. `gamescene_combat_ready_test.go` and
`gamescene_smallmap_combat_return_test.go` exercise the exceptions above.
`gamescene_surface_direction_cancel_test.go` covers the free surface precommit
Escape boundary; `gamescene_camping_cancellation_test.go` distinguishes cancelling
guard selection from explicitly choosing No One, including dungeon precharge.
Pair routing assertions with focused `internal/game_state/action_*` or workflow
tests for the terminal effects; a routing test alone does not prove a full
interaction or manual keyboard acceptance.

## Key Files

- `cmd/ultimav/gamescene.go`
- `cmd/ultimav/gamescene_input.go`
- `cmd/ultimav/gamescene_input_common.go`
- `cmd/ultimav/gamescene_input_smallmap.go`
- `cmd/ultimav/gamescene_input_largemap.go`
- `cmd/ultimav/gamescene_input_combat.go`
- `cmd/ultimav/gamescene_input_dungeon.go`
- `cmd/ultimav/gamescene_dialog_helpers.go`
- `pkg/input/keyboard.go`
- `internal/game_state/action_*.go`
