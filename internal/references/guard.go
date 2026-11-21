package references

// Guard Extortion System Constants
// Procedural tribute and Palace credentials; ref:CRIME-0001.

// Tribute per living party member; ref:CRIME-0001.
const (
	// TributePerLivingMember is the gold required per living party member
	// Formula: 10 gold × number of living party members
	TributePerLivingMember = 10
)

// Jail Location Constants
// Source: Original game data
const (
	// JailPositionX is the X coordinate of the jail in Yew.
	JailPositionX = 25

	// JailPositionY is the Y coordinate of the jail in Yew.
	JailPositionY = 4

	// JailFloorNumber is the floor level of the jail.
	JailFloorNumber = FloorNumber(0)

	// JailReleaseHour is the hour at which an ordinary sentence ends.
	JailReleaseHour = 8

	// JailSentenceStepMinutes is the ordinary sentence's time increment; ref:CRIME-0003.
	JailSentenceStepMinutes = 20
)

// Blackthorn Guard Password
// Palace credentials use a four-character password; ref:CRIME-0001.
//
//goland:noinspection GoUnusedConst
const (
	// BlackthornPassword is the correct password for Blackthorn's Castle guards
	// First 4 letters are checked, case-insensitive
	BlackthornPassword = "impe"
)
