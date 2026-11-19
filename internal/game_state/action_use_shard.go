package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ShardUsageResult indicates the outcome of reviewing or committing a Shard ritual.
type ShardUsageResult int

const (
	// ShardUsageNotAttempted means the selected item is not an owned Shard.
	ShardUsageNotAttempted ShardUsageResult = iota
	// ShardUsageSuccess means every ritual condition matched.
	ShardUsageSuccess
	// ShardUsageNoEffect means the selected Shard cannot destroy a Shadowlord in the current state.
	ShardUsageNoEffect
)

// ShardRitualReview describes the authoritative conditions for one selected Shard.
// AtMatchingFlame is intentionally separate from Result because original item-use rules
// performs the earthquakes before checking the physical Shadowlord object.
// See ref:SHADOW-0005.
type ShardRitualReview struct {
	Shard           references.QuestItem
	Flame           references.FlameOfPrinciple
	Shadowlord      references.Shadowlord
	Target          references.Position
	AtMatchingFlame bool
	Result          ShardUsageResult
	// Experience is the deed experience a successful ritual paid the party.
	Experience DeedExperienceAward
}

// ShardRitualCommit captures the exact gameplay state authorized by Begin.
// Commit consumes the token once, whether the reviewed state is still valid or stale.
type ShardRitualCommit struct {
	state             *GameState
	review            ShardRitualReview
	units             *map_units.MapUnits
	enemy             *map_units.NPCEnemy
	location          references.PlayerLocation
	inventoryQuantity uint16
	progression       references.ShadowlordState
	dateTime          datetime.UltimaDate
	committed         bool
}

// ReviewShardRitual reviews the current ritual state without mutation.
// See ref:SHADOW-0005.
func (g *GameState) ReviewShardRitual(shard references.QuestItem) ShardRitualReview {
	review := ShardRitualReview{
		Shard:      shard,
		Shadowlord: references.NoShadowlord,
		Result:     ShardUsageNotAttempted,
	}
	if !isShardQuestItem(shard) || !g.PartyState.Inventory.QuestItems.HasSome(shard) {
		return review
	}

	review.Result = ShardUsageNoEffect
	location := g.MapState.PlayerLocation
	flame, atFlame := references.IsAtFlamePosition(
		location.Location,
		int(location.Position.X),
		int(location.Position.Y),
		int(location.Floor),
	)
	if !atFlame {
		return review
	}

	review.Flame = flame
	review.Shadowlord = flame.GetShadowlord()
	review.Target = location.Position.Up()
	if flame.GetRequiredShard() != shard {
		return review
	}
	review.AtMatchingFlame = true

	if g.ShadowlordState == nil || !g.ShadowlordState.Summoned ||
		g.ShadowlordState.LastCalled != review.Shadowlord ||
		!g.ShadowlordState.IsAlive(review.Shadowlord) ||
		g.CurrentNPCAIController == nil || g.CurrentNPCAIController.GetNpcs() == nil {
		return review
	}

	enemy := matchingShadowlordAt(
		g.CurrentNPCAIController.GetNpcs(),
		review.Target,
		location.Floor,
		review.Shadowlord,
	)
	if enemy == nil {
		return review
	}

	review.Result = ShardUsageSuccess

	return review
}

// BeginShardRitualCommit revalidates and captures the exact physical Shadowlord
// without mutating gameplay state.
func (g *GameState) BeginShardRitualCommit(shard references.QuestItem) (*ShardRitualCommit, ShardRitualReview) {
	review := g.ReviewShardRitual(shard)
	if review.Result != ShardUsageSuccess {
		return nil, review
	}

	units := g.CurrentNPCAIController.GetNpcs()
	enemy := matchingShadowlordAt(
		units,
		review.Target,
		g.MapState.PlayerLocation.Floor,
		review.Shadowlord,
	)
	if enemy == nil {
		review.Result = ShardUsageNoEffect

		return nil, review
	}
	deadNPCIndex := shadowlordDeadNPCIndex(review.Shadowlord)
	if g.PartyState.IsNPCDead(references.Stonegate, deadNPCIndex) ||
		!g.DateTime.DateOnly().IsValid() {
		review.Result = ShardUsageNoEffect

		return nil, review
	}

	return &ShardRitualCommit{
		state:             g,
		review:            review,
		units:             units,
		enemy:             enemy,
		location:          g.MapState.PlayerLocation,
		inventoryQuantity: g.PartyState.Inventory.QuestItems.Get(shard),
		progression:       *g.ShadowlordState,
		dateTime:          g.DateTime,
	}, review
}

