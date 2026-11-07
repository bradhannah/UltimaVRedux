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
**Redux:** Unverified

Killing a non-PC combat entry returns `floor(monster_type_max_HP/4)+1`; nonlethal
damage returns zero. Inspected ordinary-hit and damaging-poison callers credit
the PC assailant with saturation at 9999. Use type maximum HP, not remaining HP;
there is no inherent whole-party sharing. Melee/player missiles reach that path.
Redux's shared death finalizer covers these and spell kills; exhaustive original
XP-producer and unusual-allegiance equivalence remains outside this rule.

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
