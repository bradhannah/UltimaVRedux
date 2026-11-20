# Reagent mixing

The eight-reagent preparation system and its 48 formulas are described here.
Spell effects belong to [Magic spells](MAGIC_SPELLS.md). Original assessments
refer to the inspected DOS-era material labelled 1.16, not an identified retail
executable. Random requests are written R(a,b); endpoint conventions do not
establish the unavailable external generator's distribution or evaluation order.

### MIX-0001

**Reagent encoding and complete recipe catalog**

**Kind:** rule
**Uses:** INV-0001
**Original:** Established
**Redux:** Matches

The original recipe byte uses high-bit-first inventory order. Redux represents
the same ingredient sets using enum-index bits, beginning with Ash at bit zero;
numeric Redux masks must not be compared directly to the original bytes.

Reagent labels use the full Redux display vocabulary. **Sulfurous Ash** is an
editorial expansion of original shop **Sulfur Ash**; **Nightshade** follows the
original one-word spelling. Original shop **Mandrake** and gathering **mandrake
root** identify the same ingredient, displayed as **Mandrake Root**. These label
choices do not change ingredient indexes, recipe masks or the scoped recipe match.

| Inventory index | Reagent | Abbreviation | Original bit |
|---:|---|---|---:|
| 0 | Sulfurous Ash | SA | 0x80 |
| 1 | Ginseng | GS | 0x40 |
| 2 | Garlic | GA | 0x20 |
| 3 | Spider Silk | SS | 0x10 |
| 4 | Blood Moss | BM | 0x08 |
| 5 | Black Pearl | BP | 0x04 |
| 6 | Nightshade | NS | 0x02 |
| 7 | Mandrake Root | MR | 0x01 |

Each selected ingredient costs one unit per requested mixture. Ingredient order
does not matter, but missing or extra types make an incorrect recipe.

| Spell index | Circle | Spell | Ingredients | Original mask |
|---:|---:|---|---|---:|
| 0 | 1 | In Lor | SA | 0x80 |
| 1 | 1 | Grav Por | SA, BP | 0x84 |
| 2 | 1 | An Zu | GS, GA | 0x60 |
| 3 | 1 | An Nox | GS, GA | 0x60 |
| 4 | 1 | Mani | GS, SS | 0x50 |
| 5 | 1 | An Ylem | GA, BM | 0x28 |
| 6 | 2 | An Sanct | SA, BM | 0x88 |
| 7 | 2 | An Xen Corp | SA, GA | 0xA0 |
| 8 | 2 | Rel Hur | SA, BM | 0x88 |
| 9 | 2 | In Wis | NS | 0x02 |
| 10 | 2 | Kal Xen | SS, MR | 0x11 |
| 11 | 2 | In Xen Mani | GS, GA, MR | 0x61 |
| 12 | 3 | Vas Lor | SA, MR | 0x81 |
| 13 | 3 | Vas Flam | SA, BP | 0x84 |
| 14 | 3 | In Flam Grav | SA, SS, BP | 0x94 |
| 15 | 3 | In Nox Grav | SS, BP, NS | 0x16 |
| 16 | 3 | In Zu Grav | GS, SS, BP | 0x54 |
| 17 | 3 | In Por | SS, BM | 0x18 |
| 18 | 4 | An Grav | SA, BP | 0x84 |
| 19 | 4 | In Sanct | SA, GS, GA | 0xE0 |
| 20 | 4 | In Sanct Grav | SS, BP, MR | 0x15 |
| 21 | 4 | Uus Por | SS, BM | 0x18 |
| 22 | 4 | Des Por | SS, BM | 0x18 |
| 23 | 4 | Wis Quas | SS, NS | 0x12 |
| 24 | 5 | In Bet Xen | SA, SS, BM | 0x98 |
| 25 | 5 | An Ex Por | SA, GA, BM | 0xA8 |
| 26 | 5 | In Ex Por | SA, BM | 0x88 |
| 27 | 5 | Vas Mani | GS, SS, MR | 0x51 |
| 28 | 5 | In Zu | GS, SS, NS | 0x52 |
| 29 | 5 | Rel Tym | SA, BM, MR | 0x89 |
| 30 | 6 | In Vas Por Ylem | SA, BM, MR | 0x89 |
| 31 | 6 | Quas An Wis | NS, MR | 0x03 |
| 32 | 6 | In An | SA, GA, MR | 0xA1 |
| 33 | 6 | Wis An Ylem | SA, MR | 0x81 |
| 34 | 6 | An Xen Ex | SS, BP, NS | 0x16 |
| 35 | 6 | Rel Xen Bet | SA, SS, NS, MR | 0x93 |
| 36 | 7 | Sanct Lor | BM, NS, MR | 0x0B |
| 37 | 7 | Xen Corp | BP, NS | 0x06 |
| 38 | 7 | In Quas Xen | SA, GS, SS, BM, MR | 0xD9 |
| 39 | 7 | In Quas Wis | NS, MR | 0x03 |
| 40 | 7 | In Nox Hur | SA, BM, NS | 0x8A |
| 41 | 7 | In Quas Corp | GA, NS, MR | 0x23 |
| 42 | 8 | In Mani Corp | SA, GS, GA, SS, BM, MR | 0xF9 |
| 43 | 8 | Kal Xen Corp | GA, SS, BM, MR | 0x39 |
| 44 | 8 | In Vas Grav Corp | SA, NS, MR | 0x83 |
| 45 | 8 | In Flam Hur | SA, BM, MR | 0x89 |
| 46 | 8 | Vas Rel Por | SA, BP, MR | 0x85 |
| 47 | 8 | An Tym | GA, BM, MR | 0x29 |

