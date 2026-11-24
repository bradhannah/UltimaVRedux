package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestProjectileColorMultipliersUseRequestedMagicMissileColor(t *testing.T) {
	tests := []struct {
		missile          references.MissileType
		red, green, blue float32
	}{
		{missile: references.MissileRed, red: 1, green: 0, blue: 0},
		{missile: references.MissileGreen, red: 0, green: 1, blue: 0},
		{missile: references.MissileBlue, red: 0, green: 0, blue: 1},
		{missile: references.MissileViolet, red: 1, green: 0, blue: 1},
	}

	for _, test := range tests {
		red, green, blue, alpha := projectileColorMultipliers(test.missile)
		assert.Equal(t, test.red, red)
		assert.Equal(t, test.green, green)
		assert.Equal(t, test.blue, blue)
		assert.Equal(t, float32(1), alpha)
	}
}

func TestPhysicalProjectileSourceGeometryShowsArrowAlongTravelDirection(t *testing.T) {
	right, ok := physicalProjectileSourceGeometry(references.MissileArrow, 4, 0)
	require.True(t, ok)
	require.Equal(t, 3, right.Count)
	assert.Less(t, right.Segments[0].FromX, right.Segments[0].ToX)

	left, ok := physicalProjectileSourceGeometry(references.MissileArrow, -4, 0)
	require.True(t, ok)
	require.Equal(t, 3, left.Count)
	assert.Greater(t, left.Segments[0].FromX, left.Segments[0].ToX)
}

func TestPhysicalProjectileSourceGeometryCoversInvalidSpriteProjectiles(t *testing.T) {
	for _, missile := range []references.MissileType{
		references.MissileArrow,
		references.MissileAxe,
		references.MissileCannonBall,
		references.MissileRock,
	} {
		geometry, ok := physicalProjectileSourceGeometry(missile, 1, 1)
		assert.True(t, ok, missile.GetStringName())
		assert.Positive(t, geometry.Count, missile.GetStringName())
	}

	_, ok := physicalProjectileSourceGeometry(references.MissileBlue, 1, 0)
	assert.False(t, ok)
	_, ok = physicalProjectileSourceGeometry(references.MissileArrow, 0, 0)
	assert.False(t, ok)
}

func TestPhysicalProjectileSourceGeometryUsesOriginalCannonBallSilhouette(t *testing.T) {
	geometry, ok := physicalProjectileSourceGeometry(references.MissileCannonBall, 1, 0)
	require.True(t, ok)
	require.Equal(t, 5, geometry.Count)
	assert.Less(t, geometry.Segments[0].ToX-geometry.Segments[0].FromX,
		geometry.Segments[2].ToX-geometry.Segments[2].FromX)
}

func TestProjectileTileCoordinatesUseShortestWrappedLargeMapPath(t *testing.T) {
	fromX, fromY, toX, toY := projectileTileCoordinates(
		references.Position{X: 255, Y: 100},
		references.Position{X: 0, Y: 100},
		references.Position{X: 255, Y: 100},
		true,
	)
	assert.Equal(t, float64(xCenter), fromX)
	assert.Equal(t, float64(yCenter), fromY)
	assert.Equal(t, float64(xCenter+1), toX)
	assert.Equal(t, float64(yCenter), toY)

	fromX, _, toX, _ = projectileTileCoordinates(
		references.Position{X: 0, Y: 100},
		references.Position{X: 255, Y: 100},
		references.Position{X: 255, Y: 100},
		true,
	)
	assert.Equal(t, float64(xCenter+1), fromX)
	assert.Equal(t, float64(xCenter), toX)

	_, fromY, _, toY = projectileTileCoordinates(
		references.Position{X: 100, Y: 255},
		references.Position{X: 100, Y: 0},
		references.Position{X: 100, Y: 255},
		true,
	)
	assert.Equal(t, float64(yCenter), fromY)
	assert.Equal(t, float64(yCenter+1), toY)
}

