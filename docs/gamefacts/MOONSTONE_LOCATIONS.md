# Moonstones, moongates, and remembered destinations

Physical placement, source gates, destination phases, and player knowledge are
different authorities. Original DOS rules below are scoped to inspected paths,
not every authored binary or retail edition. Redux chart/journal rules are
explicit additions; original printed-chart artwork and every historical knowledge
mechanism have not been reconstructed by this review.

### MOON-0001

**Initial placements**

**Kind:** rule
**Uses:** INV-0008
**Original:** Established
**Redux:** Matches

The reviewed initialization/template paths place all eight stones on map 0,
level 0 at these coordinates:

| Stone/phase index | Decimal X,Y | Hexadecimal X,Y |
| ---: | --- | --- |
| 0 | 224,133 | E0,85 |
| 1 | 96,102 | 60,66 |
| 2 | 38,224 | 26,E0 |
| 3 | 50,37 | 32,25 |
| 4 | 166,19 | A6,13 |
| 5 | 104,194 | 68,C2 |
| 6 | 23,126 | 17,7E |
| 7 | 187,167 | BB,A7 |

These are initialization values, not a fresh measurement of shipped `INIT.GAM`.
Not every initialization path explicitly assigns every placement field; the
template-generation path establishes map and level zero. Runtime coordinates
remain movable, not fixed destinations tied to town names.

Redux's initial moonstone references use these placements. Its immutable supplied
chart metadata is separate from current stone state and personal confirmation.

**Implementation:** `internal/references/moonstone.go`, `internal/references/charted_moonstones.go`
**Tests:** `internal/references/moonstone_test.go`

### MOON-0002

**Independent moon phases and saved caches**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Unverified

| Day | Trammel | Felucca | Day | Trammel | Felucca |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 0 | 0 | 15 | 0 | 4 |
| 2 | 1 | 0 | 16 | 1 | 5 |
| 3 | 1 | 1 | 17 | 1 | 6 |
| 4 | 2 | 2 | 18 | 2 | 7 |
| 5 | 2 | 3 | 19 | 2 | 0 |
| 6 | 3 | 4 | 20 | 3 | 0 |
| 7 | 3 | 5 | 21 | 3 | 1 |
| 8 | 4 | 6 | 22 | 4 | 2 |
| 9 | 5 | 7 | 23 | 5 | 3 |
| 10 | 5 | 0 | 24 | 5 | 4 |
| 11 | 6 | 0 | 25 | 6 | 5 |
| 12 | 6 | 1 | 26 | 6 | 6 |
| 13 | 7 | 2 | 27 | 7 | 7 |
| 14 | 7 | 3 | 28 | 7 | 0 |

Day selection supplies two distinct phases, including repeats. The original sky
refresh assigns phase-character bytes that are also saved at `SAVED.GAM` offsets
`0x02DF` and `0x02E0`; travel consumes those stored values rather than recomputing
them on every call. Derived values can still be persisted. Phases choose a normal
destination stone, not which physical stones may create source gates.

Redux's date accessors reproduce this schedule for valid dates and derive it
directly from current calendar state. That establishes table agreement, not
equivalence of stale original phase-cache scenarios or malformed saved phases.

**Implementation:** `internal/datetime/ultima_date.go`

### MOON-0003

**All-stone source eligibility and shared stage**

**Kind:** rule
**Uses:** MOON-0001, INV-0008
**Original:** Established
**Redux:** Intentional deviation

Every original gate update considers all eight stones. Map and encoded level
must equal the current location. On map 0, wrapped offsets must additionally lie
within the loaded 32×32 region. There is no discovery-bit or current-phase test.

At hours 20–23 and 0–4, the shared stage grows toward 16; otherwise it shrinks
toward zero. Shrinking terrain remains a gate until zero, when it becomes grass.
This runs through eligible display updates with update/motion/map/Time Stop gates,
not once per calendar minute. Original entry tests gate **terrain**, not an
independent stage-positive predicate. Stage is saved at `SAVED.GAM` offset `0x02E1`.

Redux uses one authoritative runtime stage 0–16 for entry, rendering, lighting,
and audio, together with placed-stone identity. Any positive stage is enterable.
Progress uses controlled elapsed/event updates and freezes under Time Stop.
**REDUX DEVIATION: MOON-0003:** stage is transient, reconstructed as 16 or 0 from
loaded hour; saves during transitions are rejected. This avoids persisting a
partially running transition. It does not preserve the original saved stage or
establish exact display-update cadence. Consistency among Redux consumers is
different from literal original entry-predicate equivalence.

**REDUX DEVIATION: MOON-0003:** the original steps every gate silently through
display updates. When growth or shrinking starts while a placed source gate on the
current map and floor is in the party's line of sight, Redux animates the change
and adds "Instantly, a shimmering blue door springs up!" or "The shimmering door
fades from view." Otherwise the stage reaches its target at once, without a message
or a running workflow, so an unseen change never interrupts held commands.

