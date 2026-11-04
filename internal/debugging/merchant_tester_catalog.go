package debugging

const (
	// CommandMerchantTesterCatalog reports canonical merchants and current launch availability.
	CommandMerchantTesterCatalog = "merchant.tester.catalog"
	// CommandMerchantTesterPreviewOpen opens a read-only production merchant presentation.
	CommandMerchantTesterPreviewOpen = "merchant.tester.preview.open"
	// CommandMerchantTesterLiveOpen opens a production merchant session, transitioning towns when required.
	CommandMerchantTesterLiveOpen = "merchant.tester.live.open"

	// OptionProviderMerchantTesterMerchants resolves canonical merchant identities from loaded references.
	OptionProviderMerchantTesterMerchants = "merchant.tester.merchants"

	categoryMerchantTester = "Merchants"
	reviewedLocationName   = "Reviewed Location"
	reviewedFloorName      = "Reviewed Floor"
	reviewedXName          = "Reviewed X"
	reviewedYName          = "Reviewed Y"
	parameterExpectedX     = "expectedX"
	parameterExpectedY     = "expectedY"
)

func merchantTesterCommands() []CommandDefinition {
	minimumLocation, maximumLocation := 0, 40
	minimumFloor, maximumFloor := -1, 7
	minimumCoordinate, maximumCoordinate := 0, 255
	merchantParameter := ParameterDefinition{
		ID: "merchantId", Name: "Merchant", Description: "Choose one canonical active merchant identity.",
		Kind: ParameterString, Required: true, OptionsProvider: OptionProviderMerchantTesterMerchants,
		ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat},
	}

	return []CommandDefinition{
		{
			ID: CommandMerchantTesterCatalog, Category: categoryMerchantTester, Name: "Merchant Tester",
			Description: "Inspect all canonical merchants and current Preview and Live launch availability.",
			Aliases:     []string{"merchant catalog", "shop tester"}, ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandMerchantTesterPreviewOpen, Category: categoryMerchantTester, Name: "Open Merchant Preview",
			Description: "Open the production merchant presentation and stop before transaction authority.",
			Aliases:     []string{"merchant preview", "shop preview"}, Parameters: []ParameterDefinition{merchantParameter},
			ExecutionModes: []ExecutionMode{ExecutionWorkflow},
		},
		{
			ID: CommandMerchantTesterLiveOpen, Category: categoryMerchantTester, Name: "Open Live Merchant Session",
			Description: "Open a reviewed production merchant session, loading the canonical town when required.",
			Aliases:     []string{"merchant live", "shop live"},
			Parameters: []ParameterDefinition{
				merchantParameter,
				{ID: parameterExpectedLocationID, Name: reviewedLocationName, Kind: ParameterInteger, Required: true, Minimum: &minimumLocation, Maximum: &maximumLocation, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: parameterExpectedFloor, Name: reviewedFloorName, Kind: ParameterInteger, Required: true, Minimum: &minimumFloor, Maximum: &maximumFloor, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: parameterExpectedX, Name: reviewedXName, Kind: ParameterInteger, Required: true, Minimum: &minimumCoordinate, Maximum: &maximumCoordinate, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: parameterExpectedY, Name: reviewedYName, Kind: ParameterInteger, Required: true, Minimum: &minimumCoordinate, Maximum: &maximumCoordinate, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: "expectedSourceFingerprint", Name: "Reviewed Source Fingerprint", Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}},
			},
			ExecutionModes: []ExecutionMode{ExecutionWorkflow, ExecutionAlteration}, Destructive: true,
		},
	}
}
