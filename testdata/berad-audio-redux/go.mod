module github.com/bradhannah/UltimaVRedux/testdata/berad-audio-redux

go 1.27.0

require (
	github.com/bradhannah/UltimaVRedux v0.0.0
	github.com/bradhannah/berad-sdk/modules/berad-audio v0.0.0-20261019225412-564225ae5d47
	github.com/bradhannah/berad-sdk/tools/berad-audio v0.0.0-20261121170048-ebe61075b92e
)

// The gate supplies only the game substitution in a temporary fixture. Berad
// Audio always resolves the same published revision as the application.
