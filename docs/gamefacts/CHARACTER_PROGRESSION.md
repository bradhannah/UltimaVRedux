# Character progression

XP calculation, kill attribution, and advancement are distinct. Original random
bounds do not establish exact stream/distribution, and inspected XP writers are
not an exhaustive audit of every producer.

### PROGRESS-0001

**XP-derived level**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

Start level at 1, take integer XP/100, and repeatedly increment level and halve
that value until zero. Equivalently level is one plus the nonnegative quotient's
bit length. Ranges: level 1=0–99, 2=100–199, 3=200–399, 4=400–799, 5=800–1599,
6=1600–3199, 7=3200–6399, 8=6400–12799. The calculation has no level-eight clamp;
ordinary inspected XP additions cap at 9999. Redux's calculation preserves this
formula; a misleading logarithm comment does not supersede executable arithmetic.

**Implementation:** `internal/game_state/action_camping_wanderer.go`

### PROGRESS-0002

**Kill XP belongs to the PC assailant path**

**Kind:** rule
**Uses:** PROGRESS-0001
**Original:** Established
**Redux:** Intentional deviation

Killing a non-PC combat entry returns `floor(monster_type_max_HP/4)+1`; nonlethal
damage returns zero. Inspected ordinary-hit and damaging-poison callers credit
the PC assailant with saturation at 9999. Use type maximum HP, not remaining HP;
there is no inherent whole-party sharing. Melee/player missiles reach that path.
Redux's shared death finalizer covers these and spell kills; exhaustive original
XP-producer and unusual-allegiance equivalence remains outside this rule, so the
default path is not a verified match.

**REDUX DEVIATION: PROGRESS-0002:** optional `ShareKillExperience`, off by
default, splits each kill's award evenly among the living members of the active
party. The original credits only the member who lands the killing blow, so
members who seldom finish a foe fall behind. Points that do not divide evenly
are carried to the next kill and saved, so shares stay equal and nothing is
lost; Dead members take no share. A kill that pays nothing by default, such as
one landed by a summoned creature, still pays nothing. With the setting off the
killer takes the whole award, as in the original.

**Implementation:** `internal/game_state/combat_experience.go`, `internal/game_state/action_attack.go`
**Tests:** `internal/game_state/deed_experience_test.go`

### PROGRESS-0003

**Wanderer restoration and stat awards**

**Kind:** rule
**Uses:** PROGRESS-0001, CAMP-0005
**Original:** Established
**Redux:** Intentional deviation

For each active member, if not Dead, fill existing maximum HP and set Good, then
calculate level. On **any difference** from stored level, assign it, set both HP
values to 30*level, request R(1,3) for STR/DEX/INT, and add one capped at 30.
One total award applies even across multiple gained levels or downward correction.
Outside that living branch, restore Avatar/Mage MP to INT and Bard MP to integer
INT/2; other classes receive no assignment. Dead spellcasters gain MP without
revival. INT awards precede MP calculation.

**REDUX DEVIATION: PROGRESS-0003:** Redux gives one stat award per upward level
gained and none for downward correction, tying rewards to earned advancement.
Each award rolls only among STR/DEX/INT values below 30, so a capped stat never
absorbs it; with all three at 30 the award and its announcement are omitted. With
no stat at 30 the roll matches the original R(1,3).
It still reconciles levels both ways, restores HP, computes MP after INT awards,
and refreshes Dead members' class MP without reviving them.

**Implementation:** `internal/game_state/action_camping_wanderer.go`
**Tests:** `internal/game_state/action_camping_wanderer_test.go`

### PROGRESS-0004

**Experience for quest deeds**

**Kind:** rule
**Uses:** PROGRESS-0001, PROGRESS-0002, PROGRESS-0003
**Original:** Incomplete
**Redux:** Intentional deviation

Every inspected original experience producer is a kill (PROGRESS-0002). No
inspected quest routine writes experience: completing a shrine quest, reading
the Codex, opening a dungeon, taking a quest item or destroying a Shadowlord
changes no member's experience. The inspected producers are not an exhaustive
audit, so this absence is not established for every routine.

**REDUX DEVIATION: PROGRESS-0004:** Redux pays every member of the active party
a fixed amount for each quest deed, once. A party that follows the quest and
does not hunt otherwise reaches Doom several levels short, with level 8 costing
6400 experience and the best ordinary kill paying 25. The `DeedExperience`
setting chooses None (the original), Lean (the default) or Fuller, which doubles
every Lean amount:

| Deed | Count | Lean, each |
| --- | ---: | ---: |
| A Codex answer read for a shrine quest | 8 | 20 |
| The return to that shrine for its reward | 8 | 20 |
| The final Codex revelation, with all eight quests done | 1 | 200 |
| A dungeon entrance opened for the first time | 8 | 25 |
| Crown, Sceptre, Amulet, Sandalwood Box, or a Shard taken | 7 | 150 |
| A Shadowlord destroyed | 3 | 300 |

The Lean total is 2670 for each member. Every active member is paid the same
amount, the Dead included, with saturation at 9999; members left elsewhere are
not paid. Learning a mantra, being given a quest, a shrine offering and
restoring a ruined shrine pay nothing. Each payment is recorded in the save, so
a deed pays once whatever happens to the quest flags afterwards.

A deed finished while the setting pays nothing stays unpaid. When the setting
next allows, on loading a game or after a change in Configure, every finished
and unpaid deed is paid at the current rate to the party as it then stands. A
paid deed is never paid again at another rate.

Deed experience raises experience only. Levels are still granted by the
Wanderer (PROGRESS-0003); when an award carries a living member's experience
across a level line their granted level has not reached, Redux says that member
is ready for that level.

**Implementation:** `internal/game_state/deed_experience.go`, `cmd/ultimav/gamescene_experience.go`
**Tests:** `internal/game_state/deed_experience_test.go`
