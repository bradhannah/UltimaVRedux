# Legacy To Native Save Crosswalk

## Scope

This document records how the typed `internal/legacy_save` decoder converts an
original DOS `SAVED.GAM` plus its sibling `SAVED.OOL` into
`save_snapshot.CurrentVersion`. Original offsets and source semantics remain authoritative in
[`SAVED_GAM_STRUCTURE.md`](../gamefacts/SAVED_GAM_STRUCTURE.md); the native
contract is defined by [`SAVE_SNAPSHOT_SCHEMA.md`](SAVE_SNAPSHOT_SCHEMA.md).

Each decoded source field has one of four dispositions:

- **Exact:** copied into a native semantic field without reinterpretation.
- **Semantic:** translated from original representation into native identity.
- **Derived/defaulted:** reconstructed deterministically and reported when
  information is unavailable.
- **Omitted:** transient, ambiguous, or not safely representable; material loss
  produces a stable `ConversionWarning` path and code.

## Core State

| Original region | Native authority | Disposition |
|---|---|---|
| Sixteen 32-byte character records | `party.roster` | Exact values with validated identity enums and packed active-party normalization. |
| Inventory counters and arrays | `inventory` | Exact quantities; original special items map to stable quest-item identities. |
| Calendar, position, wind, light, camp, and wanderer counters | `world`, `wind`, `effects` | Exact source values where durable; native elapsed time and RNG continuation use deterministic conversion defaults. |
| Active-player selection and beggar reward cooldown | `continuation.activePlayerIndex`, `continuation.beggarKarmaCounter` | Semantic validation and normalization. |
| Combat action counter and opening-campaign reduction | `continuation.combatActionTimeCounter`, `continuation.openingCampaignEncounterReduction` | Exact durable continuation. Opening reduction clears at the first month rollover; the action counter remains modulo ten across encounters ([TIME-0003](../gamefacts/TIME_COSTS.md#time-0003), [TIME-0007](../gamefacts/TIME_COSTS.md#time-0007)). |
| Shrine, dungeon, moonstone, Shadowlord, search, and NPC bitsets | `progression`, `party.metNpcs`, `party.deadNpcs` | Semantic conversion into stable identities and fixed native arrays. |
| Dungeon cells, facing, last direction, active object slots 1 and 2 | `dungeon` | Exact active cells, facing, last direction, and optional roaming-monster current/previous state. |
| Active spell action, magic action flags, automatic sailing, and other action scratch | none | Omitted as transient orchestration with deterministic warnings where material. |

There is no original source field for native Journal History or tab-visit/change
metadata. Conversion defaults root `presentation` to empty History,
revision zero, a non-null empty change array, and three zero seen baselines
(`journal.EmptyPresentation`). This is a quiet default, not missing transcript
reconstruction or a data-loss warning. No name/domain association, Flame/place,
method, Word of Power, or dated discovery is invented from met-NPC bits, visits,
inventories, or canonical engine pairings. Existing defeat migration and hidden-spell
inference keep their source-backed rules. Native snapshot compatibility is
separate from original-save conversion; see the
[native compatibility contract](SAVE_SNAPSHOT_SCHEMA.md#snapshot-version-compatibility).

Current conversion defaults every chest assessment to uninspected, including all 512 active
dungeon cells. Physical trap state is preserved separately and never interpreted
as player inspection. Dungeon journal sites and surveys start empty. The only
moonstone journal backfill is actually carried `inventory.moonstones`: those
phases become known from possession with `originalSiteRetired: true` because
possession proves pickup, without a personal visit, site, timestamp, or survey. Raw
moonstone placements/found bits do not disclose journal evidence. No Oppression
password knowledge or personal original-site visits are manufactured.
Atlas landmark observations likewise start empty: the 34 supplied directory rows
and separate 27-symbol artwork allowlist come from immutable reference metadata,
not imported visits or discoveries. Static moonstone origins do not authorize
personal current-destination markers after retirement.
V22's survey `flexible` flag defaults to false in every empty dungeon and
moonstone survey; original source bytes provide no measurement-policy evidence.
The current navigation preference is not an import input or saved authority and
cannot backfill measurements, atlas entry/naming evidence, or private moonstone
coordinates.

Snapshot 23's native `openDoor` continuation defaults to null. Conversion does
not infer a timed opening, countdown, remembered closed variant, or unlock
history from actor occupancy or object tiles; native map mutations still start
empty. Occupancy-only openness is derived after normal actor hydration.

The expanded V23 atlas domain does not add legacy backfills. Sextant ownership,
Greyhaven position, met-NPC flags, and supplied chart references prove neither a
live `sextant-receipt` nor its prior Enter naming evidence. They also do not prove
a Look observation at Guardians. Imported `atlasLandmarks` remains empty and
all unmeasured surveys retain `flexible: false`. These are quiet native defaults,
not inferred acquisition events or new conversion warnings.

### Snapshot 25 Excerpt Defaults

Original saves do not contain Redux kept excerpts. Conversion initializes an
empty `excerpts` collection in the native snapshot and never reads an external
notebook. Temporary notebook import applies only to older **native** snapshots;
see [save-owned excerpts](SAVE_SNAPSHOT_SCHEMA.md#snapshot-25-save-owned-excerpts).

### Snapshot 24 Native-Only Defaults

Original files contain no player-authored atlas notes, registered fleet identities
or proof of final Codex-page acknowledgement. Current conversion therefore uses
empty `atlasNotes.records`, `atlasNotes.lastId: 0`, `atlasFleet.lastId: 0`, zero
`vesselId` on every vehicle, and `codexRevelationComplete: false`. Sextant ownership,
parked/boarded boats, shrine completion and word knowledge do not backfill those
facts. These are quiet native defaults, not invented visits or data-loss warnings.

`basementChestHistory` is a three-record native daily ledger, not an original
calendar field. `internal/legacy_save/basement_chests.go` uses only an active Lord
British basement `SAVED.GAM` object list: if no non-player CHEST occupies a given
authored source square on floor -1, that source's `lastOpened` conservatively
becomes the imported **saved date**. It emits `basement_chest_saved_date` at
`gam.activeObjects` for each such default. A chest at that square is ambiguous
(it could be a generated child), so conversion preserves it without inventing a
prior opening. Outside-castle/off-floor absence and stale OOL halves leave dates
null. Only the three obsolete target death flags are removed; legacy daily dates
come from this active-object rule, not those flags. Physical objects and already
generated loot retain normal object conversion authority, without loot rerolls.
Authored and generated-child lifetime identity remains separate from physical
trap/value bytes and inspection knowledge.

See [the V24 contract](SAVE_SNAPSHOT_SCHEMA.md#snapshot-24-personal-state-and-chest-lifetimes)
for the native daily-chest fields.

## Object Lists

The original eight-byte record order is base identity, displayed shape, X, Y,
level, contextual number, animation state, and miscellaneous state
([SAVE-0006](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0006)). Classification uses
base identity; displayed shape may be an animation or facing frame. The number
byte is contextual and is never treated as a universal HP field.

Object-list authority depends on the active map:

| Active context | Britannia authority | Underworld authority |
|---|---|---|
| Britannia | `SAVED.GAM` active object list | `SAVED.OOL` Underworld half |
| Underworld | `SAVED.OOL` Britannia half | `SAVED.GAM` active object list |
| Town or dungeon | `SAVED.OOL` Britannia half | `SAVED.OOL` Underworld half |

An active list is never merged with its stale matching OOL half. Slot 0 is the
boarded party vehicle and is not emitted as a map unit. Supported records retain
their source slot as `npcNum`; each native controller receives contiguous
`controllerOrder` values.

Before classifying outdoor records, conversion requires their signed `level` to
match the authoritative list's world (Britannia `0`, Underworld `-1`). Nonempty
off-world records are omitted with `off_world_object_omitted` at the exact
`<source>[<slot>].level` path; they are neither normalized nor moved to the other
world. This follows original noncombat lookup/rendering level checks and the
distinct outdoor list authority in [SAVE-0006](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0006).
Small-map basement and upper-floor records
are not subject to this world filter; loose items remain active-floor scoped.

| Original object | Native representation | Rule |
|---|---|---|
| Horse, carpet, skiff, anchored frigate | `kind: vehicle` | Preserve position and encoded direction; directionless carpet records default to right. Frigates preserve hull and clamp carried skiffs to Redux capacity with a warning. |
| Pirate ship | `kind: enemy`, variant `pirate` | Preserve hull durability and cardinal facing. |
| Outdoor monster or whirlpool | `kind: enemy` | Restore original undamaged base HP because the outdoor contextual number is not HP. |
| Provable active-town Shadowlord | `kind: enemy`, variant `shadowlord` | Requires the saved current Shadowlord identity; ambiguous records are omitted with a warning. |
| Surface chest (base identity `0x01`) | `kind: chest` | Preserve the complete contextual number byte, including lock/trap bit and treasure value. |
| Pending ship-shop delivery | `kind: vehicle`, `pendingDelivery: true` | Synthesize from saved berth X/Y and pending delivery/skiff encoding as in [MERCH-0008](../gamefacts/MERCHANT_SERVICES.md#merch-0008). |
| Safe active-map loose item | `itemStacks` | Convert provisions, canonical equipment, potions, scrolls, moonstones, and source-proven singleton quest items. Invalid payloads warn and are omitted. |
| Inactive-world loose item | none | Omitted with `inactive_loose_item_omitted`; native item stacks are current-map scoped. |
| Ordinary active-town actor | authored small-map reconstruction | The original saved NPC-to-object-slot association is not decoded, so object slot and contextual number cannot safely identify an authored NPC. |
| Corpse, ambiguous Shadowlord, invalid payload, or unknown object | none | Omitted with an exact source path such as `gam.activeObjects[7]`. |

For active towns, conversion carries both the private reconstruction hint and
supported semantic overlays. Hydration retains authored actors and all objects
on inactive floors. On the active floor it replaces generated vehicles, chests,
and supported item-shaped references with active-list authority. Exact or
uniquely identifiable authored vehicles retain their NPC identity and ownership;
other overlays receive collision-free runtime identities. This prevents
duplicate vehicles and resurrection of collected authored objects. The hint is
process-only and never appears in native JSON.

## Startup Authority

Player-facing import calls `legacy_save.ReadBundleContext`,
`legacy_save.Convert`, and native snapshot hydration. The source pair is
read-only. Test fixtures load their checked-in `SAVED.GAM` through the same
conversion, with an empty `SAVED.OOL`; there is no separate one-file loader.

Outdoor map-window offsets are not imported as durable history. Normal original
startup overwrites loaded offsets by initializing from the final party position.
Redux follows that behavior after legacy import and every native hydration,
including player loads and Workbench rollback. The
plane-tagged window and temporary Push arrangement remain runtime-only; saving
and continuing does not reset the live window. See
[SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002) for history-dependent Blink
range and [the native contract](SAVE_SNAPSHOT_SCHEMA.md#capture-and-hydration).

## Decoded Field Coverage

The documentation checker derives leaf paths from `SavedGame` and
`SavedObjects` in `internal/legacy_save/types.go` and requires every path below.
Adding a decoded field without assigning a crosswalk disposition fails
`make validate`.

- **Semantic:** Character identity fields `Characters[].Occupied`, `Characters[].Name`, `Characters[].NameBytes`, `Characters[].Gender`, `Characters[].Class`, `Characters[].Status`, `Characters[].Strength`, `Characters[].Dexterity`, `Characters[].Intelligence`, `Characters[].CurrentMP`, `Characters[].CurrentHP`, `Characters[].MaximumHP`, `Characters[].Experience`, `Characters[].Level`, `Characters[].MonthsAtInn`, `Characters[].ArmorClass`, `Characters[].PartyMap` are validated and normalized into the native roster.
- **Semantic:** Character equipment fields `Characters[].Equipment.Helmet`, `Characters[].Equipment.Armor`, `Characters[].Equipment.Hands`, `Characters[].Equipment.Ring`, `Characters[].Equipment.Amulet` map to stable native item identities.
- **Semantic:** Inventory counters `Inventory.Food`, `Inventory.Gold`, `Inventory.Keys`, `Inventory.Gems`, `Inventory.Torches`, `Inventory.Grapples`, `Inventory.Carpets`, `Inventory.SkullKeys`, `Inventory.Amulet`, `Inventory.Crown`, `Inventory.Sceptre` map to native provisions and quest-item identities.
- **Semantic:** Inventory arrays `Inventory.Equipment`, `Inventory.PreparedSpells`, `Inventory.Scrolls`, `Inventory.Potions`, `Inventory.Reagents` map by validated stable identity.
- **Semantic:** Shard and special-item fields `Inventory.Shards.Source`, `Inventory.Shards.Falsehood`, `Inventory.Shards.Hatred`, `Inventory.Shards.Cowardice`, `Inventory.SpecialItems.Spyglass`, `Inventory.SpecialItems.HMSCapePlans`, `Inventory.SpecialItems.Sextant`, `Inventory.SpecialItems.PocketWatch`, `Inventory.SpecialItems.BlackBadge`, `Inventory.SpecialItems.WoodenBox` map to native progression and quest-item state.
- **Semantic:** Moonstone fields `Moonstones[].X`, `Moonstones[].Y`, `Moonstones[].Map`, `Moonstones[].Level` map source location sentinels to native found/buried state.
- **Exact:** Calendar and map fields `Calendar.Year`, `Calendar.Month`, `Calendar.Day`, `Calendar.Hour`, `Calendar.Minute`, `Position.Location`, `Position.Floor`, `Position.X`, `Position.Y` retain their validated values.
- **Exact:** Durable scalar fields `SkullKeysFoundDay`, `ReagentCacheDays`, `OldHour`, `CombatActionTimeCount`, `WindUpdateCount`, `Karma`, `LastGave`, `LastCamped`, `LastWandererMonth`, `SpellDuration`, `MagicLight`, `TorchTurns`, `OpenDungeons`, `DungeonRooms`, `Smashed`, `Easy` retain their validated values in semantic native fields.
- **Semantic:** Contextual scalar and bitset fields `PartyCount`, `SearchFlags`, `ActiveSpell`, `ActivePlayer`, `PlayerShape`, `Wind`, `ShadowlordLords`, `LastCalledShadowlord`, `CurrentShadowlord`, `DeadNPCs`, `MetNPCs` are normalized into native party, effects, vehicle, progression, and NPC state.
- **Omitted:** Transient action fields `Spell`, `Magic` emit `transient_action_flags_omitted` when set.
- **Semantic:** Shrine and ship fields `Shrines.ActiveBits`, `Shrines.CompletedBits`, `Shrines.Closed`, `Ship.X`, `Ship.Y`, `Ship.Sailing` map to progression and pending-delivery/vehicle state.
- **Exact:** Active dungeon fields `Dungeon.Cells`, `Dungeon.LastDirection`, `Dungeon.Facing` retain their validated values; `Dungeon.Active` selects whether that state is present.
- **Omitted:** `Dungeon.Type` is redundant with the validated dungeon location and has no separate native authority.
- **Semantic:** `Dungeon.Skiffs` is interpreted as the original pending ship-delivery marker and carried-skiff payload.
- **Semantic:** Active object fields `ActiveObjects[].Tile`, `ActiveObjects[].Shape`, `ActiveObjects[].X`, `ActiveObjects[].Y`, `ActiveObjects[].Level`, `ActiveObjects[].Number`, `ActiveObjects[].Animation`, `ActiveObjects[].Misc` are classified contextually into semantic units/items or reported omissions.
- **Semantic:** OOL Britannia object fields `Objects.Britannia[].Tile`, `Objects.Britannia[].Shape`, `Objects.Britannia[].X`, `Objects.Britannia[].Y`, `Objects.Britannia[].Level`, `Objects.Britannia[].Number`, `Objects.Britannia[].Animation`, `Objects.Britannia[].Misc` are classified contextually into semantic units or reported inactive-item omissions.
- **Semantic:** OOL Underworld object fields `Objects.Underworld[].Tile`, `Objects.Underworld[].Shape`, `Objects.Underworld[].X`, `Objects.Underworld[].Y`, `Objects.Underworld[].Level`, `Objects.Underworld[].Number`, `Objects.Underworld[].Animation`, `Objects.Underworld[].Misc` are classified contextually into semantic units or reported inactive-item omissions.

## Behavioral References

- Original layout: [SAVE-0001](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0001)
  through [SAVE-0007](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0007).
- Object-list ownership: [SAVE-0006](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0006).
- Chests and loose items: [CMD-0007](../gamefacts/COMMANDS_REFERENCE.md#cmd-0007)
  and [CMD-0008](../gamefacts/COMMANDS_REFERENCE.md#cmd-0008).
- Vehicle boarding/Exit: [VEH-0002](../gamefacts/VEHICLES.md#veh-0002) and
  [VEH-0003](../gamefacts/VEHICLES.md#veh-0003); delivery:
  [MERCH-0008](../gamefacts/MERCHANT_SERVICES.md#merch-0008).
- Dungeon wanderer and movement: [DNG-0010](../gamefacts/DUNGEON_SYSTEM.md#dng-0010).

These references establish original format and behavior boundaries. Native-only
defaults, omission warnings and hydration choices above are conversion policy,
not proof that every original working field has a portable interpretation.