// CommitShardRitual atomically applies the permanent destruction before its
// cancellable impact presentation. Gameplay state that changed after Begin is
// a safe no-effect result; malformed or reused programmer tokens still panic.
func (g *GameState) CommitShardRitual(commit *ShardRitualCommit) ShardRitualReview {
	if commit == nil || commit.state != g || commit.committed {
		panic("invalid Shard ritual commit")
	}
	commit.committed = true
	review := commit.review
	if !g.shardRitualCommitStateMatches(commit) {
		review.Result = ShardUsageNoEffect

		return review
	}

	// Every fallible gameplay condition was revalidated above. The remaining
	// operations cannot fail without an internal single-threaded invariant bug.
	g.CommitCastUseAttempt()
	if !commit.units.RemoveEnemy(commit.enemy) {
		panic("validated Shard ritual target could not be removed")
	}
	if !g.PartyState.Inventory.QuestItems.DecrementByOne(review.Shard) {
		panic("validated Shard ritual inventory could not be consumed")
	}
	// Original item-use rules writes identity bits 0x02/0x04/0x08 into
	// persistent NPC absence[0x1c], the fixed Stonegate row, rather than the temporary Keep slot.
	// See ref:SHADOW-0005.
	g.PartyState.SetDeadNpc(references.Stonegate, shadowlordDeadNPCIndex(review.Shadowlord))
	g.ShadowlordState.Kill(review.Shadowlord)
	g.SystemCallbacks.UI.JournalChanged(journal.ShadowlordsTab, int(review.Shadowlord))
	// Journal metadata may already contain this event; the authoritative defeat
	// still commits, while append records it when absent.
	g.RecordShadowlordDefeatEvent(review.Shadowlord)
	review.Experience = g.settleDeedExperience(false)

	return review
}

func (g *GameState) shardRitualCommitStateMatches(commit *ShardRitualCommit) bool {
	review := commit.review
	if g.MapState.PlayerLocation != commit.location ||
		g.PartyState.Inventory.QuestItems.Get(review.Shard) != commit.inventoryQuantity ||
		g.ShadowlordState == nil || *g.ShadowlordState != commit.progression ||
		g.CurrentNPCAIController == nil || g.CurrentNPCAIController.GetNpcs() != commit.units ||
		g.PartyState.IsNPCDead(references.Stonegate, shadowlordDeadNPCIndex(review.Shadowlord)) ||
		g.DateTime != commit.dateTime {
		return false
	}

	for _, unit := range *commit.units {
		if unit != commit.enemy || !commit.enemy.IsVisible() ||
			commit.enemy.Pos() != review.Target || commit.enemy.Floor() != commit.location.Floor {
			continue
		}
		identity, hasIdentity := commit.enemy.ShadowlordIdentity()

		return hasIdentity && identity == review.Shadowlord
	}

	return false
}

func matchingShadowlordAt(
	units *map_units.MapUnits,
	position references.Position,
	floor references.FloorNumber,
	lord references.Shadowlord,
) *map_units.NPCEnemy {
	if units == nil {
		return nil
	}
	for _, unit := range *units {
		enemy, ok := unit.(*map_units.NPCEnemy)
		if !ok || !enemy.IsVisible() || enemy.Pos() != position || enemy.Floor() != floor {
			continue
		}
		identity, hasIdentity := enemy.ShadowlordIdentity()
		if hasIdentity && identity == lord {
			return enemy
		}
	}

	return nil
}

func isShardQuestItem(item references.QuestItem) bool {
	return item >= references.ShardOfFalsehood && item <= references.ShardOfCowardice
}

func shadowlordDeadNPCIndex(lord references.Shadowlord) int {
	return int(lord) + 1
}
