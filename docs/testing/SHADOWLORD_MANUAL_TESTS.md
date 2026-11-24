# Shadowlord System Manual Tests

These scenarios validate the current Shadowlord implementation through normal gameplay and the browser Debug Workbench. The removed text console and its command grammar are not used.

## Prerequisites

- Start the game with `-debug-workbench` and open the loopback Workbench URL.
- Capture a baseline in **Saves** before destructive tests.
- Use a gameplay scene with no active modal dialog, workflow, combat, or vehicle transition.
- Keep the game window visible for gameplay messages, NPC behavior, and visual effects.

## Workbench Surfaces

| Instrument | Relevant typed commands | Use |
|---|---|---|
| Calendar | `calendar.status`, `calendar.set-time`, `calendar.set-day` | Stage a 23:59 boundary and inspect the resulting date |
| World Diagnostics | `shadowlord.status` | Inspect life state, active presence, summoning, and persistent assignments |
| World Navigator | `world.position.status`, `world.position.set` | Review and atomically apply a location, valid floor, and bounded coordinate draft |
| Quest Inventory | `quest.item.status`, `quest.item.ensure`, `quest.shard.use` | Grant, inspect, and use shards through typed production workflows |
| Logging Control | Typed logging status and mutation commands | Enable `gamestate` diagnostics when needed |
| Saves | Typed snapshot commands | Restore a known living-Shadowlord baseline between destructive cases |

## Setup Semantics

Applying a **World Navigator** destination draft performs an immediate
synchronized setup. A location-changing draft runs authoritative map-transition
mutations, including automatic Shadowlord NPC, map, and AI effects, but it does
not run the normal player-entry presentation workflow. Do not use it to verify
the atmosphere dialog or entry fade.

An exact draft within the current map changes coordinates only. Moving to Y=4
after entering a town does not simulate jail entry and does not rerun town-entry
checks.

To test normal entry presentation, select the town in **World Navigator** and read its world-map entrance placement, teleport to a passable adjacent overworld coordinate, and walk onto the entrance in the game window.

## Quick Bug Check

Use this table for a short smoke test of the persistent-assignment work. The
numbered sections below provide full setup and troubleshooting details.

| Check | Short procedure | Expected result | Pass/Fail and notes |
|---|---|---|---|
| New-game state | Start a clean native game and refresh `shadowlord.status` several times. | All three Shadowlords are alive and unassigned; status reads do not create assignments. | |
| First midnight | Set time to 23:59 and perform a normal one-minute action. | Each living Shadowlord receives a different virtue town; none is assigned to the party's current town. | |
| Defeated-state exclusion | Destroy one Shadowlord's matching shard, then cross midnight. | The defeated Shadowlord remains defeated and unassigned while living Shadowlords receive unique assignments. | |
| Read stability | Record the ledger, refresh status repeatedly, then change the calendar day directly. | The assignment ledger does not change until gameplay crosses another midnight. | |
| Save continuation | Save the assigned state, cross midnight and record the ledger, reload, then repeat the same action. | Reload restores the saved ledger and replay produces the same next ledger. | |
| Assigned-town entry | Walk normally from the overworld into one assigned town. | The atmosphere message, matching Shadowlord NPC, automatic presence, and identity-specific town effects appear. | |
| Unassigned-town entry | Walk normally into a town absent from the ledger. | No Shadowlord message, NPC, automatic presence, or town effects appear. | |
| Midnight while inside | Enter an assigned town, cross midnight without leaving, then inspect status and the loaded map. | The persistent ledger moves, but the loaded NPC, presence, NPC overrides, and map effects remain until the party leaves. | |
| Tactical handoff | Approach an automatic or Yell Shadowlord, with and without the Sceptre. | Adjacent contact enters map `0x0a`; an owned Sceptre is reclaimed once before tactical play. | |

## 1. Persistent Midnight Assignments

Assignments are authoritative state. Reading status or changing the calendar
day directly does not recompute them; only crossing midnight through gameplay
time moves living Shadowlords.

### 1.1 New Game Starts Unassigned

1. Start from a clean native new game before its first midnight.
2. Open **World Diagnostics** and inspect the assignment ledger.

Expected:

- All three Shadowlords are alive and unassigned.
- Refreshing diagnostics consumes no gameplay RNG and creates no assignments.

