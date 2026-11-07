package party_state

const maxKarma = 99

// Karma is the Avatar karma score, kept within 0..99 like the original byte.
type Karma struct {
	Value int
}

// AddDiff applies a signed karma change and clamps the result to 0..99.
func (k *Karma) AddDiff(nDiff int) {
	k.Value += nDiff
	if k.Value < 0 {
		k.Value = 0
	} else if k.Value > maxKarma {
		k.Value = maxKarma
	}
}