**Implementation:** `internal/game_state/moongate_events.go`, `internal/game_state/moongate_system_integration.go`
**Tests:** `internal/game_state/moongate_stage_test.go`

### MOON-0004

**Exposure, pickup, burial, and visible layering**

**Kind:** rule
**Uses:** INV-0008, MOON-0003
**Original:** Established
**Redux:** Unverified

The placement/possession mutations are canonical in
[INV-0008](INVENTORY_SYSTEM.md#inv-0008). Search skips moonstone discovery while
terrain is an active gate. Exposing a loose stone leaves its placement eligible
as source/destination; pickup's map-255 sentinel prevents ordinary destination
travel and removes its normal placed-source eligibility.

Burial permits map IDs below 33 and plowed field, crops, or terrain numerically
above shoals and below hills. That helper imposes no separate above-ground level
test. Colocated stones and duplicate loose-object handling were not exhaustively
audited.

Eligibility does not prove that an exposed stone leaves the gate visibly
unobscured: the original object pass can replace terrain presentation at that
cell. No original partial-opacity overlay rule was established.

Redux retains distinct buried/exposed/carried states and source checks by full
map/floor/position, with tests for exposed destinations. It draws gates before
items and gives a loose stone over an active gate partial opacity. That is
current presentation, not an original layering Match. `CommitMoonstoneBurial`
commits inventory, placement, and earned observation before confirmation feedback;
ordinary time remains workflow-owned. Full original cancellation/layering
equivalence remains unverified.

**Implementation:** `internal/game_state/moonstone_system.go`, `internal/game_state/journal_moonstones.go`
**Tests:** `internal/game_state/moonstone_integration_test.go`, `internal/game_state/moonstone_burial_test.go`

### MOON-0005

**Travel selection and midnight shrine dispatch**

**Kind:** rule
**Uses:** MOON-0002, MOON-0003, MOON-0004
**Original:** Established
**Redux:** Known discrepancy

After the original gate disappearance sequence:

| Current time | Selection |
| --- | --- |
| 00:00–00:09 | Return a shrine-path indication, bypassing destination-stone selection. |
| Other times before noon | Stored Trammel phase. |
| Noon onward | Stored Felucca phase. |

Ordinary travel reads the selected stone's current X,Y,map,level and fails if
its map is 255. The midnight branch is independent of that destination's
availability. The subsequent shrine routine compares current X,Y with shrine
coordinates, choosing a match if found and defaulting to Spirituality index 6
only if none match. An unconditional coordinate-free index-6 dispatch is therefore
not the literal original rule. Reachable relocated-stone cases require further
verification before generalizing that exceptional outcome.

Redux captures a semantic route before its travel workflow and explicitly chooses
Spirituality index 6 at midnight, restoring the source location afterward. Normal
travel uses live date-derived phase and current movable placement. The midnight
coordinate exception and original phase-cache cases are not established matches;
their product intent remains unresolved.

An independently documented **REDUX DEVIATION: MOON-0005** keeps an ordinary
source gate steady beneath the Avatar's departure dissolve rather than contracting
it first. This preserves the departure presentation; it is not a rationale for
the separate shrine-selection discrepancy.

**Implementation:** `internal/game_state/moonstone_system.go`, `internal/game_state/moongate_system_integration.go`
**Tests:** `internal/game_state/moongate_entry_integration_test.go`, `internal/game_state/moongate_phase_logic_test.go`

### MOON-0006

**Redux supplied chart and personal confirmation**

**Kind:** rule
**Uses:** MOON-0001
**Original:** Incomplete
**Redux:** Unverified

The original initialization sites are established above; original printed-chart
artwork, inscriptions, and a full historical knowledge-system comparison are not.
The following is the maintained Redux navigation contract, not a claim that the
original game carried the same journal model.

The supplied chart is available from the start. Its eight **Charted origins** are
immutable, value-owned landmarks in phase order: Near Moonglow, Britain, Jhelom,
Yew, Minoc, Trinsic, Skara Brae, and New Magincia. These are Redux regional labels,
not claimed original inscriptions. Each phase initially says **Not personally
confirmed**, despite visible supplied origin artwork.

Personal original-site authority and its regional note retire permanently after
successful pickup/burial, including reburial at the same coordinates, or personal
observation at another site. `OriginalSiteRetired` means handling/relocation
evidence, not a visit inferred from a supplied chart. Static original-phase symbols
remain artwork after retirement, granting no personal discovery, numeric fix,
leader, selection brackets, or focus. Returning to an original coordinate or
reloading does not restore retired authority.

**Implementation:** `internal/references/charted_moonstones.go`, `internal/game_state/journal_moonstones.go`
**Tests:** `internal/references/charted_moonstones_test.go`, `internal/game_state/journal_moonstones_test.go`

### MOON-0007

**Redux identified observations and live gate Look**

**Kind:** rule
**Uses:** MOON-0003, MOON-0004, MOON-0006
**Original:** Incomplete
**Redux:** Unverified

This is current Redux knowledge policy; the original Look/knowledge system has
not been exhaustively compared. Successful ordinary travel identifies its captured
destination phase; Vas Rel Por identifies its selected phase. Search, pickup, and
burial identify only the handled stone. An adjacent Look at a uniquely identifiable,
unobscured active source can be first discovery.

Look resolves physical source identity by current map, signed floor, X,Y—not the
travel destination phase, chart origin, discovery flag, or remembered position.
Stage must be positive. Closed gates, orphan gate sprites, carried stones, wrong
map/floor, and out-of-bounds targets grant no observation. Live map units, visible
occupants, item stacks, corpses, and visible quest overlays take precedence;
cached renderer visibility is not observation authority. Colocated source stones
remain ambiguous and receive only a generic portal description.

Observation changes knowledge only, without exposing/moving the buried stone or
changing inventory. It commits before the ordinary Look minute, adding no time,
RNG, or resource cost. The dossier has one row per phase, no separate gate category,
and no unmeasured site/observer coordinates. Finite source descriptions reveal no
hidden location keys. Projection uses remembered evidence and inventory, never
live placement arrays to fill gaps or track remote movement.

**Implementation:** `internal/game_state/moongate_system_integration.go`, `internal/game_state/journal_moonstones.go`, `internal/journal/moonstones.go`
**Tests:** `internal/game_state/atlas_moongate_look_test.go`

### MOON-0008

**Automatic target surveys and sampling boundaries**

**Kind:** rule
**Uses:** ASTRO-0006, MOON-0007
**Original:** Established
**Redux:** Intentional deviation

**REDUX DEVIATION: MOON-0008:** original Sextant Use reports the Avatar's position;
Redux also surveys the exact successfully identified moonstone target automatically
for navigation, without another command, prompt, time, RNG, or resource cost.
Default eligibility requires a carried Sextant on Britannia's surface at
19:00–05:59. Flexible policy also permits daytime measurements of actual surface
sites. Town-local, dungeon, and Underworld sites can earn identity/internal
recognition but not an overworld target measurement. A settlement-exterior
instrument fix is a different coordinate authority.

Travel captures destination phase when selecting the route and samples possession,
destination context, and game clock immediately after authoritative arrival.
Departure eligibility and cosmetic cleanup do not choose the survey. Same-position
arrival counts. Generic teleports, failed activation, pre-arrival cancellation,
and Spirituality grant none; post-arrival cancellation preserves an already
committed observation.

First discovery source/time are stable. Last observation and measurement carry
separate timestamps. Eligible source-gate Look can discover and measure the exact
adjacent target in one action; the Avatar's position remains observer provenance.
An ineligible Look can record identity first and a later eligible Look can measure.
Earned flexible provenance survives disabling the option. Acquiring a Sextant,
clock ticks, rendering, journal/atlas reads, and hydration do not measure old
sightings or follow remote placements.

Surface Look emits `AtlasUpdated` for first site discovery or a newly measured
site with the existing phase key. Repetition at an already measured site adds no
duplicate notice; off-surface observations add no Britannia notice. Eligible
repeats may refresh dates without replacing first-discovery evidence.

**Implementation:** `internal/game_state/journal_moonstones.go`
**Tests:** `internal/game_state/atlas_moongate_look_test.go`, `internal/game_state/journal_moonstone_arrivals_test.go`

### MOON-0009

**Placement-specific surveys, atlas, and persistence**

**Kind:** rule
**Uses:** INV-0008, MOON-0006, MOON-0008
**Original:** Incomplete
**Redux:** Unverified

Original physical placement fields are defined in INV-0008; the following
placement-specific journal and native migrations are Redux contracts, not a
verified original personal-navigation format.

Pickup records its source before discarding physical placement, clears the old
survey/context, and takes no new measurement. Inventory owns carried status;
carried stones have no destination. Burial starts a new placement even at the
same coordinates. An unmeasured new placement never inherits an old measurement.
An ineligible confirmation at the same measured placement preserves its survey.

`LastObservedSurvey` is the only plottable remembered current placement;
`HistoricalSurvey` remains empty. Valid remembered measurements remain readable
without a currently usable Sextant. Unobserved relocation leaves the remembered
position qualified as **last observed**, not silently updated. The atlas has one
stable row per phase: carried/unmeasured new placements have no earned plot;
measured new placements have only their new plot. Static supplied origin artwork
does not enter `PlotPosition`, mapped counts, or interactive labels. An earned
point coincident with its matching origin coalesces visually.

Native snapshots persist origin retirement and the current placement, and reject
surveys inconsistent with the latest placed, uncarried site. Possession-only
legacy imports retire the origin but invent no site, visit, time, or survey; their
atlas row remains personally undiscovered. Hidden placement bytes/current position
cannot manufacture observations. Native snapshots accept `look` as first discovery
evidence in its existing fields.

**Implementation:** `internal/game_state/journal_moonstones.go`, `internal/journal/moonstones.go`, `internal/save_snapshot/validation_v20.go`, `internal/save_snapshot/legacy_inference.go`
**Tests:** `internal/save_snapshot/moongate_look_test.go`, `internal/game_state/journal_moonstones_test.go`