### 1.2 Midnight Moves Living Shadowlords

1. Record the current assignments in **World Diagnostics**.
2. If the party is in a virtue town, note that current location.
3. In **Calendar**, set the time to 23:59.
4. Perform a normal one-minute gameplay action in the game window.
5. Refresh **World Diagnostics** several times.

Expected:

- Each living Shadowlord has one unique assignment in Moonglow through New Magincia.
- No assignment equals the party's current town.
- The ledger remains unchanged on repeated reads.
- Directly setting another calendar day does not reroll the ledger.

### 1.3 Native Save Preserves Continuation

1. Capture a native save after midnight assignments exist.
2. Advance to another midnight and record the resulting ledger.
3. Restore the capture, repeat the same gameplay steps, and inspect the ledger.

Expected:

- Loading restores the captured assignment ledger.
- Repeating the same post-load actions produces the same next ledger.
- Status reads do not perturb the saved gameplay RNG continuation.

### 1.4 Defeated Shadowlords Are Excluded

1. Complete one matching shard-destruction test from section 4.
2. Advance through one or more midnight boundaries and refresh **World Diagnostics**.

Expected:

- The defeated Shadowlord remains marked defeated.
- It does not appear in any town assignment.
- Living Shadowlords remain uniquely assigned.

## 2. Automatic Town Presence

### 2.1 Normal Entry Applies Presence

1. Use **World Diagnostics** to choose one assigned town.
2. Exit to the overworld in the game window.
3. Use **World Navigator**'s world-entrance placement to stage beside that town's overworld entrance.
4. Walk onto the entrance normally.
5. Dismiss the presence dialog and refresh **World Diagnostics**.

Expected:

- The game displays the assigned Shadowlord's atmosphere message after the entry sequence.
- The dedicated presence cue plays once with the slow dark flash before the
  atmosphere dialog; no successful-Yell summon cue or fizz is presented.
- World Diagnostics reports that Shadowlord as **Automatic Presence**.
- A Shadowlord NPC appears at the town's fixed spawn position.
- Eligible crops and trees may be replaced according to the per-tile survival roll.
- Dismissing the dialog leaves no flash, modal, sound, or input block active.

### 2.2 Astaroth Affects Eligible NPCs

1. Select a town currently assigned to Astaroth.
2. Enter normally as in test 2.1.
3. Observe movable civilian NPCs and talk to affected NPCs.

Expected:

- Eligible NPCs independently have a one-in-two chance to become hostile.
- Excluded NPC types and unaffected rolls remain normal.
- Talking to an affected hostile NPC damages the party, displays `"Begone, vermin!"`, and changes that NPC to flee behavior.

### 2.3 Nosfentor Affects Eligible NPCs

1. Select a town currently assigned to Nosfentor.
2. Enter normally.
3. Observe and talk to affected civilian NPCs.

Expected:

- Eligible NPCs independently have a one-in-two chance to flee.
- Talking to an affected NPC displays `"Don't hurt me! Please go away!"`.
- The conversation does not apply Astaroth's party-damage sequence.

### 2.4 Faulinei Does Not Override NPC AI

1. Select a town currently assigned to Faulinei.
2. Enter normally and talk to civilian NPCs.

Expected:

- The falsehood atmosphere message appears.
- Automatic presence and the Shadowlord NPC are active.
- Faulinei does not apply hostile or flee overrides to civilian NPCs.

Guild, Arms, Reagent, Horse, Ship, and Tavern purchases, paid healer services,
and charged Inn Rest/Pick Up actions exercise automatic-Faulinei shop theft on
the active merchant implementations. Tavern coverage includes meals, drinks,
Wine, and normally completed or food-saturated provisions; partial provision
fulfillment and charity intentionally return before Faulinei.
Completing an ordinary non-merchant NPC conversation under automatic Faulinei
presence also runs the original provision/equipment/potion/scroll/gold theft
selection. Cancellation, merchant-handler conversations, and Yell-summoned
Faulinei do not run that conversation theft.

### 2.5 Unassigned Town Has No Presence

1. Use **World Diagnostics** to select a town absent from the assignment ledger.
2. Enter it normally from the overworld.

Expected:

- No Shadowlord atmosphere dialog appears.
- World Diagnostics reports no automatic presence.
- No automatic Shadowlord NPC or Shadowlord map effects are applied.

### 2.6 Presence Survives Floor Changes

1. Enter an assigned multi-floor town normally.
2. Note affected NPC behavior and automatic presence.
3. Change floors through normal Klimb gameplay, then return.
4. Refresh **World Diagnostics**.

Expected:

- Automatic presence remains active while changing floors within the same location.
- NPC resets preserve the Shadowlord override state expected for the location.
- Leaving the location clears automatic presence; re-entry reads the current persistent assignment.

### 2.7 Midnight Does Not Refresh A Loaded Town

1. Enter an assigned town normally and confirm its automatic presence and spawned Shadowlord.
2. Set the time to 23:59 and perform a normal one-minute action without leaving town.
3. Refresh **World Diagnostics** and inspect the loaded map.

Expected:

- The persistent assignment ledger moves and excludes the loaded town.
- Automatic presence, the existing Shadowlord NPC, NPC overrides, and map changes remain as they were at entry.
- Leaving and entering a location again applies the new assignment state.

## 3. Yell Summoning at Keeps

Any living Shadowlord can be summoned at any of the three keeps. Summoning requires player Y to be at least 2 and no other yell-summoned Shadowlord to be active.

| Keep | Exact floor | Flame coordinate | Matching destruction pair |
|---|---:|---:|---|
| Lycaeum | 2 | 15,9 | Faulinei / Shard of Falsehood |
| Empath Abbey | 1 | 15,3 | Astaroth / Shard of Hatred |
| Serpent's Hold | -1 | 15,16 | Nosfentor / Shard of Cowardice |

### 3.1 Summon at Each Keep

For each keep:

1. In **World Navigator**, select the keep as the destination.
2. Choose **Arbitrary exact position**, then select the floor from the row above.
3. Enter the listed coordinate or another clear coordinate with Y at least 2, then apply the complete draft.
4. In the game window, press Y and enter `FAULINEI`, `ASTAROTH`, or `NOSFENTOR`.
5. Refresh **World Diagnostics**.

Expected:

- The game reports `A shadowlord appears!`, plays one finite summon cue, and
  presents one fizz/sparkle reveal on the exact spawned Shadowlord tile.
- The NPC spawns two tiles north of the Avatar on the current floor.
- The successful Yell does not advance game time, and the Shadowlord remains at
  that initial position until a subsequent action advances NPCs.
- Repeat on foot and carpet, in each mounted cadence phase. Wait through the
  entire appearance and input release: no extra step or encounter may occur.
  From directly south of the matching Flame, with no temporal modifier, one
  explicit Pass should move the lord onto the Flame without attacking in the same
  step; the next matching shard Use should remain available. Record the build,
  platform and observed result. Injected-input tests are not this native check.
- World Diagnostics reports the selected Shadowlord as summoned.
- Yell summoning does not apply automatic town NPC or crop effects.
- Yell summoning does not display the automatic-presence `An air of ...`
  atmosphere message or play the separate presence cue.
- A second summon attempt while one is active has no effect and completes one
  normal failed-Yell turn, with no additional summon fizz or cue.

Restore the baseline or leave the keep before testing the next summon.

### 3.2 Yell Outside a Keep

1. Apply a **World Navigator** draft for Britain.
2. Ensure the Avatar's Y coordinate is at least 2.
3. Yell a living Shadowlord's name.

Expected:

- The game reports `No effect!`
- World Diagnostics reports no yell-summoned Shadowlord.
- The yell consumes the normal failed-yell turn.
- No summon fizz, summon cue, or presence presentation runs.

## 4. Shard Destruction

Use a fresh baseline for each destructive case.

### 4.1 Correct Shadowlord, Flame, and Shard

For each row in the keep table:

1. Apply one **World Navigator** destination draft for the keep, exact floor, and exact flame coordinate.
2. In **Quest Inventory**, grant the matching shard if absent.
3. Yell the matching Shadowlord's name in the game window.
4. Confirm time did not advance and the matching Shadowlord spawned two tiles north.
5. Take one normal action and confirm the Shadowlord moves onto the visible blue
   Flame exactly one tile north.
