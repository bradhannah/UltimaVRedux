import type { WorldPositionDungeonRoomData } from "./protocol"

export interface LoggingStatusData {
  enabled: boolean
  stdoutEnabled: boolean
  minimumLevel: string
  categoryStates: Record<string, boolean>
}

export interface CalendarStatusData {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  turn: number
  date: string
  time: string
  moonPhase: string
  moonPhaseIndex: number
}

export interface ObservationResultData {
  workflow: string
  queued: boolean
  expectedBranch: "sun_damage" | "no_stars" | "night_sky" | "character_selection"
  context: {
    date: string
    time: string
    hour: number
    minute: number
    isNight: boolean
    mapType: string
    locationId: number
    locationName: string
    floor: number
  }
}

export interface PartyRosterCharacterData {
	rosterIndex: number
	name: string
	gender: number
	class: string
	status: "good" | "poisoned" | "sleep" | "charmed" | "dead" | "unknown"
	active: boolean
  partyMap: number
	strength: number
	dexterity: number
	intelligence: number
	currentMp: number
	currentHp: number
	maximumHp: number
	experience: number
	level: number
	monthsAtInn: number
  armorClass: number
	helmet: number
	armor: number
	weapon: number
	shield: number
	ring: number
	amulet: number
}

export interface PartyCharacterInspectData {
	fingerprint: string
	karma: number
	characters: PartyCharacterMetadataData[]
	roster: PartyRosterCharacterData[]
}

export interface PartyCharacterMetadataData {
	partyPosition: number
	name: string
	class: string
	status: string
	strength: number
	dexterity: number
	intelligence: number
	currentMp: number
	currentHp: number
	maximumHp: number
	experience: number
	level: number
}

export interface SearchLocationItem {
  index: number
  found: boolean
  repeatable: boolean
	rawObjectType: number
	rawPayload: number
	classification: string
	category: string
  itemType: number
	itemId: number
  itemName: string
  quantity: number
	discoveryText: string
}

export interface SearchItemStack {
  itemType: number
  itemId: number
  itemName: string
  quantity: number
}

export interface SearchCellData {
  direction: string
  x: number
  y: number
  locationId: number
  locationName: string
  mapType: string
  floor: number
  inBounds: boolean
  tileIndex?: number
  tileName?: string
  authenticLocations: SearchLocationItem[]
  itemStack: SearchItemStack[]
}

export interface SearchStatusData {
  locationId: number
  locationName: string
  mapType: string
  floor: number
  cells: SearchCellData[]
  progress: {
    totalLocations: number
    foundLocations: number
    oneTimeLocations: number
    foundOneTimeLocations: number
  }
  overlayEnabled: boolean
}

export interface QuestItemData {
  key: string
  name: string
  category: string
  quantity: number
  canonicalPlacement?: {
    locationId: number
    locationName: string
    x: number
    y: number
    floor: number
  }
	shardUseReview?: {
		shard: string
		quantity: number
		locationId: number
		locationName: string
		x: number
		y: number
		floor: number
		flame: "none" | "truth" | "love" | "courage"
		flameMatches: boolean
		shadowlordSummoned: boolean
		shadowlord: "none" | "faulinei" | "astaroth" | "nosfentor"
		shadowlordAlive: boolean
		expectedOutcome: "not_attempted" | "no_effect" | "success"
	}
}

export interface QuestItemStatusData {
  items: QuestItemData[]
}

export interface WorldPositionBoundsData {
  minimumX: number
  maximumX: number
  minimumY: number
  maximumY: number
}

export interface WorldPositionSetupData {
  floor: number
  x: number
  y: number
}

export interface WorldPositionPlacementTargetData {
  placement: string
  placementLabel: string
  route: string
  routeLabel: string
  available: boolean
  unavailableReason?: string
  locationId: number
  setup?: WorldPositionSetupData
  facingLabel?: string
  lastDirectionLabel?: string
}

