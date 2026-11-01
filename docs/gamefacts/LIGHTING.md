# Lighting, visibility, and cosmetic daylight

Original DOS visibility combines an 11×11 flood fill, time-dependent sight,
static light pools, and carried fuel. Redux's visibility distances and RGB
presentation are separate comparisons. A saved or derived sight threshold is
not automatically a radius or brightness multiplier. No unchecked-capacity
observation below is a reproduced retail memory-corruption result.

### LIGHT-0001

**Ambient sight and carried-light floors**

**Kind:** rule
**Uses:** TIME-0001
**Original:** Established
**Redux:** Unverified

Ordinary original ambient recalculation runs only when current sight is below 51:

| Time | Sight |
| --- | ---: |
| 00:00–04:59 and 20:00–23:59 | 2 |
| 05:00–05:09 | 2 |
| 05:10–05:19 | 5 |
| 05:20–05:29 | 10 |
| 05:30–05:39 | 20 |
| 05:40–05:49 | 34 |
| 05:50–05:59 | 49 |
| 06:00–18:59 | 50 |
| 19:00–19:09 | 49 |
| 19:10–19:19 | 34 |
| 19:20–19:29 | 20 |
| 19:30–19:39 | 10 |
| 19:40–19:49 | 5 |
| 19:50–19:59 | 2 |

Ararat/map 25 or encoded level above 127 forces baseline 2. Then nonzero magical
light raises sight to at least 18; nonzero torch fuel raises it to at least 10.
These are visibility thresholds, not RGB values or integer radii. Sight itself
is saved at `SAVED.GAM` offset `0x02FF` despite conditional recalculation.

Redux separates calendar queries, visibility distances, and tint darkness.
Its smoothly interpolated tint and player-light selection do not establish exact
stepped-sight equivalence. In particular, a predicate named full daylight is not
a substitute for this numeric schedule; review its actual caller before assuming
hour 19 still has sight 50. Redux's lighthouse full-daylight predicate now uses
06:00–18:59; it no longer suppresses beams throughout hour 19. Smooth tinting,
window daylight, source colours and other declared presentation improvements remain.

**Implementation:** `internal/datetime/ultima_date.go`, `internal/map_state/lighting.go`

### LIGHT-0002

**Eight-neighbor flood fill and obstruction**

**Kind:** rule
**Uses:** LIGHT-0001
**Original:** Established
**Redux:** Unverified

Original exploration uses an 11×11 eight-neighbor flood fill, not true ray tracing.
For valid view coordinates centered at (5,5), the mirrored range table equals
`(x - 5)^2 + (y - 5)^2`. The fill increments supplied illumination before a strict
comparison, equivalent to including squared range at most the supplied sight.
Obstruction/static illumination still affect admission; light can propagate around
corners differently from a ray-cast system.

Opaque categories are forest, tropical forest, mountains, peaks, rocks, secret
doors, walls, window shelves, ordinary/locked/magically locked opaque doors,
fireplaces, special wall tiles D0–D3 hex, wall signs, white walls, and hidden/empty
terrain. Windows, arrow slits, and all three windowed-door variants transmit only
when the supplied squared range is exactly 1. That range is relative to the current
fill source, which can be a static emitter rather than the party.

Redux's distance map uses its own radius discs and visibility pipeline. Similar
names and overlapping tile categories do not prove identical propagation, corner,
window, or out-of-view behavior. Full visibility equivalence remains unverified.

**Implementation:** `internal/map_state/lighting.go`

### LIGHT-0003

**Static emitters and scan capacity**

**Kind:** rule
**Uses:** LIGHT-0002
**Original:** Established
**Redux:** Unverified

| Original emitter | Tile, hexadecimal |
| --- | --- |
| Wall torch 1 / wall torch 2 | B0 / B1 |
| Red flame / roasting meat | B2 / B3 |
| Fireplace / lamp | BC / BD |
| Candelabrum / stove | BE / BF |
| Moongate / blue flame | DC / DE |

All ten feed illumination 10 to the same fill; off-screen emitters can illuminate
visible cells. Fireplace is both emitter and blocker. The inspected algorithm
has no per-source light-color model; that says nothing about artwork colors.

The source queue has space for 32 coordinate pairs, without an inspected bounds
check enforcing a safe 32-source limit. Scanning is not entry-only: verified
refreshes include gate-terrain changes, outdoor-region changes, and wall-torch
pickup. Overflow and boundary-index outcomes have not been runtime-reproduced.

Redux assigns per-tile emission distances and has additional field, quest-item,
and creature sources. Its collection/visibility filters differ; the original ten-
emitter table is not a complete list of Redux lights. Its presentation choices
are described below without claiming equal original source admission.

### LIGHT-0004

**Magical light and wall-torch assignments**

**Kind:** rule
**Uses:** PROV-0003, TIME-0001, INV-0006, INV-0007
**Original:** Established
**Redux:** Unverified