6. In the game window, press U and select the matching Shard from the production Use dialog.

Expected:

- The production workflow presents the selected Shard and pulse, the matching
  Flame, three earthquakes, seven impacts, the doom message, and victory cue.
- One workflow-owned `SoundShardRitualEarthquake` loop fades in before the three
  shakes and fades out when the ritual completes or is cancelled. No stacked
  one-shots or lingering ritual audio remain, and finalization does not stop
  unrelated `SoundEarthquake` one-shots.
- No teleport-to-Flame or dissolve sequence runs.
- Exactly one selected Shard is consumed.
- The Shadowlord NPC disappears.
- World Diagnostics marks that Shadowlord defeated and no longer summoned.
- Later midnight ledgers exclude the defeated Shadowlord.
- The Use command completes one normal turn only after the doom presentation.

### 4.2 Wrong Shard or Wrong Shadowlord

1. Stage at one exact flame coordinate.
2. Grant a Shard that does not match that Flame, or summon a Shadowlord that does not match it.
3. Press U in the game window and select that exact Shard.

Expected:

- The game reports no effect.
- The shard remains in inventory.
- The summoned Shadowlord remains alive and present.
- The failed Use completes one normal turn after its presentation.

### 4.3 Late Physical-Object Validation

1. Stage a matching Shard, Flame, and summoned identity, but use the Shard
   immediately while the Shadowlord is still two tiles north.
2. Repeat after one normal action moves it exactly one tile north.

Expected:

- The first attempt reaches the Flame message and three earthquakes, then ends
  without an additional no-effect message, impacts, consumption, or defeat.
- The second attempt completes all seven impacts and the destruction.
- Each selected Use completes one normal turn after the ritual ends.

### 4.4 Multiple Owned Shards

1. Grant all three Shards.
2. Open the production Use dialog and deliberately select a non-first Shard.
3. Repeat at its wrong Flame and matching Flame setup.

Expected:

- The workflow always presents and validates the exact selected identity; it
  never substitutes the first owned Shard.
- Wrong-Flame attempts consume none of the three Shards.

After destruction, revisit the Shard's original Underworld location and confirm
that it does not rematerialize. The Workbench Use Shard command queues this same
production ritual after its stale-review checks.

## 5. Tactical Combat Handoff

Repeat the identity-sensitive cases with Faulinei, Astaroth, and Nosfentor.
Restore the baseline between Sceptre cases.

### 5.1 Automatic Presence Adjacency

1. Enter a town with automatic presence and locate its physical Shadowlord.
2. Stand four Manhattan tiles away and complete one normal action.
3. Repeat from three and two tiles away, observing one-tile approaches.
4. Let the Shadowlord move adjacent, then complete one later normal action.

Expected:

- Distance four does not move; distances three and two approach one tile.
- Moving adjacent does not start combat during that same NPC phase.
- The later pre-movement adjacency check enters BRIT.CBT map `0x0a` with exactly
  one Shadowlord and no friend substitution.

### 5.2 Yell Presence and Explicit Attack

1. At a Keep, Yell one living Shadowlord name and confirm the source remains two
   tiles north until a later NPC phase.
2. Complete one normal action so it approaches, then either complete a later
   action for automatic contact or press A and select its adjacent direction.
3. Repeat the explicit Attack and press Escape instead of a direction.
4. Keep Escape held briefly, release it, then press a fresh command key. Separately
   repeat with Space to exercise Pass rather than cancellation.

Expected:

- Successful Yell itself neither moves the source nor starts combat.
- Later contact and explicit Attack use the same map-`0x0a` encounter while
  retaining the summoned identity.
- Escape cancels the uncommitted surface Attack, reports `Cancelled`, and returns
  to normal input without combat, time, actor processing, resource use, or gameplay
  RNG. The held cancellation key does not open the Escape menu; the next fresh
  command works normally.
- Space is still Pass: this Keep's small-map Attack completes its ordinary turn
  and can therefore let the adjacent Shadowlord initiate contact. Do not apply
  outdoor Attack's free-Pass exception to this small-map case, or these surface
  Escape rules to a combat/dungeon prompt.

### 5.3 Sceptre Reclamation and Return Outcomes