export interface WorldPositionDestinationData {
  locationId: number
  locationName: string
  mapType: string
  floors: number[]
  bounds: WorldPositionBoundsData
  defaultSetup: WorldPositionSetupData
  worldEntrance?: WorldPositionSetupData
  placementTargets?: WorldPositionPlacementTargetData[]
  /** Dungeons only: every fixed-monster room, ordered by floor then room number. */
  rooms?: WorldPositionDungeonRoomData[]
}

export interface WorldPositionStatusData {
  sourceFingerprint: string
  locationId: number
  locationName: string
  mapType: string
  floor: number
  x: number
  y: number
  bounds: WorldPositionBoundsData
  destinations: WorldPositionDestinationData[]
}

export interface MerchantTesterContextData {
  locationId: number
  locationName: string
  floor: number
  x: number
  y: number
}

export interface MerchantTesterMerchantData {
  merchantId: string
  townId: number
  townName: string
  shopName: string
  ownerName: string
  familyId: string
  familyLabel: string
  dialogCode: number
  implemented: boolean
  unavailableReason?: string
  previewAvailable: boolean
  previewUnavailableReason?: string
  liveAvailable: boolean
  liveUnavailableReason?: string
  liveExecutionMode: string
}

export interface MerchantTesterCatalogData {
  sourceFingerprint: string
  currentContext: MerchantTesterContextData
  merchants: MerchantTesterMerchantData[]
}

export interface MerchantTesterOpenData {
  merchantId: string
  townId: number
  townName: string
  preview: boolean
  transitioned: boolean
  workflowQueued: boolean
  locationId: number
  floor: number
  x: number
  y: number
}

export interface TestSetupStatusData {
  partyFingerprint: string
  wipeReview: ActivePartyWipeReviewData
  locationId: number
  locationName: string
  mapType: string
  floor: number
  date: string
  time: string
  food: number
  combatActive: boolean
  dialogActive: boolean
  gameplayWorkflowActive: boolean
  scriptedSceneActive: boolean
  gameplayTerminal: boolean
  finaleActive: boolean
  allPartyDead: boolean
  party: Array<{
    rosterIndex: number
    partyPosition: number
    name: string
    class: string
    status: string
    currentHp: number
    maximumHp: number
    currentMp: number
    experience: number
    level: number
  }>
}

export interface ActivePartyWipeReviewData {
  fingerprint: string
  members: Array<{
    slot: number
    name: string
    hp: number
    status: string
  }>
}

export interface ActivePartyWipeChangeData {
  before: ActivePartyWipeReviewData["members"][number]
  after: ActivePartyWipeReviewData["members"][number]
}

export interface ActivePartyWipeResultData {
  beforeFingerprint: string
  afterFingerprint: string
  changes: ActivePartyWipeChangeData[]
  workflow?: string
  queued: boolean
}

export interface MoongateMoonstoneData {
  index: number
  phase: string
  x: number
  y: number
  map: number
  level: number
  state: string
  found: boolean
}

export interface MoongateStatusData {
  sourceFingerprint: string
  phase: string
  phaseIndex: number
  visible: boolean
  playerOnActiveGate: boolean
  teleportationActive: boolean
  moonstones: MoongateMoonstoneData[]
  route?: {
    fingerprint: string
    kind: "moonstone" | "spirituality"
    destination?: {
      x: number
      y: number
      map: number
      level: number
      locationId: number
      locationName: string
    }
    shrineIndex?: number
    shrineName?: string
  }
  unavailableReason?: string
  overlayEnabled: boolean
}

export interface DebugPositionData {
  x: number
  y: number
}

export interface DebugOptionData {
  id: number
  name: string
}

export interface CombatMapSummaryData {
  index: number
  name: string
}

export type CombatEncounterEntityKind = "monster" | "random" | "object" | "loot" | "field"

export interface CombatEncounterEntityData {
  kind: CombatEncounterEntityKind
  name: string
  sprite: number
  x: number
  y: number
  concealed: boolean
}

