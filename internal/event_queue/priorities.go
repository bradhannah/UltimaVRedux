package event_queue

// Event Queue Priority Constants
//
// Priority determines the order in which event queues execute when multiple queues
// are registered. Lower numbers indicate higher priority.
//
// Priority Rules:
//   - Only ONE queue executes at a time (no concurrency)
//   - If new.priority < active.priority: Active queue YIELDS (interruption)
//   - If new.priority == active.priority: New queue QUEUES (FIFO tie-breaking)
//   - If new.priority > active.priority: New queue QUEUES (active continues)
//
// Usage:
//   Workflow("my_queue").
//       Priority(PriorityHigh).  // Use constant, not magic number
//       Sequential().
//       First(...).
//       Start()

const (
	// PriorityCritical - Highest priority, interrupts everything
	// Use for: Fatal errors, system dialogs, save corruption warnings
	// Examples: "Game must exit", "Cannot continue", critical system messages
	PriorityCritical = 1

	// PriorityHigh - High priority gameplay events
	// Use for: Moongate travel, ship collisions, environmental hazards, combat encounters
	// Examples: Moongate teleportation, lava damage, earthquake, ship collision
	// Interrupts: Normal and Low priority queues
	PriorityHigh = 2

	// PriorityMedium - User-initiated interactions
	// Use for: Player commands (Open, Talk, Search), dialog interactions, NPC conversations
	// Examples: Dialog workflows, command sequences, conversation trees
	// Interrupts: Normal and Low priority queues
	PriorityMedium = 3

	// PriorityNormal - Standard gameplay operations
	// Use for: Sailing, camping, background state updates, routine gameplay
	// Examples: Automatic sailing, camping sequences, time progression
	// Interrupted by: Critical, High, Medium priorities
	PriorityNormal = 5

	// PriorityLow - Background effects and animations
	// Use for: Visual effects, ambient animations, non-critical sound triggers
	// Examples: Kapow effects, missile animations, background flourishes
	// Interrupted by: All higher priorities
	PriorityLow = 10
)

// Priority Guidelines:
//
// Critical (1):
//   ✅ Use for system-level issues that must be seen immediately
//   ✅ Fatal errors, data corruption, must-acknowledge dialogs
//   ❌ Don't use for normal gameplay events
//
// High (2):
//   ✅ Use for immediate gameplay events that affect player state
//   ✅ Moongate teleportation, hazard damage, collisions
//   ❌ Don't use for user-initiated commands (use Medium)
//
// Medium (3):
//   ✅ Use for player-initiated interactions
//   ✅ Commands with direction input, dialog interactions
//   ❌ Don't use for automatic background processes
//
// Normal (5):
//   ✅ Use for standard gameplay loops and sequences
//   ✅ Sailing, camping, routine state changes
//   ❌ Don't use for user commands or critical events
//
// Low (10):
//   ✅ Use for visual/audio effects that don't block gameplay
//   ✅ Animations, ambient effects, optional visuals
//   ❌ Don't use for anything that affects game state