The scoped Redux match is the logical ingredient sets, including In Quas Xen's
five ingredients without Nightshade. Native continuation normalizes persisted
known-recipe selections to that canonical set. The non-player Nox effect is not
a 49th mixable spell.

**Implementation:** `internal/references/spell_reagents.go`, `internal/references/item_reagent_data.go`
**Tests:** `internal/references/spell_reagents_fidelity_test.go`, `internal/references/reagent_test.go`

### MIX-0002

**Preparation, expenditure, and stock cap**

**Kind:** rule
**Uses:** MIX-0001, INV-0005
**Original:** Established
**Redux:** Matches

Mix is admitted outdoors, in towns, and in dungeons; tactical combat rejects it
and reprompts the same actor. The original sequence is:

1. Sum the eight reagent stocks; zero stock stops before spell input.
2. Enter a spell. Empty/cancelled input stops. Invalid nonempty syllables retain
   an invalid spell sentinel and still permit the experiment.
3. Show only owned reagent types; manually toggle types, accept with M, or abort
   with Escape. There is no automatic formula selection.
4. Request at most two quantity digits. Zero cancels; insufficient stock for any
   selected type repeats the quantity prompt.
5. A zero ingredient mask changes nothing. Otherwise perform ten update ticks
   on surface maps, or ten wait ticks in the dungeon path, then subtract the
   entire requested amount from every selected type.
6. Only now check validity and exact recipe equality. Success adds the requested
   amount and caps prepared stock at 99; failure triggers MIX-0003 without refund.

For example, preparing five correct mixtures when 97 already exist consumes
five of every selected ingredient but adds only two charges. The cap follows
deduction. Redux's authoritative mixing operation preserves this cost/cap order
and validates stock and a nonzero eight-bit selection before mutation. The
match is this operation, not the original input/presentation lifecycle.

**Implementation:** `internal/game_state/action_mix.go`
**Tests:** `internal/game_state/action_mix_feedback_test.go`

### MIX-0003

**Failed experiment and primary-target uncertainty**

**Kind:** rule
**Uses:** RNG-0001, ENV-0001
**Original:** Incomplete
**Redux:** Known discrepancy

An invalid spell or incorrect recipe spends the ingredients, then selects a
failure trap. The noncombat trap selector has eight outcomes: three Acid, two
Poison, two Bomb, and one Gas. Those are weights, not proof of a uniform external
random generator; 3/8, 2/8, 2/8, and 1/8 are conditional probabilities only.

| Trap | Effect |
|---|---|
| Acid | One transformed d30 of damage to the primary member. |
| Poison | Poison the primary member. |
| Bomb | Separate R(1,8) damage to each active non-Dead member. |
| Gas | Invoke poisoning across all six party slots. |