export interface CombatEncounterPreviewData {
  tiles: number[]
  party: Record<string, DebugPositionData[]>
  monsterSlots: DebugPositionData[]
  entities: CombatEncounterEntityData[]
  triggers: DebugPositionData[]
}

export interface CombatEncounterMapData {
  index: number
  name: string
  preview: CombatEncounterPreviewData
}

export interface CombatEncounterRoomData {
  room: number
  floor?: number
  cell?: DebugPositionData
  preview: CombatEncounterPreviewData
}

export interface CombatEncounterDungeonData {
  id: number
  name: string
  rooms: CombatEncounterRoomData[]
}

export interface CombatEncounterFriendData {
  id: number
  name: string
}

export interface CombatEncounterMonsterData {
  id: number
  name: string
  strength: number
  dexterity: number
  intelligence: number
  armour: number
  damage: number
  hitPoints: number
  maxGroup: number
  friend?: CombatEncounterFriendData
}

export interface CombatEncounterOptionsData {
  britanniaMaps: CombatEncounterMapData[]
  dungeons: CombatEncounterDungeonData[]
  entryDirections: DebugOptionData[]
  countModes: DebugOptionData[]
  monsters: CombatEncounterMonsterData[]
}

export interface CombatEncounterStartData {
  territory: string
  contents: "custom" | "authored"
  mapIndex: number
  dungeon: number
  dungeonRoom: number
  monsterType?: number
  monsterName?: string
  seed?: number
  primaryCount: number
  friendCount: number
  totalCount: number
  positions: DebugPositionData[]
}

export interface MonsterEnemyData {
  runtimeToken: number
  monsterType: number
  name: string
  position: DebugPositionData
  floor: number
  visible: boolean
  activelyAttacks: boolean
  canFlyOverWater: boolean
  canPassThroughWalls: boolean
  doNotMove: boolean
  hasWhirlpoolMovement: boolean
  skipNextTurn: boolean
  isSandEnemy: boolean
  isWaterEnemy: boolean
}

export interface MonsterEcosystemData {
  locationId: number
  locationName: string
  mapType: string
  floor: number
  generationEnabled: boolean
  generationBaseOdds: number
  capacity: number
  currentUnitCount: number
  enemyCount: number
  friendlyCount: number
  vehicleCount: number
  enemies: MonsterEnemyData[]
}

export interface MonsterClearData {
  removed: number
  supported: boolean
  unsupportedReason?: string
}

export interface MonsterSpawnData {
  spawned: boolean
  position?: DebugPositionData
  rejectionReason?: string
}

export interface VehicleStatusData {
  vehicleType: number
  vehicleName: string
  currentDirection: string
  previousDirection: string
  ownedByNpc: boolean
  hullHp: number
  maximumHullHp: number
  sailState: string
  skiffQuantity: number
  position: DebugPositionData
  floor: number
  hmsCapeActive: boolean
  rawHullDamageAvailable: boolean
  fullHullRepairAvailable: boolean
}

export interface DebugOptionsStatusData {
  freeMove: boolean
  textPlaceholdersEnabled: boolean
  dialogBoundsEnabled: boolean
  unlimitedCastingResources: boolean
  bypassSpellLevelRestrictions: boolean
  lightingOverlayEnabled: boolean
}

export interface GuardStatusData {
  supported: boolean
  unavailableReason?: string
  locationId: number
  locationName: string
  floor: number
  criminal: boolean
  friendlyNpcCount: number
  potentialGuardCount: number
  hostilePotentialGuardCount: number
  otherHostileNpcCount: number
}

export interface ShadowlordStatusEntryData {
  id: number
  name: string
  antiVirtue: string
  alive: boolean
}

export interface ShadowlordStatusData {
  day: number
  aliveCount: number
  allDefeated: boolean
  summoned: boolean
  summonedShadowlord?: ShadowlordStatusEntryData
  automaticPresence?: ShadowlordStatusEntryData
  shadowlords: ShadowlordStatusEntryData[]
  townAssignments: Array<{
    locationId: number
    locationName: string
    shadowlordId: number
    shadowlordName: string
  }>
}