func TestCalculateProjectileDuration(t *testing.T) {
	tests := []struct {
		name     string
		fromX    float64
		fromY    float64
		toX      float64
		toY      float64
		expected uint64
		desc     string
	}{
		{
			name:  "Minimum_distance_returns_minimum_duration",
			fromX: 0, fromY: 0,
			toX: 10, toY: 0,
			expected: ProjectileMinDurationMs,
			desc:     "Very short distance (10px) clamps to 100ms minimum",
		},
		{
			name:  "One_tile_horizontal",
			fromX: 0, fromY: 0,
			toX: 16, toY: 0,
			expected: ProjectileMinDurationMs,
			desc:     "16 pixels / 0.325 = 49ms, clamped to 100ms",
		},
		{
			name:  "Three_tiles_horizontal",
			fromX: 0, fromY: 0,
			toX: 48, toY: 0,
			expected: 147, // 48 / 0.325 = 147.69
			desc:     "48 pixels = 147ms (above minimum)",
		},
		{
			name:  "Four_tiles_horizontal",
			fromX: 0, fromY: 0,
			toX: 64, toY: 0,
			expected: 196, // 64 / 0.325 = 196.92
			desc:     "64 pixels = 196ms",
		},
		{
			name:  "Diagonal_distance",
			fromX: 0, fromY: 0,
			toX: 48, toY: 48,
			expected: 208, // sqrt(48^2 + 48^2) = 67.88 / 0.325 = 208.86
			desc:     "Diagonal uses Euclidean distance",
		},
		{
			name:  "Long_distance",
			fromX: 0, fromY: 0,
			toX: 200, toY: 0,
			expected: 615, // 200 / 0.325 = 615.38
			desc:     "Long distance proportional time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateProjectileDuration(tt.fromX, tt.fromY, tt.toX, tt.toY, references.MissileBlue, references.CombatLocation)

			// Allow for floating point rounding (±1ms)
			if result < tt.expected-1 || result > tt.expected+1 {
				t.Errorf("CalculateProjectileDuration(%v, %v, %v, %v) = %d, want %d (%s)",
					tt.fromX, tt.fromY, tt.toX, tt.toY, result, tt.expected, tt.desc)
			}
		})
	}
}

func TestStartProjectileCalculatesDurationFromPixelCoordinates(t *testing.T) {
	gs, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: gs}
	from := references.Position{X: 5, Y: 5}
	to := references.Position{X: 8, Y: 5}

	duration := scene.startProjectile(from, to, references.MissileBlue)

	assert.Equal(t, uint64(320), duration, "town projectiles advance six pixels per 40ms")
}

func TestProjectileDurationUsesOriginalMissileAndMapPacing(t *testing.T) {
	for _, missile := range []references.MissileType{
		references.MissileArrow, references.MissileAxe, references.MissileRock, references.MissileCannonBall,
		references.MissileBlue, references.MissileRed, references.MissileGreen, references.MissileViolet,
	} {
		for _, location := range []references.Location{references.BritanniaUnderworld, references.CombatLocation, references.Britain} {
			want := uint64(196)
			if location == references.Britain {
				want = 426
			} else if missile == references.MissileRock {
				want = 320
			}
			assert.Equal(t, want, CalculateProjectileDuration(0, 0, 64, 0, missile, location), "%s/%s", missile.GetStringName(), location)
			assert.GreaterOrEqual(t, CalculateProjectileDuration(0, 0, 16, 0, missile, location), uint64(100))
		}
	}
}

func TestActiveProjectile_UpdatePosition(t *testing.T) {
	proj := &ActiveProjectile{
		fromTileX: 0,
		fromTileY: 0,
		toTileX:   10,
		toTileY:   10,
		active:    true,
	}

	tests := []struct {
		progress float64
		expectX  float64
		expectY  float64
	}{
		{0.0, 0, 0},
		{0.5, 5, 5},
		{1.0, 10, 10},
		{0.25, 2.5, 2.5},
		{0.75, 7.5, 7.5},
	}

	for _, tt := range tests {
		proj.UpdatePosition(tt.progress)

		if proj.currentTileX != tt.expectX || proj.currentTileY != tt.expectY {
			t.Errorf("UpdatePosition(%v) = (%v, %v), want (%v, %v)",
				tt.progress, proj.currentTileX, proj.currentTileY, tt.expectX, tt.expectY)
		}
	}
}

func TestActiveProjectile_UpdatePosition_Inactive(t *testing.T) {
	proj := &ActiveProjectile{
		fromTileX:    0,
		fromTileY:    0,
		toTileX:      10,
		toTileY:      10,
		currentTileX: 5,
		currentTileY: 5,
		active:       false, // Inactive
	}

	// Should not update when inactive
	proj.UpdatePosition(1.0)

	if proj.currentTileX != 5 || proj.currentTileY != 5 {
		t.Errorf("UpdatePosition on inactive projectile should not change position")
	}
}