The original primary-target search accepts the first Good or Poisoned active
member. Sleeping is not eligible. If none exists, the search leaves a global
target index unchanged; the caller ignores failure and still executes the trap.
Acid/Poison can therefore use stale state. This is not a safe, specified fallback.

Redux selects the same first eligible member when one exists, and uses the
transformed d30 for Acid. With no eligible member it returns before trap selection
or effect, avoiding the stale-index path. This is a concrete difference; the
scoped normal-case match must not be expanded to the all-sleeping/dead edge case.
Bomb/Gas retain their separate party-sweep logic when a trap is executed.

**Implementation:** `internal/game_state/boom.go`, `cmd/ultimav/gamescene_dialogs.go`

### MIX-0004

**Redux formula knowledge and preparation interface**

**Kind:** rule
**Uses:** MIX-0001, MIX-0002
**Original:** Established
**Redux:** Intentional deviation

The original Mix interaction stores no learned-recipe state and never fills in
ingredients. Redux persists formula knowledge and can select known ingredients
to reduce repeated manual entry while retaining experimentation and its costs.
Book-of-Lore formulas are available in all three spell views; learned hidden
spells retain confirmed/uncertain knowledge. An undiscovered recipe is not exposed
merely by browsing. Successful experimentation confirms knowledge.

New-game templates do not infer taught formulas from prepared stock. Legacy
progressed-save imports retain their knowledge inference. Grendel and Lord Stuart
discoveries use their semantic formula boundaries; Chronicle projects their
persisted knowledge source/date rather than storing duplicate discovery events.

The spell finder accepts canonical syllable initials and name/effect search, with
All known, Prepared, and Mixable filters. Mixable requires a confirmed formula,
sufficient ingredients, and room for another charge. Mix opens on Mixable when
anything can be mixed and on All known otherwise. Mixable is a convenience, not a
wall: initials or search words that match no mixable spell widen the list to All
known, where the spell's row and details say why it cannot be mixed. Manual
mixing still permits the full-cost cap behavior. Exact abbreviations precede
longer prefixes: In Zu wins over In Zu Grav. Ctrl+M from Cast or Z-stats opens
preparation for the selected spell; Escape returns.

Preparation has two tabs, changed with Tab. Recipe mixes the recorded recipe and
lists only its ingredients, which cannot be changed there. Experiment lists all
eight ingredients with tick boxes, the recorded recipe ticked to begin with, and
is the only place a different mixture can be chosen; a spell with no recorded
recipe has the Experiment tab alone. A top line states, before commitment,
whether the mixture is ready, untested, short of a named ingredient, or would
lose charges to the 99 cap. It informs and never blocks: untested and wasteful
mixes stay allowed at full cost.

Preparation defaults to one and Escape cancels, replacing the original zero-to-
cancel quantity prompt. Left and Right step the quantity within what the
ingredients allow. M sets the largest quantity that the ingredients allow and the
prepared stock can hold. For maxima 1–9 a valid digit replaces the quantity;
otherwise two digits are accepted and a third starts a new number. Over-limit
digits are rejected, not clamped. Backspace/Delete reset to one without modifying
spell syllables; a zero maximum prevents commitment. These are input conveniences,
not discounts for incorrect recipes.

Successful feedback states the spell, the actual charges added, the final stock
and any charges lost to the cap; one success sound follows creation. From Mix's
own list it is one line on the list's header as the list returns: the typed
initials are cleared, and the mixed spell keeps its row, even if it can no longer
be mixed, until the list is next typed into or filtered. A mix opened from Cast or
Z-stats, which has no Mix list to return to, and a mixture that had no recorded
recipe report the same facts in a box that is dismissed. Failure presents the
actual trap without an extra fizzle prefix. No-stock entry is reported before
opening preparation.

**Implementation:** `internal/game_state/action_mix.go`, `internal/game_state/magic_state.go`, `internal/ui/widgets/reagent_mixing_dialog.go`, `internal/ui/widgets/reagent_mixing_window.go`, `internal/ui/widgets/spell_browser.go`
**Tests:** `internal/ui/widgets/reagent_mixing_dialog_test.go`, `internal/game_state/taught_spell_discovery_test.go`
