package minimap

import "github.com/bradhannah/UltimaVRedux/internal/references"

// GetStepProgress returns normalized progress for the current timed step.
func (e *ScriptExecutor) GetStepProgress() float64 {
	step := e.GetCurrentStep()
	if step == nil {
		return 1
	}
	duration := scriptStepDuration(step)
	if duration <= 0 {
		return 0
	}

	return min(float64(e.elapsedMs)/float64(duration), 1)
}

// GetCurrentStepIndex returns the index of the currently executing step
func (e *ScriptExecutor) GetCurrentStepIndex() int {
	return e.currentStepIndex
}

// GetActorPosition returns the current position of the actor with the given ID.
// Returns zero position if actor not found.
func (s *Scene) GetActorPosition(id ActorID) references.Position {
	actor := s.actors[id]
	if actor == nil {
		return references.Position{}
	}

	return actor.Position
}

// SetCameraCenter sets the camera center position
func (s *Scene) SetCameraCenter(pos references.Position) {
	s.cameraCenter = pos
}

// GetAllActors returns a slice of all actors in the scene
func (s *Scene) GetAllActors() []*Actor {
	actors := make([]*Actor, 0, len(s.actorOrder))
	for _, id := range s.actorOrder {
		if actor := s.actors[id]; actor != nil {
			actors = append(actors, actor)
		}
	}

	return actors
}

func scriptStepDuration(step *ScriptStep) int {
	switch step.Type {
	case StepWalk:
		return step.DelayMs
	case StepWait:
		return step.Duration
	case StepTileCommand:
		return tileCommandDuration(step.TileCommand)
	case StepFace, StepSetVisible, StepSetSprite, StepSetFrame, StepAwaitAdvance:
		return 0
	default:
		return 0
	}
}

// GetCurrentStep returns the active step for presentation rendering.
func (e *ScriptExecutor) GetCurrentStep() *ScriptStep {
	if e.IsComplete() {
		return nil
	}

	return &e.script.Steps[e.currentStepIndex]
}