1. Grant the Sceptre and one Shard, then begin either automatic or Yell combat.
2. Confirm `The Sceptre is reclaimed!` and one pulse cue occur before the first
   tactical actor acts.
3. Exercise victory, defeat or flight, and full-party evacuation on fresh
   baselines; include one total-party-kill case.
4. Repeat entry without owning the Sceptre.

Expected:

- The exact physical source disappears before tactical play.
- An owned Sceptre is removed once. Entry without it has no reclamation message
  or pulse.
- No conclusion restores the Sceptre, consumes the Shard, or marks the
  Shadowlord permanently defeated.
- Automatic presence remains governed by its town assignment. A Yell source
  does not reappear after return.

## 6. Jail Entry Bypass

The implemented arrest path sends the party to Yew jail at 25,4 on floor 0. It does not run the normal town-entry Shadowlord check.

1. Advance midnight boundaries until **World Diagnostics** reports an assignment in Yew.
2. Enter a different, unassigned town with guards.
3. In the game window, attack a defenseless townsperson to commit the town-attack crime and turn the guards hostile.
4. Approach or talk to a guard and surrender to the arrest prompt.
5. After the jail sequence, refresh **World Navigator** and **World Diagnostics**.

Expected:

- World Navigator reports Yew at 25,4 on floor 0.
- No Yew atmosphere dialog or automatic Shadowlord NPC appears during jail transport.
- World Diagnostics reports no automatic presence.

Contrast case:

1. Exit Yew to the overworld.
2. Re-enter Yew normally without crossing another midnight.

Expected:

- The assigned Yew presence now runs because this is a normal entry rather than jail transport.

## 7. Completion and Cleanup

### 7.1 All Shadowlords Defeated

1. Destroy all three Shadowlords with their matching keep, flame, and shard.
2. Advance through several midnight boundaries.
3. Refresh **World Diagnostics** after each boundary.

Expected:

- All three are marked defeated.
- The assignment ledger remains empty.
- No town receives automatic Shadowlord presence.

### 7.2 Presence Clears on Location Change

1. Enter an assigned town and confirm automatic presence.
2. Leave for the overworld or another location.
3. Refresh **World Diagnostics**.

Expected:

- Automatic presence clears when leaving the location.
- Re-entering an assigned town reapplies presence from the persistent ledger.

### 7.3 Presentation Lifecycle Cleanup

1. Open the small-map Yell input and cancel it without submitting text.
2. Confirm no summon cue, fizz, atmosphere message, or lingering input block appears.
3. Perform a successful keep summon and, while its non-blocking fizz is visible,
   use the normal Escape menu to return to the introduction without saving.
4. Re-enter gameplay and complete one automatic-presence dialog and one failed
   Shard ritual normally.

Expected:

- Scene replacement leaves no summon audio or temporary fizz visible or audible.
- Presence dismissal leaves no flash, sound, modal, or input block.
- The failed Shard ritual leaves no dedicated ritual earthquake audio, shake,
  modal, or input block and does not consume the Shard.
- Forced stale-state, priority-interruption, and mid-workflow disposal paths are
  automated presentation lifecycle checks; manual work here is perceptual only.

## Results

Production tactical entry, map `0x0a`, Sceptre reclamation, explicit Attack, and
return behavior are included above. Bounded monster-special behavior and Doom
entrance combat are covered by automated combat evidence and
add no manual steps here.

| Test | Pass/Fail | Notes |
|---|---|---|
| Initial unassigned state | | |
| Persistent midnight movement | | |
| Native save continuation | | |
| Defeated Shadowlord exclusion | | |
| Normal assigned-town entry | | |
| Astaroth NPC behavior | | |
| Nosfentor NPC behavior | | |
| Faulinei NPC behavior | | |
| Unassigned town | | |
| Floor-change persistence | | |
| Loaded-town midnight stability | | |
| Keep yell summoning | | |
| Yell outside a keep | | |
| Automatic adjacency combat | | |
| Yell-origin and explicit-Attack combat | | |
| Sceptre reclamation and combat returns | | |
| Correct shard destruction | | |
| Wrong shard or Shadowlord | | |
| Jail entry bypass | | |
| All Shadowlords defeated | | |
| Presence cleanup | | |
| Summon presentation cleanup | | |
| Shard presentation cleanup | | |
