package text

// CreditParagraph is content only. Layout, scrolling and input belong to the
// reader. Empty paragraphs provide spacing; labels and values have distinct roles.
type CreditParagraph struct {
	Heading string
	Label   string
	Value   string
}

// OriginalAndReduxCredits preserves the names/role wording printed in the
// original STARTSC.16 center panel, plus the Book of Lore music acknowledgment.
// Evidence and contributor provenance: docs/gamefacts/STARTUP_MENU_AND_INTRODUCTIONS.md.
//
//nolint:exhaustruct_v5 // Content selects heading, prose, label/value, or blank spacing; other roles stay empty.
func OriginalAndReduxCredits() []CreditParagraph {
	return []CreditParagraph{
		{Heading: "Original Ultima V - Warriors of Destiny (1988)"},
		{Value: "Origin Systems"},
		{},
		{Label: "Produced and Designed by", Value: "Lord British"},
		{},
		{Label: "IBM Version by", Value: "John Fachini, Herman Miller, Ed Nelson, Cheryl Chen, Steve Meuse"},
		{},
		{Label: "Original Apple II Version by",
			Value: "Lord British, John Miles, Mark Hamner, Toshi Morita, Steve Meuse, Dallas Snell"},
		{},
		{Label: "Artwork by", Value: "Denis Loubet and Doug Wike"},
		{},
		{Label: "The Book of Lore acknowledges", Value: "Kenneth Arnold, for his masterful music."},
		{},
		{Heading: "Ultima V Redux"},
		{Label: "Project creator and developer", Value: "Brad Hannah"},
		{},
	}
}

// SpecialThanksCredits closes the continuous document on a readable resting row.
//
//nolint:exhaustruct_v5 // Declarative heading/prose paragraphs intentionally leave the other roles empty.
func SpecialThanksCredits() []CreditParagraph {
	return []CreditParagraph{
		{Heading: "Special Thanks"},
		{Value: "Origin Systems and Richard Garriott, for the world of Ultima."},
		{},
		{Value: "The Ultima community, for preserving and celebrating these adventures."},
		{},
		{Value: "To everyone who has kept Britannia alive - thank you."},
	}
}