export interface DeathStatusData {
  wipeReview: ActivePartyWipeReviewData
  allPartyDead: boolean
  activePartyCount: number
  deadPartyCount: number
  karma: number
  experienceRetainedPercent: number
  experienceLostPercent: number
  minimumPostDeathKarma: number
  minimumPostDeathFood: number
  resurrection: {
    locationId: number
    locationName: string
    x: number
    y: number
    floor: number
  }
}

export interface InventoryProvisionData {
  name: string
  quantity: number
  maximum: number
}

export interface InventoryNamedQuantityData {
  id: number
  name: string
  quantity: number
  maximum: number
}

export interface InventoryEquipmentData {
  id: number
  name: string
  category: string
  slot: string
  ammo: boolean
  quantity: number
  maximum: number
}

export interface InventoryEquippedData {
  rosterIndex: number
  name: string
  slots: {
    slot: string
    equipmentId: number
    equipmentName: string
  }[]
}

export interface InventoryLockerStatusData {
  snapshotFingerprint: string
  provisions: InventoryProvisionData[]
  reagents: InventoryNamedQuantityData[]
  potions: InventoryNamedQuantityData[]
  scrolls: InventoryNamedQuantityData[]
  equipment: InventoryEquipmentData[]
  equipped: InventoryEquippedData[]
	usablePresetReview: {
		reviewFingerprint: string
		changes: {
			category: "quest_items" | "moonstones" | "potions" | "scrolls"
			path: string
			before: number
			after: number
		}[]
	}
}

export interface SpellInventoryStatusData {
  snapshotFingerprint: string
  spells: {
    id: number
    name: string
    description: string
    circle: number
    targeting: string
    context: string
    quantity: number
    maximum: number
  }[]
  magicLight: {
    active: boolean
    minutesRemaining: number
  }
  activeSpell: {
    active: boolean
    effectType: string
    friendlyName: string
    incantations: string[]
    sourceAmbiguous: boolean
    description: string
    turnsRemaining: number
  }
}

export interface ActiveCombatStatusData {
  active: boolean
  unavailableReason?: string
  snapshotFingerprint: string
  monsterTypes: { id: number; name: string }[]
  encounterId?: string
  map?: {
    name: string
    index: number
    territory: string
    entryDirection: string
    victory: boolean
    turnStarted: boolean
    pendingAdvanceTurn: boolean
    soloMoveMode: boolean
    currentPartyIndex: number
    activeMonsterIndex: number
    targetPartyIndex: number
    objectCount: number
    cells: {
      x: number
      y: number
      tileId: number
      occupantKind?: string
      occupantId?: string
      occupantName?: string
    }[]
  }
  party: {
    index: number
    name: string
    x: number
    y: number
    active: boolean
    currentActor: boolean
    currentTarget: boolean
    currentHp: number
    maximumHp: number
    currentMp: number
    maximumMp: number
    dexterity: number
    status: string
    queueCountdown: number
    queueStatus: string
    canAct: boolean
  }[]
  monsters: {
    id: string
    managerIndex: number
    typeId: number
    name: string
    x: number
    y: number
    currentHp: number
    maximumHp: number
    currentDexterity: number
    baseDexterity: number
    active: boolean
    dead: boolean
    invisible: boolean
    asleep: boolean
    poisoned: boolean
    currentActor: boolean
    queueCountdown: number
    queueStatus: string
    canAct: boolean
    abilities: string[]
    hostile: boolean
  }[]
  forecast: {
    order: number
    kind: string
    id: string
    name: string
    dexterity: number
    resetCountdown: number
    canAct: boolean
    status: string
  }[]
  activeMonsterCount: number
  activeHostileMonsterCount: number
  totalMonsterCount: number
  fixedDungeonRoom: boolean
  dungeonIndex?: number
  dungeonRoom?: number
  defeatedExperience: number
	  rngSeed: number
  workflowActive: boolean
  presentationSpeedPercent: number
	configurationEditing: boolean
}