Torch inventory commitment and context-specific fuel belong to
[PROV-0003](PROVISIONS_CONSUMPTION.md#prov-0003). Other original effect assignments:

| Effect reached | Mutation |
| --- | --- |
| Prepared In Lor | Replace magical light with 100. |
| Prepared Vas Lor | Replace magical light with 255. |
| Light scroll | Consume the scroll before assigning magical light 240. |
| Take wall torch | Remove emitting terrain and replace torch fuel with 100. |

Magical light replaces rather than adds fuel; it and torch fuel are independent
of the active-duration spell slot. Their decrements use effective game minutes
and the temporal modifiers in TIME-0001. Assignment values are not universal
future-command counts. Prepared-cast eligibility/cost/build qualifications remain
in INV-0006; reaching a light effect is narrower than proving every cast path.

Redux has an independent durable magical-light timer; `ActivateLightEffect`
replaces it and updates light state without replacing the active duration effect.
The In Lor/Vas Lor constants match 100/255. Taking a wall torch is verified on
both the town and tactical Get paths: the sconce becomes brick floor and carried
torch fuel is replaced with 100, with no inventory torch gained. An earlier Redux
town path pocketed a torch instead; that is corrected. This narrow effect review
does not certify every scroll, save-hydration, or resource-commit path.

**Implementation:** `internal/game_state/effect_spells.go`, `internal/game_state/turn.go`, `internal/game_state/action_get.go`, `internal/map_state/lighting.go`
**Tests:** `internal/game_state/combat_terrain_commands_test.go`

### LIGHT-0005

**Dungeon darkness and tactical terrain**

**Kind:** rule
**Uses:** LIGHT-0004
**Original:** Established
**Redux:** Unverified

Original dungeon perspective draws panels only if torch or magical-light fuel is
nonzero; otherwise it fills the view black. Dungeon Look and Search select the
character, then reject darkness before further targeting. This is not graduated
surface sight.

Tactical-map display bypasses exploration fill and copies combat terrain, so
ambient sight/light pools/carried fuel do not restrict terrain visibility. Object
admission is another pass: invisible monsters, hidden/empty terrain, arches, and
other occupant rules can still suppress sprites. Fully visible terrain is not a
promise to draw every occupant.

Redux has separate dungeon/combat lighting tests, but exact action-selection and
every occupant-admission branch were not re-audited in this lighting conversion.

**Tests:** `internal/map_state/lighting_dungeon_test.go`, `cmd/ultimav/gamescene_combat_lighting_test.go`

### LIGHT-0006

**Redux radii, tint interpolation, and additional sources**

**Kind:** rule
**Uses:** LIGHT-0001, LIGHT-0003, LIGHT-0004
**Original:** Established
**Redux:** Intentional deviation

**REDUX DEVIATION: LIGHT-0006:** Redux uses presentation-oriented emission
distances, colored source tints, blended contributions, and cosmetic breathing,
rather than ten equal original illumination-10 static sources. Carried torch
radius is three tiles; both In Lor and Vas Lor use five tiles. Those are Redux
distances, not the original sight thresholds 10 and 18. Fields, quest items, and
some creatures add emitters beyond the original list. Radius/source admission
changes must not be conflated with tint-only changes.

The configured ambient radius defaults to two tiles and supports values one
through five. Ordinary Underworld exploration receives that ambient pool at every
surface hour when no torch or magical light is active; see LIGHT-0008. Carried
magical light takes precedence over torch, which takes precedence over ambient.

For smoother day/night appearance, cosmetic darkness fades from 0 to 1 during
19:30–20:00 and from 1 to 0 during 05:00–05:30. This fade does not itself alter
tile visibility or add black tiles. Color modes use global darkness: night-only,
half-strength during day, or always-on. Breathing takes animation time and affects
color intensity, not gameplay fuel, RNG, or calendar.

The original has no equivalent per-source RGB model. These choices improve
presentation and source distinction while retaining separate authoritative fuel
and visibility responsibilities. Exact visibility fidelity remains scoped to
the earlier rules, not certified by cosmetic tests.

**Implementation:** `internal/map_state/lighting.go`, `internal/map_state/light_source_tinting.go`, `internal/datetime/ultima_date.go`

### LIGHT-0007

**Redux daylight targets, openings, and lifecycle**

**Kind:** rule
**Uses:** LIGHT-0006
**Original:** Established
**Redux:** Intentional deviation

**REDUX DEVIATION: LIGHT-0007:** original global sight/static sources do not provide
indoor brightness targets or window-daylight pools. Redux adds whole-tile cosmetic
targets to distinguish shaded interiors from outdoors without changing visibility,
collision, fuel, or entity admission. It reuses immutable map exposure rather than
adding another flood fill. Hidden rooms can have targets without becoming visible.

`DefaultDaylightOptions` owns application defaults; each field receives validated,
copied options. Change defaults and recreate/restart GameState to retune; there
is no mutable global, settings dialog, or runtime reload for these options.

| Option | Default | Contract |
| --- | --- | --- |
| OutdoorBrightness | 1.00 | Finite multiplier in [0.3,1]. |
| IndoorBrightness | 0.75 | Finite [0.3,OutdoorBrightness]; current visual tuning. |
| OpeningFalloff | [1.00,0.60,0.25] | Up to eight finite nonincreasing weights in [0,1]; empty disables openings. |

Weight `d-1` applies in inward Euclidean band `(d-1,d]`:

```text
target = indoor + (outdoor - indoor) * weight
```

Defaults yield 1.00, 0.90, 0.8125, then baseline 0.75. The existing calendar tint
controls dawn/dusk; no second daylight clock is introduced.

Stone cross/glass windows and windowed doors are automatic openings, even when
closed or locked. Opaque source doors require live open-door state, including
same-floor occupancy holds and the player timer. An opening must have exactly
one opposite cardinal outdoor/indoor pair; internal, outdoor-to-outdoor,
diagonal-only, or ambiguous pairs do not qualify.

An active opening brightens itself and an inward semicircle. Structural receivers
along the opening plane may brighten too. Integer squared thresholds select
distance bands; overlapping targets take their maximum, never sum. Walls receiving
light do not relay it. There are no rays, obstacle tests, recursive relays, sun
direction, or cast shadows. Source identity/exposure remain stable under runtime
layer changes; live door activation is refreshed separately.

Floor reload caches opening descriptors after exposure classification. Full source-
terrain rebuild clears transient player-open timer/position, including same-location
debug repositioning. Visible nonempty same-floor actors and Avatar re-establish
occupancy holds from authoritative state, not camera/renderer caches. Departure or
removal ends occupancy-only openness; replacing one timer does not close terrain
under another occupant. Lock identity remains separate. Targets belong to the
current layer and deactivate on clear, replacement, excluded context, or full night.

Serpent's Hold regressions cover window (12,25) lighting westward to (10,25), the
non-emitting internal door (5,25), and exterior door (11,14) only while open,
along with expiry/rebuilds, unchanged dither masks, and preserved snapshot/RNG.

**Implementation:** `internal/map_state/daylight.go`, `internal/game_state/daylight_tinting.go`
**Tests:** `internal/map_state/daylight_original_test.go`, `internal/game_state/daylight_tinting_test.go`, `cmd/ultimav/gamescene_daylight_test.go`

### LIGHT-0008

**Boundary viewpoint, color composition, and Underworld**

**Kind:** rule
**Uses:** LIGHT-0007
**Original:** Established
**Redux:** Intentional deviation

**REDUX DEVIATION: LIGHT-0008:** this is Redux RGB presentation, not an original
color formula. Whole-tile walls have one multiplier, not separately shaded faces.
When viewed from outdoors, a boundary adjoining outdoor space in any of eight
directions takes an outdoor target. An exterior threshold needs cardinal outdoor
contact to count as an outdoor viewpoint. Viewed from inside, walls retain their
indoor target plus direct opening spill; enclosed partitions gain no outside
override. This one-hop rule does not reclassify walls or brighten interior floors.

For participating sources, daytime brightness is the greater of local target and
closest daytime-eligible source brightness, capped at one. Night-only sources do
not supply that lift. Blend this scalar toward existing nighttime source brightness,
then add weighted color deltas once and clamp. Unlit tiles blend their daylight
target toward the 0.40 night fallback. Full night and outdoor target 1 preserve
the pre-daylight composition; deliberate tints may still reduce RGB channels.
Indoor shade does not activate night-only sources during the day.

Daylight targets apply to ordinary small-map floors at least zero and overworld
floor zero. Underworld exploration uses full-night cosmetic darkness at every
surface hour, with nighttime source strength/colors/falloff. Its no-carried-light
visibility also keeps the configured ambient player radius at every hour instead
of losing that source during surface daytime. This is an explicit Redux ambient
visibility policy, separate from original sight thresholds and from RGB tinting.
Full visibility recomputation therefore preserves the admitted tile set when only
surface time changes. LOS/obstructions and changing sources still affect admission.
Breathing remains independent of world time. Neither policy consumes fuel, advances
the calendar, or draws RNG. Returning
to the surface restores clock tinting and stale daylight targets cannot cross layers.
Basement, dungeon, combat/camping, mini-map, and finale policies are not broadened
by this exploration rule.

Regression fixtures compare indoor (10,24) and outdoor (15,23) views of Serpent's
Hold, diagonal exterior corners, and bricks beside windows. Underworld tests fix
source/animation state across midnight, dawn, noon, dusk, and transitions.

**Implementation:** `internal/game_state/daylight_tinting.go`, `internal/game_state/ambient_tinting.go`, `internal/game_state/game_state.go`, `internal/map_state/light_source_tinting.go`
**Tests:** `internal/game_state/daylight_boundary_view_test.go`, `cmd/ultimav/gamescene_daylight_boundary_test.go`, `cmd/ultimav/gamescene_underworld_tint_test.go`
