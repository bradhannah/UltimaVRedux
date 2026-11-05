<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue"
import { provideInstrumentSession } from "./instrument_session"

import {
  envelope,
  type CatalogSnapshot,
  type CommandDefinition,
  type CommandResult,
  type Envelope,
  type SessionReady,
} from "./protocol"
import {
  choicesForParameter,
  hasUnsupportedParameters,
  reconcileDraft,
  resetDraft,
  serializeDraft,
  validateDraft,
  type CommandDraft,
  type FormErrors,
  type OptionSets,
} from "./command_forms"
import GoToPage from "./GoToPage.vue"
import PartyPage from "./PartyPage.vue"
import ItemsPage from "./ItemsPage.vue"
import TimePage from "./TimePage.vue"
import FightPage from "./FightPage.vue"
import LoggingConsole from "./LoggingConsole.vue"
import MerchantTester from "./MerchantTester.vue"
import MonsterEcosystemConsole from "./MonsterEcosystemConsole.vue"
import QuestInventoryInspector from "./QuestInventoryInspector.vue"
import SearchItemStacksInspector from "./SearchItemStacksInspector.vue"
import TestSetups from "./TestSetups.vue"
import SaveLibrary from "./SaveLibrary.vue"
import ShrineProgressInspector from "./ShrineProgressInspector.vue"
import ShipsConsole from "./ShipsConsole.vue"
import DebugOptionsConsole from "./DebugOptionsConsole.vue"
import WorldDiagnostics from "./WorldDiagnostics.vue"
import ObservationPane, { type ObservationCommandID } from "./ObservationPane.vue"
import WorkbenchTopBar, { type TopBarClock, type TopBarParty } from "./WorkbenchTopBar.vue"
import CommandPalette from "./CommandPalette.vue"
import WorkbenchHome from "./WorkbenchHome.vue"
import { recentNavigationSurfaces, updateNavigationRecents, type NavigationSurface } from "./command_navigation"
import {
  compareToolOrder,
  homeTaskIDs,
  pageDefinitions,
  partyHealthSummary,
  placeLabel,
  preferredLandmarks,
  taskDefinition,
  taskDefinitions,
  timePresets,
  toolDefinitions,
  toolGroupOrder,
  type HomeCard,
  type HomeQuickAction,
  type TaskID,
  type ToolGroup,
} from "./workbench_tasks"
import type { PageNotice, PageStep, WorldPositionApply } from "./page_steps"
import type {
  CalendarStatusData,
  CombatEncounterOptionsData,
  MerchantTesterCatalogData,
  MonsterEcosystemData,
  MoongateStatusData,
  QuestItemStatusData,
  SearchStatusData,
  VehicleStatusData,
  WorldPositionStatusData,
  InventoryLockerStatusData,
  SpellInventoryStatusData,
  ActiveCombatStatusData,
  LoggingStatusData,
	PartyCharacterInspectData,
	DebugOptionsStatusData,
	DeathStatusData,
	GuardStatusData,
  ShadowlordStatusData,
  TestSetupStatusData,
  ObservationResultData,
} from "./approved_tool_views"
import type { ShrineStatusData } from "./shrine_views"
import type { SaveSnapshotGetData, SaveSnapshotListData, SaveSnapshotSummary } from "./saves"
import {
  hydratePartyCharacterDraft,
  partyAlterationPreview,
  partyResourceErrors,
  partyCharacterCommandID,
  partyCharacterFieldIDs,
  partyFieldIsDirty,
  reconcilePartyCharacterDraft,
  serializePartyCharacterDraft,
} from "./party_character_editor"
import { partyEquipmentLoadoutCommandID } from "./party_equipment_loadout"

type ConnectionState = "connecting" | "handshaking" | "ready" | "reconnecting" | "disconnected"
type WorkspaceID = "home" | TaskID
type HistoryDock = "bottom" | "right"
interface ResultEntry extends CommandResult {
  entryID: string
  receivedAt: Date
  ownerCommandID?: string
}

interface PendingRequest {
  commandID: string
  ownerCommandID?: string
  phase: "sending" | "accepted"
  startedAt: number
  lastConfirmedAt: number
  statusRequested: boolean
}

interface ToolGroupView {
  name?: ToolGroup
  commands: CommandDefinition[]
}

const resultLimit = 100
const windStatusCommandID = "environment.wind.status"
const loggingStatusCommandID = "logging.status"
const loggingMutationCommandIDs = ["logging.enabled.set", "logging.stdout.set", "logging.level.set", "logging.category.set", "logging.categories.set-all"]
const windSetCommandID = "environment.wind.set"
const shrineStatusCommandID = "shrine.status"
const shrineProgressCommandID = "shrine.progress.update"
const codexQuestStateSetAllCommandID = "codex.quest-state.set-all"
const calendarStatusCommandID = "calendar.status"
const calendarSetTimeCommandID = "calendar.set-time"
const calendarSetDayCommandID = "calendar.set-day"
const searchStatusCommandID = "search.status"
const searchItemStackAddCommandID = "search.item-stack.add"
const searchProgressResetCommandID = "search.progress.reset"
const searchOverlaySetCommandID = "overlay.search.set"
const commandTestSearchCommandID = "command-test.search"
const questItemStatusCommandID = "quest.item.status"
const questItemEnsureCommandID = "quest.item.ensure"
const questItemRemoveCommandID = "quest.item.remove"
const questShardUseCommandID = "quest.shard.use"
const usableInventoryPresetCommandID = "quest.inventory.preset-usable"
const worldPositionStatusCommandID = "world.position.status"
const worldPositionSetCommandID = "world.position.set"
const movementLandmarkTravelCommandID = "movement.landmark.travel"
const partyKarmaSetCommandID = "party.karma.set"
const partyCharacterInspectCommandID = "party.character.inspect"
const partyRestoreCommandID = "party.restore"
const partyRosterMutationCommandIDs = ["party.maximize-levels", "party.status.set", "party.fill", "party.member.add", partyRestoreCommandID]
const testSetupStatusCommandID = "test-setup.status"
const saveSnapshotListCommandID = "save.snapshot.list"
const saveSnapshotGetCommandID = "save.snapshot.get"
const saveSnapshotCaptureCommandID = "save.snapshot.capture"
const saveSnapshotLoadCommandID = "save.snapshot.load"
const saveSnapshotDeleteCommandID = "save.snapshot.delete"
const saveSnapshotCommandIDs = [saveSnapshotListCommandID, saveSnapshotGetCommandID, saveSnapshotCaptureCommandID, saveSnapshotLoadCommandID, saveSnapshotDeleteCommandID]
const commandTestHoleUpCommandID = "command-test.hole-up"
const deathKillPartyCommandID = "death.kill-party"
const deathTestSequenceCommandID = "death.test-sequence"
const observationCommandID = "command-test.telescope"
const observationCommandIDs: ObservationCommandID[] = [observationCommandID, "command-test.spyglass", "command-test.crystal-ball"]
const testSetupMutationCommandIDs = [commandTestHoleUpCommandID, deathKillPartyCommandID, deathTestSequenceCommandID]
const moongateStatusCommandID = "moongate.status"
const moongateOverlaySetCommandID = "overlay.moongate.set"
const moongateTravelCommandID = "moongate.travel"
const combatEncounterOptionsCommandID = "combat.encounter.options"
const combatEncounterStartCommandID = "combat.encounter.start"
const merchantTesterCatalogCommandID = "merchant.tester.catalog"
const merchantTesterPreviewCommandID = "merchant.tester.preview.open"
const merchantTesterLiveCommandID = "merchant.tester.live.open"
const merchantTesterOperationCommandIDs = [merchantTesterPreviewCommandID, merchantTesterLiveCommandID]
const monsterEcosystemStatusCommandID = "monster.ecosystem.status"
const monsterGenerationSetCommandID = "monster.generation.set"
const monsterClearMapEnemiesCommandID = "monster.clear-map-enemies"
const monsterSpawnWhirlpoolCommandID = "monster.spawn-whirlpool"
const monsterSpawnPirateCommandID = "monster.spawn-pirate"
const vehicleStatusCommandID = "vehicle.status"
const vehicleSpawnCommandID = "vehicle.spawn"
const vehicleForceBoardCommandID = "vehicle.force-board"
const shipDamageRawCommandID = "ship.damage-raw"
const shipRepairFullCommandID = "ship.repair-full"
const shipSkiffSetCommandID = "ship.skiff.set"
const shipMutationCommandIDs = [vehicleSpawnCommandID, vehicleForceBoardCommandID, shipDamageRawCommandID, shipRepairFullCommandID, shipSkiffSetCommandID]
const debugOptionsStatusCommandID = "debug.options.status"
const lightingOverlaySetCommandID = "overlay.lighting.set"
const movementFreeMoveSetCommandID = "movement.free-move.set"
const textPlaceholdersSetCommandID = "overlay.text-placeholders.set"
const dialogBoundsSetCommandID = "overlay.dialog-bounds.set"
const unlimitedCastingResourcesSetCommandID = "magic.unlimited-casting-resources.set"
const spellLevelRestrictionsBypassSetCommandID = "magic.spell-level-restrictions-bypass.set"
const debugOptionsMutationCommandIDs = [movementFreeMoveSetCommandID, textPlaceholdersSetCommandID, dialogBoundsSetCommandID, lightingOverlaySetCommandID, unlimitedCastingResourcesSetCommandID, spellLevelRestrictionsBypassSetCommandID]
const combatPresentationSpeedSetCommandID = "combat.presentation.speed.set"
const guardStatusCommandID = "guards.status"
const shadowlordStatusCommandID = "shadowlord.status"
const deathStatusCommandID = "death.status"
const worldDiagnosticCommandIDs = [guardStatusCommandID, shadowlordStatusCommandID, deathStatusCommandID]
const inventoryLockerStatusCommandID = "inventory.locker.status"
const activeCombatStatusCommandID = "combat.status"
// Queried on every poll so the top bar can show When, Where, Party and the combat mode on every view.
const globalStatusCommandIDs = [calendarStatusCommandID, activeCombatStatusCommandID, testSetupStatusCommandID]
const activeCombatSleepCommandID = "combat.monster.sleep.set"
const activeCombatExitCommandID = "combat.exit-debug"
const activeCombatSpawnCommandID = "combat.monster.spawn"
const activeCombatRNGSeedSetCommandID = "rng.seed.set"
const activeCombatDamageCommandID = "combat.monster.damage-raw"
const activeCombatKillCommandID = "combat.monster.kill"
const activeCombatKillAllCommandID = "combat.monster.kill-all"
const activeCombatDungeonRoomClearCommandID = "combat.dungeon-room.clear"
const activeCombatClearCommandID = "combat.monster.clear"
const activeCombatSleepAllCommandID = "combat.monster.sleep-all"
const activeCombatMutationCommandIDs = [
	activeCombatSleepCommandID, activeCombatExitCommandID, activeCombatSpawnCommandID, activeCombatRNGSeedSetCommandID,
	activeCombatDamageCommandID, activeCombatKillCommandID, activeCombatKillAllCommandID, activeCombatSleepAllCommandID,
	activeCombatDungeonRoomClearCommandID, activeCombatClearCommandID,
	combatPresentationSpeedSetCommandID,
]
const spellInventoryStatusCommandID = "inventory.spell.status"
const spellInventorySetCommandID = "inventory.spell.set"
const spellInventoryPresetMaxCommandID = "inventory.spell.preset-max"
const spellInventoryMutationCommandIDs = [spellInventorySetCommandID, spellInventoryPresetMaxCommandID, "spell.effect.set-remaining", "spell.effect.activate"]
const inventoryLockerMutationCommandIDs = [
	"inventory.provision.set", "inventory.provision.preset", "inventory.reagent.set", "inventory.reagent.preset", "inventory.potion.add",
	"inventory.potion.preset", "inventory.scroll.add", "inventory.scroll.preset", "inventory.equipment.preset",
	"inventory.equipment.grant-preset", "inventory.equipment.set", "inventory.equipment.clear", usableInventoryPresetCommandID,
]
const integratedCommandIDs = new Set([
  loggingStatusCommandID,
  ...loggingMutationCommandIDs,
  windStatusCommandID,
  windSetCommandID,
  shrineStatusCommandID,
  shrineProgressCommandID,
  codexQuestStateSetAllCommandID,
	partyCharacterInspectCommandID,
  partyCharacterCommandID,
	...partyRosterMutationCommandIDs,
  partyEquipmentLoadoutCommandID,
  calendarStatusCommandID,
  calendarSetTimeCommandID,
  calendarSetDayCommandID,
  searchStatusCommandID,
  searchItemStackAddCommandID,
  searchProgressResetCommandID,
  searchOverlaySetCommandID,
  commandTestSearchCommandID,
  questItemStatusCommandID,
  questItemEnsureCommandID,
  questItemRemoveCommandID,
	questShardUseCommandID,
  worldPositionStatusCommandID,
  worldPositionSetCommandID,
  movementLandmarkTravelCommandID,
  partyKarmaSetCommandID,
  testSetupStatusCommandID,
  ...saveSnapshotCommandIDs,
  commandTestHoleUpCommandID,
  deathKillPartyCommandID,
  deathTestSequenceCommandID,
  moongateStatusCommandID,
  moongateOverlaySetCommandID,
  moongateTravelCommandID,
  combatEncounterOptionsCommandID,
  combatEncounterStartCommandID,
  merchantTesterCatalogCommandID,
  ...merchantTesterOperationCommandIDs,
  monsterEcosystemStatusCommandID,
  monsterGenerationSetCommandID,
  monsterClearMapEnemiesCommandID,
  monsterSpawnWhirlpoolCommandID,
  monsterSpawnPirateCommandID,
  vehicleStatusCommandID,
  vehicleSpawnCommandID,
  vehicleForceBoardCommandID,
  shipDamageRawCommandID,
  shipRepairFullCommandID,
	shipSkiffSetCommandID,
	debugOptionsStatusCommandID,
	...debugOptionsMutationCommandIDs,
	...worldDiagnosticCommandIDs,
  inventoryLockerStatusCommandID,
  ...inventoryLockerMutationCommandIDs,
  spellInventoryStatusCommandID,
  ...spellInventoryMutationCommandIDs,
  activeCombatStatusCommandID,
  ...activeCombatMutationCommandIDs,
  ...observationCommandIDs,
  ...Object.values(pageDefinitions).flatMap((page) => [page.anchor, ...page.queries, ...(page.loadOnce ?? []), ...page.commands, ...(page.uses ?? [])]),
])
const connectionTimeoutMs = 5000
const operationStallMs = 10000
const operationReconnectMs = 30000
const sessionStorageKey = "u5r-debug-workbench-session"
const instrumentStorageKey = "u5r-debug-workbench-instrument"
const historyCollapsedStorageKey = "u5r-debug-workbench-history-collapsed"
const historyDockStorageKey = "u5r-debug-workbench-history-dock"
const navigationRecentsStorageKey = "u5r-debug-workbench-navigation-recents"
const token = document.querySelector<HTMLMetaElement>('meta[name="u5r-workbench-token"]')?.content ?? ""

function restoredHistoryDock(): HistoryDock {
  return localStorage.getItem(historyDockStorageKey) === "right" ? "right" : "bottom"
}

function restoredNavigationRecents(): string[] {
  try {
    const stored = JSON.parse(localStorage.getItem(navigationRecentsStorageKey) ?? "[]")
    return Array.isArray(stored) ? stored.filter((value): value is string => typeof value === "string").slice(0, 8) : []
  } catch {
    return []
  }
}

const connectionState = ref<ConnectionState>("connecting")
const socket = shallowRef<WebSocket | null>(null)
const sessionID = ref(localStorage.getItem(sessionStorageKey) ?? "")
const sceneGeneration = ref(0)
const optionsRevision = ref(0)
const awaitingOptionsRefresh = ref(false)
const commands = ref<CommandDefinition[]>([])
const optionSets = ref<OptionSets>({})
const optionUnavailableReasons = ref<Record<string, string>>({})
const selectedCommandID = ref(localStorage.getItem(instrumentStorageKey) ?? "")
const activeWorkspace = ref<WorkspaceID>("home")
const pageHidden = ref(document.hidden)
const workspaceSelections = new Map<WorkspaceID, string>()
const ownerErrors = ref<Record<string, string>>({})
provideInstrumentSession(computed(() => `${sessionID.value}:${sceneGeneration.value}:${connectionState.value}:${activeWorkspace.value}:${selectedCommandID.value}:${pageHidden.value}`), (message) => {
  ownerErrors.value[selectedCommandID.value] = message
})
const selectedShrineIndex = ref(0)
const fightRoomRequest = ref<{ locationName: string; room: number; token: number }>()
let fightRoomSequence = 0
const drafts = ref<Record<string, CommandDraft>>({})
const dirtyFields = ref<Record<string, string[]>>({})
const formErrors = ref<Record<string, FormErrors>>({})
const formMessages = ref<Record<string, string>>({})
const pending = ref(new Map<string, PendingRequest>())
const results = ref<ResultEntry[]>([])
const latestResults = ref<Record<string, ResultEntry>>({})
const lastSuccessfulWorldPositionResult = ref<ResultEntry>()
const trimmedResults = ref(0)
const reconnectAttempt = ref(0)
const stream = ref<HTMLElement | null>(null)
const historyPanel = ref<HTMLElement | null>(null)
const followResults = ref(true)
const historyCollapsed = ref(localStorage.getItem(historyCollapsedStorageKey) === "true")
const historyDock = ref<HistoryDock>(restoredHistoryDock())
const historyHeight = ref(220)
const historyWidth = ref(420)
const lastCloseReason = ref("")
const retryScheduled = ref(false)
const liveStatus = ref("Workbench initialized.")
const now = ref(Date.now())
const processedRequestIDs = new Set<string>()
const processedRequestOrder: string[] = []
const silentRequestIDs = new Set<string>()
const commandPaletteOpen = ref(false)
const navigationRecents = ref<string[]>(restoredNavigationRecents())
let reconnectTimer: number | undefined
let connectionTimer: number | undefined
let operationTimer: number | undefined
let requestSequence = 0
let connectionGeneration = 0
let localEntrySequence = 0
let stopHistoryResize: (() => void) | undefined
let commandPalettePreviousFocus: HTMLElement | null = null
let merchantCatalogExplicitRefreshRequired = false

watch(selectedCommandID, (commandID) => {
  if (activeWorkspace.value !== "home") workspaceSelections.set(activeWorkspace.value, commandID)
  if (commandID) localStorage.setItem(instrumentStorageKey, commandID)
  else localStorage.removeItem(instrumentStorageKey)
})
watch(historyCollapsed, (collapsed) => localStorage.setItem(historyCollapsedStorageKey, String(collapsed)))
watch(historyDock, (dock) => localStorage.setItem(historyDockStorageKey, dock))
watch([sessionID, sceneGeneration], () => {
  lastSuccessfulWorldPositionResult.value = undefined
}, { flush: "sync" })
const connectionLabel = computed(() => connectionState.value.replaceAll("_", " ").toUpperCase())
const historyPanelStyle = computed(() => historyDock.value === "right"
  ? { width: historyCollapsed.value ? "38px" : `${historyWidth.value}px` }
  : { height: historyCollapsed.value ? undefined : `${historyHeight.value}px` })
const isReady = computed(() => connectionState.value === "ready")
const supportedCommands = computed(() => commands.value.filter(isSupportedWorkbenchCommand))
const availableCommands = computed(() => supportedCommands.value
  .filter((command) => ownerTask(command) === activeWorkspace.value)
  .sort((left, right) => compareToolOrder(left.id, right.id)))
const toolGroups = computed<ToolGroupView[]>(() => {
  if (activeWorkspace.value !== "more") return [{ commands: availableCommands.value }]
  const grouped = new Map<ToolGroup, CommandDefinition[]>()
  for (const command of availableCommands.value) {
    const group = toolDefinitions[command.id]?.group ?? "For developers"
    grouped.set(group, [...(grouped.get(group) ?? []), command])
  }
  return [...grouped].map(([name, groupCommands]) => ({ name, commands: groupCommands })).sort((left, right) => toolGroupOrder(left.name!) - toolGroupOrder(right.name!))
})
const selectedCommand = computed(() => availableCommands.value.find((command) => command.id === selectedCommandID.value))
const selectedQueryFailure = computed(() => {
  const command = selectedCommand.value
  if (!command) return undefined
  const result = latestResults.value[command.id]
  return result && !result.success ? result : undefined
})
const latestOperationResult = computed(() => {
  const owner = selectedCommand.value
  if (!owner) return undefined
  return [...results.value].reverse().find((result) => {
    const command = commands.value.find((candidate) => candidate.id === result.commandId)
    return command !== undefined && (result.ownerCommandID ?? companionOwnerCommandID(result.commandId)) === owner.id
  })
})
const latestWindResult = computed(() => latestResults.value[windStatusCommandID])
const latestLoggingResult = computed(() => latestResults.value[loggingStatusCommandID])
const latestShrineStatusResult = computed(() => latestResults.value[shrineStatusCommandID])
const latestCalendarResult = computed(() => latestResults.value[calendarStatusCommandID])
const currentCalendar = computed(() => latestCalendarResult.value?.data ? calendarStatusData(latestCalendarResult.value.data) : undefined)
const latestSearchResult = computed(() => latestResults.value[searchStatusCommandID])
const latestQuestItemResult = computed(() => latestResults.value[questItemStatusCommandID])
const latestWorldPositionResult = computed(() => latestResults.value[worldPositionStatusCommandID])
const latestMoongateResult = computed(() => latestResults.value[moongateStatusCommandID])
const latestCombatEncounterOptionsResult = computed(() => latestResults.value[combatEncounterOptionsCommandID])
const latestMerchantTesterCatalogResult = computed(() => latestResults.value[merchantTesterCatalogCommandID])
const latestMonsterEcosystemResult = computed(() => latestResults.value[monsterEcosystemStatusCommandID])
const latestVehicleStatusResult = computed(() => latestResults.value[vehicleStatusCommandID])
const latestInventoryLockerResult = computed(() => latestResults.value[inventoryLockerStatusCommandID])
const latestSpellInventoryResult = computed(() => latestResults.value[spellInventoryStatusCommandID])
const latestActiveCombatResult = computed(() => latestResults.value[activeCombatStatusCommandID])
const currentCombat = computed(() => latestActiveCombatResult.value?.data
  ? activeCombatStatusData(latestActiveCombatResult.value.data)
  : undefined)
const quickCombatExitDisabled = computed(() => {
	const combat = currentCombat.value
	return !isReady.value || commandPending(activeCombatExitCommandID) || !combat?.active || combat.workflowActive
		|| combat.map?.pendingAdvanceTurn || combat.map?.activeMonsterIndex !== -1
})
const latestDebugOptionsResult = computed(() => latestResults.value[debugOptionsStatusCommandID])
const freeMoveEnabled = computed(() => latestDebugOptionsResult.value?.data ? debugOptionsStatusData(latestDebugOptionsResult.value.data).freeMove : false)
const latestGuardStatusResult = computed(() => latestResults.value[guardStatusCommandID])
const latestShadowlordStatusResult = computed(() => latestResults.value[shadowlordStatusCommandID])
const latestDeathStatusResult = computed(() => latestResults.value[deathStatusCommandID])
const latestTestSetupStatusResult = computed(() => latestResults.value[testSetupStatusCommandID])
const latestSaveListResult = computed(() => latestResults.value[saveSnapshotListCommandID])
const latestSaveDetailResult = computed(() => latestResults.value[saveSnapshotGetCommandID])
const saveList = computed(() => latestSaveListResult.value?.data ? saveSnapshotListData(latestSaveListResult.value.data) : undefined)
const latestPartyCharacterInspectResult = computed(() => latestResults.value[partyCharacterInspectCommandID])
const latestPartyEquipmentLoadoutResult = computed(() => latestResults.value[partyEquipmentLoadoutCommandID])
const latestObservationResult = computed(() => [...results.value].reverse().find((result) =>
  observationCommandIDs.includes(result.commandId as ObservationCommandID) && result.success && result.data !== undefined,
))
const observationPending = computed(() => observationCommandIDs.some(commandPending))
const worldLandmarkChoices = computed(() => commands.value.find((command) => command.id === movementLandmarkTravelCommandID)?.parameters?.find((parameter) => parameter.id === "landmark")?.choices ?? [])
const navigationSurfaces = computed<NavigationSurface[]>(() => supportedCommands.value.flatMap((command) => {
  const task = ownerTask(command)
  if (!task) return []
  return [{
    id: `${task}:${command.id}`,
    workspace: task,
    workspaceLabel: taskDefinition(task).label,
    label: toolName(command),
    description: toolDescription(command),
    commandId: command.id,
    commands: navigationSurfaceCommands(command),
  }]
}))
const homeRecentSurfaces = computed(() => recentNavigationSurfaces(navigationSurfaces.value, navigationRecents.value))
const topBarTestSetup = computed(() => latestTestSetupStatusResult.value?.data ? latestTestSetupStatusResult.value.data as TestSetupStatusData : undefined)
const topBarWhere = computed(() => topBarTestSetup.value ? placeLabel(topBarTestSetup.value) : undefined)
const topBarClock = computed<TopBarClock | undefined>(() => currentCalendar.value)
const topBarParty = computed<TopBarParty | undefined>(() => partyHealthSummary(topBarTestSetup.value?.party))
const topBarMode = computed(() => currentCombat.value ? (currentCombat.value.active ? "combat" : "exploring") : undefined)
const homeQuickState = ref<{ requestID?: string; error?: string }>()
const homeFeedback = computed(() => {
  const state = homeQuickState.value
  if (!state) return undefined
  if (state.error) return { success: false, text: state.error }
  const result = results.value.find((candidate) => candidate.requestId === state.requestID)
  if (result) {
    return result.success
      ? { success: true, text: result.summary || `${commandName(result.commandId)} completed.` }
      : { success: false, text: `${commandName(result.commandId)} failed: ${result.errorMessage ?? result.errorCode ?? "unknown error"}` }
  }
  return state.requestID && pending.value.has(state.requestID) ? { success: true, text: "Working…" } : undefined
})
const homeCards = computed<HomeCard[]>(() => homeTaskIDs.map((task) => ({ task: taskDefinition(task), quick: homeQuickActions(task) })))
const retryDetail = computed(() => {
  if (retryScheduled.value) return `automatic retry ${reconnectAttempt.value} scheduled`
  if (["connecting", "reconnecting", "handshaking"].includes(connectionState.value)) return "connecting now"
  return "automatic retry stopped"
})

const activePage = computed(() => activeWorkspace.value === "home" ? undefined : pageDefinitions[activeWorkspace.value])
const batch = ref<BatchState>()
const batchNotice = ref<PageNotice>()
const homeRestorePending = ref(false)
const pageQueryIDs = new Set(Object.values(pageDefinitions).flatMap((page) => [...page.queries, ...(page.loadOnce ?? [])]))
const pageMutationPending = computed(() => {
  const page = activePage.value
  if (!page) return false
  const owned = new Set([...page.commands, ...(page.uses ?? [])])
  return [...pending.value.values()].some((request) => owned.has(request.commandID) && !pageQueryIDs.has(request.commandID))
})
const pageBusy = computed(() => batch.value !== undefined || pageMutationPending.value)
const rosterFingerprint = computed(() => {
  const result = latestResults.value[partyCharacterInspectCommandID]
  return result?.success && result.data ? (result.data as PartyCharacterInspectData).fingerprint : undefined
})
const fightPreselect = computed(() => {
  const request = fightRoomRequest.value
  const options = latestResults.value[combatEncounterOptionsCommandID]?.data as CombatEncounterOptionsData | undefined
  const found = request && options?.dungeons.find((candidate) => candidate.name === request.locationName)
  return request && found ? { dungeon: found.id, room: request.room, token: request.token } : undefined
})

interface BatchState {
  steps: PageStep[]
  index: number
  total: number
  owner: string
  sentRequestID?: string
  /** Commands (or "catalog") whose fresh answer the next step needs first. */
  waiting: Set<string>
}

function connectionLost(): void {
  if (batch.value) batchNotice.value = { success: false, text: `The connection was lost after ${batch.value.index} of ${batch.value.total} changes. Check the game before applying again.` }
  batch.value = undefined
}

function stepArguments(step: PageStep): Record<string, string> {
  const locker = latestResults.value[inventoryLockerStatusCommandID]?.data as InventoryLockerStatusData | undefined
  return Object.fromEntries(Object.entries(step.argumentsByID).map(([key, value]) => [key, value === "$locker" ? locker?.snapshotFingerprint ?? "" : value]))
}

/** Sends a page's staged changes one command at a time, refreshing what the next command was reviewed against. */
function runPageSteps(steps: PageStep[]): void {
  const page = activePage.value
  if (!page || steps.length === 0 || batch.value) return
  batchNotice.value = undefined
  batch.value = { steps, index: 0, total: steps.length, owner: page.anchor, waiting: new Set() }
  sendBatchStep()
}

function failBatch(text: string): void {
  const state = batch.value
  batch.value = undefined
  batchNotice.value = { success: false, text: state && state.total > 1 ? `Stopped after ${state.index} of ${state.total} changes. ${text}` : text }
}

function sendBatchStep(): void {
  const state = batch.value
  if (!state) return
  const step = state.steps[state.index]
  const sent = runSpecializedMutation(step.commandID, stepArguments(step), (requestID) => { state.sentRequestID = requestID })
  if (!sent) failBatch(ownerErrors.value[state.owner] || `${commandName(step.commandID)} could not be sent.`)
}

function advanceBatchAfter(result: CommandResult): void {
  const state = batch.value
  if (!state || state.sentRequestID !== result.requestId) return
  if (!result.success) {
    const stale = result.errorCode?.startsWith("stale") ? " That changed in the game since this page loaded; the page was refreshed. Review it and apply again." : ""
    failBatch(`${commandName(result.commandId)} failed: ${result.errorMessage ?? result.errorCode ?? "unknown error"}.${stale}`)
    if (stale) runPageQueries(activePage.value)
    return
  }
  state.index += 1
  state.sentRequestID = undefined
  if (state.index >= state.total) {
    batch.value = undefined
    batchNotice.value = { success: true, text: state.total === 1 ? result.summary || `${commandName(result.commandId)} completed.` : `Applied ${state.total} changes.` }
    return
  }
  const next = state.steps[state.index]
  const nextCommand = commands.value.find((candidate) => candidate.id === next.commandID)
  if (Object.values(next.argumentsByID).includes("$locker")) {
    state.waiting.add(inventoryLockerStatusCommandID)
    runQueryByID(inventoryLockerStatusCommandID, true)
  }
  if (nextCommand && commandUsesDynamicOptions(nextCommand)) {
    state.waiting.add("catalog")
    requestCatalog()
  }
  if (state.waiting.size === 0) void nextTick(sendBatchStep)
}

function batchStopWaiting(what: string): void {
  const state = batch.value
  if (!state?.waiting.delete(what) || state.waiting.size > 0 || state.sentRequestID) return
  void nextTick(sendBatchStep)
}

function runPageQueries(page: typeof activePage.value): void {
  if (!page) return
  for (const commandID of page.queries) runQueryByID(commandID, true)
  for (const commandID of page.loadOnce ?? []) {
    const latest = latestResults.value[commandID]
    if (!latest || latest.sceneGeneration !== sceneGeneration.value || latest.sessionId !== sessionID.value) runQueryByID(commandID, true)
  }
}

function pageOwnerFor(commandID: string): string | undefined {
  const page = activePage.value
  return page && (page.commands.includes(commandID) || page.uses?.includes(commandID)) ? page.anchor : undefined
}

function openFightForRoom(value: { locationName: string; room: number }): void {
  fightRoomRequest.value = { ...value, token: ++fightRoomSequence }
  activeWorkspace.value = "fight"
  selectedCommandID.value = activeCombatStatusCommandID
  void nextTick(autoExecuteSelectedQuery)
}

// More tools that refresh while they are on screen.
const polledMoreQueryIDs = new Set([loggingStatusCommandID, shrineStatusCommandID, searchStatusCommandID, questItemStatusCommandID, monsterEcosystemStatusCommandID, vehicleStatusCommandID, debugOptionsStatusCommandID, testSetupStatusCommandID])

function websocketURL(): string {
  const scheme = location.protocol === "https:" ? "wss:" : "ws:"
  return `${scheme}//${location.host}/ws?token=${encodeURIComponent(token)}`
}

function connect(): void {
  window.clearTimeout(reconnectTimer)
  window.clearTimeout(connectionTimer)
  retryScheduled.value = false
  const generation = ++connectionGeneration
  const previousSocket = socket.value
  socket.value = null
  previousSocket?.close(1000, "connection replaced")
  connectionState.value = reconnectAttempt.value > 0 ? "reconnecting" : "connecting"
  const nextSocket = new WebSocket(websocketURL())
  socket.value = nextSocket
  connectionTimer = window.setTimeout(() => nextSocket.close(1013, "connection timeout"), connectionTimeoutMs)

  nextSocket.addEventListener("open", () => {
    if (generation !== connectionGeneration || socket.value !== nextSocket) {
      nextSocket.close(1000, "stale connection")
      return
    }
    window.clearTimeout(connectionTimer)
    connectionState.value = "handshaking"
    connectionTimer = window.setTimeout(() => nextSocket.close(1013, "session handshake timeout"), connectionTimeoutMs)
    if (!send(envelope("hello", "session.hello", { sessionId: sessionID.value }), nextSocket)) {
      nextSocket.close(1011, "session handshake send failed")
    }
  })
  nextSocket.addEventListener("message", (event) => {
    if (generation !== connectionGeneration || socket.value !== nextSocket) return
    try {
      handleMessage(parseEnvelope(String(event.data)))
    } catch {
      receiveProtocolError({
        version: 1,
        id: "decode",
        type: "protocol.error",
        payload: { code: "invalid_json", message: "The server sent malformed JSON." },
      })
      nextSocket.close(1002, "invalid protocol message")
    }
  })
  nextSocket.addEventListener("close", (event) => {
    if (generation !== connectionGeneration || socket.value !== nextSocket) return
    socket.value = null
    window.clearTimeout(connectionTimer)
    connectionState.value = "disconnected"
    lastCloseReason.value = event.reason || `socket closed (${event.code})`
    if (event.code === 1002 || event.code === 1008) return
    scheduleReconnect()
  })
  nextSocket.addEventListener("error", () => nextSocket.close())
}

function scheduleReconnect(): void {
  reconnectAttempt.value += 1
  retryScheduled.value = true
  const delay = Math.min(500 * 2 ** (reconnectAttempt.value - 1), 5000)
  reconnectTimer = window.setTimeout(connect, delay)
}

function retryNow(): void {
  reconnectAttempt.value = 0
  retryScheduled.value = false
  lastCloseReason.value = ""
  connect()
}

function send(message: Envelope, target = socket.value): boolean {
  if (target?.readyState !== WebSocket.OPEN) return false
  try {
    target.send(JSON.stringify(message))
    return true
  } catch {
    return false
  }
}

function parseEnvelope(raw: string): Envelope {
  const parsed = JSON.parse(raw) as Partial<Envelope>
  if (parsed.version !== 1 || typeof parsed.type !== "string") throw new Error("invalid protocol envelope")
  return parsed as Envelope
}

function handleMessage(message: Envelope): void {
  switch (message.type) {
    case "session.ready": {
      const ready = message.payload as SessionReady
      if (!ready.resumed) {
        lastSuccessfulWorldPositionResult.value = undefined
        reportLostPendingRequests()
        pending.value.clear()
      }
      sessionID.value = ready.sessionId
      sceneGeneration.value = ready.sceneGeneration
      localStorage.setItem(sessionStorageKey, ready.sessionId)
      window.clearTimeout(connectionTimer)
      reconnectAttempt.value = 0
      lastCloseReason.value = ""
      connectionState.value = "ready"
      if (ready.resumed) {
        for (const [requestID, request] of pending.value) {
          pending.value.set(requestID, { ...request, lastConfirmedAt: Date.now(), statusRequested: false })
          send(envelope(requestID, "operation.status", { requestId: requestID }))
        }
      }
      requestCatalog()
      break
    }
    case "catalog.snapshot": {
      const catalog = message.payload as CatalogSnapshot
      commands.value = catalog.commands
      optionSets.value = Object.fromEntries((catalog.optionSets ?? []).map((optionSet) => [optionSet.provider, optionSet.choices]))
      optionUnavailableReasons.value = Object.fromEntries(
        (catalog.optionSets ?? []).filter((optionSet) => optionSet.unavailableReason).map((optionSet) => [optionSet.provider, optionSet.unavailableReason ?? ""]),
      )
      optionsRevision.value = catalog.optionsRevision
      awaitingOptionsRefresh.value = false
      const nextDrafts: Record<string, CommandDraft> = {}
      const nextDirtyFields: Record<string, string[]> = {}
      for (const command of catalog.commands) {
        const existing = drafts.value[command.id] ?? {}
        if (command.id === partyCharacterCommandID || command.id === partyEquipmentLoadoutCommandID) {
          const reconciled = reconcilePartyCharacterDraft(command, existing, dirtyFields.value[command.id] ?? [], optionSets.value)
          nextDrafts[command.id] = reconciled.draft
          if (command.id === partyCharacterCommandID) nextDirtyFields[command.id] = reconciled.dirtyFields
          if (existing.character && existing.character !== reconciled.draft.character) clearCharacterAlterationErrors(command.id)
        } else {
          nextDrafts[command.id] = reconcileDraft(command, existing, optionSets.value)
        }
      }
      drafts.value = nextDrafts
      dirtyFields.value = nextDirtyFields
      if (activeWorkspace.value !== "home" && taskCommands(catalog.commands, activeWorkspace.value).length === 0) activeWorkspace.value = "home"
      if (activeWorkspace.value === "home") {
        if (!catalog.commands.some((command) => isSupportedWorkbenchCommand(command) && command.id === selectedCommandID.value)) {
          selectedCommandID.value = catalog.commands.find(isSupportedWorkbenchCommand)?.id ?? ""
        }
      } else {
        const workspaceCommands = taskCommands(catalog.commands, activeWorkspace.value)
        if (!workspaceCommands.some((command) => command.id === selectedCommandID.value)) selectedCommandID.value = workspaceCommands[0]?.id ?? ""
      }
      sceneGeneration.value = catalog.sceneGeneration
      batchStopWaiting("catalog")
		void nextTick(() => {
			if (!globalStatusCommandIDs.includes(selectedCommandID.value)) autoExecuteSelectedQuery()
      for (const globalID of globalStatusCommandIDs) {
        const latest = latestResults.value[globalID]
        if (!latest || latest.sceneGeneration !== sceneGeneration.value || latest.sessionId !== sessionID.value) runQueryByID(globalID, true)
      }
			runQueryByID(debugOptionsStatusCommandID, true)
		})
      break
    }
    case "command.accepted": {
      const accepted = message.payload as { requestId: string; commandId: string }
      const existing = pending.value.get(accepted.requestId)
      if (processedRequestIDs.has(accepted.requestId)) {
        send(envelope(`ack-${accepted.requestId}`, "command.ack", { requestId: accepted.requestId }))
      } else if (existing?.phase !== "accepted") {
        pending.value.set(accepted.requestId, {
          commandID: accepted.commandId,
          ownerCommandID: existing?.ownerCommandID,
          phase: "accepted",
          startedAt: existing?.startedAt ?? Date.now(),
          lastConfirmedAt: Date.now(),
          statusRequested: false,
        })
      }
      break
    }
    case "command.completed":
    case "command.failed":
      receiveResult(message.payload as CommandResult)
      break
    case "protocol.error":
      receiveProtocolError(message)
      break
  }
}

function runCommand(command: CommandDefinition, silent = false, onSent?: (requestID: string) => void): boolean {
  if (!isReady.value || !isSupportedWorkbenchCommand(command) || commandPending(command.id) || hasUnsupportedParameters(command, optionSets.value) || commandUnavailableReason(command)) return false
  const errors = validateDraft(command, draftFor(command), optionSets.value)
	if (command.id === partyCharacterCommandID) Object.assign(errors, partyResourceErrors(command, optionSets.value, draftFor(command), dirtyFieldsFor(command)))
  formErrors.value[command.id] = errors
  if (Object.keys(errors).length > 0) {
    formMessages.value[command.id] = "Correct the highlighted fields before running this operation."
    void nextTick(() => firstInvalidControl(command.id)?.focus())
    return false
  }
  if (command.id === partyCharacterCommandID && dirtyFieldsFor(command).length === 0) {
    formMessages.value[command.id] = "Choose at least one character change before applying."
    return false
  }
  if (command.id === partyCharacterCommandID && partyAlterationPreview(command, optionSets.value, draftFor(command), dirtyFieldsFor(command)).length === 0) {
    formMessages.value[command.id] = "The requested values already match the selected character."
    return false
  }
  delete formMessages.value[command.id]
  const requestID = `query-${Date.now()}-${++requestSequence}`
  const sent = send(envelope(requestID, "command.execute", {
    commandId: command.id,
    expectedSceneGeneration: sceneGeneration.value,
    ...(commandUsesDynamicOptions(command) ? { expectedOptionsRevision: optionsRevision.value } : {}),
    arguments: command.id === partyCharacterCommandID
      ? serializePartyCharacterDraft(draftFor(command), dirtyFieldsFor(command))
      : serializeDraft(command, draftFor(command)),
  }))
  if (sent) {
    if (silent) silentRequestIDs.add(requestID)
    pending.value.set(requestID, {
      commandID: command.id,
      phase: "sending",
      startedAt: Date.now(),
      lastConfirmedAt: Date.now(),
      statusRequested: false,
    })
    onSent?.(requestID)
  } else {
    rememberProcessedRequest(requestID)
    appendResult({
      entryID: `transport-${++localEntrySequence}`,
      sessionId: sessionID.value,
      requestId: requestID,
      commandId: command.id,
      sceneGeneration: sceneGeneration.value,
      success: false,
      errorCode: "transport_unavailable",
      errorMessage: "The command could not be sent to the game process.",
      receivedAt: new Date(),
    })
    socket.value?.close(1011, "command send failed")
  }
  return sent
}

function draftFor(command: CommandDefinition): CommandDraft {
  return drafts.value[command.id] ?? {}
}

function dirtyFieldsFor(command: CommandDefinition): string[] {
  return dirtyFields.value[command.id] ?? []
}

function errorFor(commandID: string, parameterID: string): string | undefined {
  return formErrors.value[commandID]?.[parameterID]
}

function describedBy(command: CommandDefinition, parameterID: string): string | undefined {
  const parameter = command.parameters?.find((candidate) => candidate.id === parameterID)
  const identifiers: string[] = []
  if (parameter?.description) identifiers.push(`${command.id}-${parameterID}-help`)
  if (errorFor(command.id, parameterID)) identifiers.push(`${command.id}-${parameterID}-error`)

  return identifiers.length > 0 ? identifiers.join(" ") : undefined
}

function updateDraft(command: CommandDefinition, parameterID: string, value: string): void {
  const current = draftFor(command)
  if (command.id === partyCharacterCommandID && parameterID === "character" && current.character !== value) {
    drafts.value[command.id] = hydratePartyCharacterDraft(command, value, optionSets.value)
    dirtyFields.value[command.id] = []
    clearCharacterAlterationErrors(command.id)
  } else {
    drafts.value[command.id] = { ...current, [parameterID]: value }
    if (command.id === partyCharacterCommandID && parameterID !== "character") {
      const nextDirty = new Set(dirtyFieldsFor(command))
      if (partyFieldIsDirty(command, optionSets.value, current, parameterID, value)) {
        nextDirty.add(parameterID)
      } else {
        nextDirty.delete(parameterID)
      }
      dirtyFields.value[command.id] = [...nextDirty]
    }
  }
  if (formErrors.value[command.id]?.[parameterID]) {
    const errors = { ...formErrors.value[command.id] }
    delete errors[parameterID]
    formErrors.value[command.id] = errors
  }
  delete formMessages.value[command.id]
}

function resetCommand(command: CommandDefinition): void {
  drafts.value[command.id] = command.id === partyCharacterCommandID
    ? hydratePartyCharacterDraft(command, draftFor(command).character ?? "", optionSets.value)
    : resetDraft(command, optionSets.value)
  if (command.id === partyCharacterCommandID) dirtyFields.value[command.id] = []
  delete formErrors.value[command.id]
  delete formMessages.value[command.id]
}

function selectCommand(command: CommandDefinition): void {
  if (command.id === merchantTesterCatalogCommandID) merchantCatalogExplicitRefreshRequired = false
  selectedCommandID.value = command.id
  if (commandUsesDynamicOptions(command)) requestCatalog()
  void nextTick(autoExecuteSelectedQuery)
}

function selectWorkspace(workspace: WorkspaceID): void {
  if (activeWorkspace.value !== "home") workspaceSelections.set(activeWorkspace.value, selectedCommandID.value)
  activeWorkspace.value = workspace
  if (workspace === "home") return
  const workspaceCommands = taskCommands(supportedCommands.value, workspace)
  const remembered = workspaceSelections.get(workspace) ?? selectedCommandID.value
  selectedCommandID.value = workspaceCommands.some((command) => command.id === remembered) ? remembered : workspaceCommands[0]?.id ?? ""
  if (selectedCommandID.value === merchantTesterCatalogCommandID) merchantCatalogExplicitRefreshRequired = false
  void nextTick(autoExecuteSelectedQuery)
}

function selectShrine(index: number): void {
  selectedShrineIndex.value = index
}

function activateCommand(command: CommandDefinition): void {
  selectCommand(command)
  void nextTick(() => firstInstrumentControl()?.focus())
}

function firstInvalidControl(commandID: string): HTMLInputElement | HTMLSelectElement | null {
  return document.querySelector(`[data-command-form="${commandID}"] [aria-invalid="true"]`)
}

function firstInstrumentControl(): HTMLInputElement | HTMLSelectElement | HTMLButtonElement | null {
  return document.querySelector(".instrument form input, .instrument form select, .instrument form button:not(:disabled)")
}

function receiveResult(result: CommandResult): void {
  const firstReceipt = !processedRequestIDs.has(result.requestId)
  send(envelope(`ack-${result.requestId}`, "command.ack", { requestId: result.requestId }))
  if (!firstReceipt) return

  rememberProcessedRequest(result.requestId)
  const ownerCommandID = pending.value.get(result.requestId)?.ownerCommandID
  pending.value.delete(result.requestId)
  sceneGeneration.value = result.sceneGeneration
  const silent = silentRequestIDs.delete(result.requestId)
  const entry = { ...result, ownerCommandID, entryID: `result-${result.requestId}`, receivedAt: new Date() }
  if (result.commandId === worldPositionStatusCommandID && result.success && result.data && result.sessionId === sessionID.value) {
    lastSuccessfulWorldPositionResult.value = entry
  }
  if (silent) {
    latestResults.value = { ...latestResults.value, [result.commandId]: entry }
  } else {
    appendResult(entry)
  }
  if (result.success && result.commandId === partyCharacterCommandID) dirtyFields.value[result.commandId] = []
  if (result.errorCode === "stale_options") {
    awaitingOptionsRefresh.value = true
    formMessages.value[result.commandId] = "Available party options changed. Review the refreshed selection before retrying."
  }
  if (result.success && merchantTesterOperationCommandIDs.includes(result.commandId)) merchantCatalogExplicitRefreshRequired = true
  if (!result.success && result.errorCode === "stale_world_position" && result.commandId === worldPositionSetCommandID) {
    void nextTick(() => runQueryByID(worldPositionStatusCommandID, true))
  }
  if (result.errorCode === "stale_options" || result.effectiveExecutionMode && result.effectiveExecutionMode !== "query") requestCatalog()
  advanceBatchAfter(result)
  batchStopWaiting(result.commandId)
  if (result.success && result.commandId === partyCharacterInspectCommandID && homeRestorePending.value && result.data) {
    homeRestorePending.value = false
    const fingerprint = (result.data as PartyCharacterInspectData).fingerprint
    const sent = runSpecializedMutation(partyRestoreCommandID, { expectedFingerprint: fingerprint }, (requestID) => { homeQuickState.value = { requestID } })
    if (!sent) homeQuickState.value = { error: ownerErrors.value[companionOwnerCommandID(partyRestoreCommandID)] || "The action could not be sent." }
  }
  const finishedCommand = commands.value.find((candidate) => candidate.id === result.commandId)
  if (result.success && finishedCommand && isMutation(finishedCommand) && !pageQueryIDs.has(result.commandId)) {
    for (const page of Object.values(pageDefinitions)) {
      if (page.commands.includes(result.commandId) || page.uses?.includes(result.commandId)) void nextTick(() => { for (const commandID of page.queries) runQueryByID(commandID, true) })
    }
  }
  if (result.success && result.commandId === windSetCommandID) void nextTick(() => runQueryByID(windStatusCommandID, true))
  if (result.success && loggingMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(loggingStatusCommandID, true))
  if (result.success && (result.commandId === shrineProgressCommandID || result.commandId === codexQuestStateSetAllCommandID)) void nextTick(() => runQueryByID(shrineStatusCommandID, true))
  if (result.success && (result.commandId === calendarSetTimeCommandID || result.commandId === calendarSetDayCommandID)) void nextTick(() => runQueryByID(calendarStatusCommandID, true))
  if (result.success && (result.commandId === searchItemStackAddCommandID || result.commandId === searchProgressResetCommandID || result.commandId === commandTestSearchCommandID)) void nextTick(() => runQueryByID(searchStatusCommandID, true))
  if (result.success && result.commandId === searchOverlaySetCommandID) void nextTick(() => runQueryByID(searchStatusCommandID, true))
  if (result.success && (result.commandId === questItemEnsureCommandID || result.commandId === questItemRemoveCommandID || result.commandId === questShardUseCommandID)) void nextTick(() => runQueryByID(questItemStatusCommandID, true))
  if (result.success && (result.commandId === worldPositionSetCommandID || result.commandId === movementLandmarkTravelCommandID)) void nextTick(() => runQueryByID(worldPositionStatusCommandID, true))
  if (result.success && testSetupMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(testSetupStatusCommandID, true))
  if (result.success && [saveSnapshotCaptureCommandID, saveSnapshotDeleteCommandID].includes(result.commandId)) {
    void nextTick(() => runQueryByID(saveSnapshotListCommandID, true))
  }
  if (result.success && result.commandId === saveSnapshotLoadCommandID) {
    requestCatalog()
    void nextTick(() => {
      runQueryByID(calendarStatusCommandID, true)
      runQueryByID(saveSnapshotListCommandID, true)
      runQueryByID(testSetupStatusCommandID, true)
    })
  }
  if (result.success && (result.commandId === deathKillPartyCommandID || result.commandId === deathTestSequenceCommandID)) void nextTick(() => runQueryByID(deathStatusCommandID, true))
  if (result.success && (result.commandId === moongateOverlaySetCommandID || result.commandId === moongateTravelCommandID)) void nextTick(() => runQueryByID(moongateStatusCommandID, true))
  if (result.success && [monsterGenerationSetCommandID, monsterClearMapEnemiesCommandID, monsterSpawnWhirlpoolCommandID, monsterSpawnPirateCommandID].includes(result.commandId)) {
    void nextTick(() => runQueryByID(monsterEcosystemStatusCommandID, true))
  }
	if (result.success && shipMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(vehicleStatusCommandID, true))
	if (result.success && debugOptionsMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(debugOptionsStatusCommandID, true))
  if (result.success && inventoryLockerMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(inventoryLockerStatusCommandID, true))
  if (result.success && spellInventoryMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(spellInventoryStatusCommandID, true))
	if (result.success && activeCombatMutationCommandIDs.includes(result.commandId)) void nextTick(() => runQueryByID(activeCombatStatusCommandID, true))
	if (result.success && (partyRosterMutationCommandIDs.includes(result.commandId) || result.commandId === partyKarmaSetCommandID)) {
		void nextTick(() => runQueryByID(partyCharacterInspectCommandID, true))
	}
	if (result.success && result.commandId === partyEquipmentLoadoutCommandID) {
		void nextTick(() => {
			runQueryByID(partyCharacterInspectCommandID, true)
			runQueryByID(inventoryLockerStatusCommandID, true)
		})
	}
}

function requestCatalog(): void {
  if (!send(envelope(`catalog-${Date.now()}`, "catalog.request", {}))) socket.value?.close(1011, "catalog request send failed")
}

function isSupportedWorkbenchCommand(command: CommandDefinition): boolean {
  if (integratedCommandIDs.has(command.id)) return true
  return !command.destructive && command.executionModes.some((mode) => mode === "query" || mode === "live_safe")
}

/** The task page that lists this command as a tool; undefined for companion commands owned by another tool. */
function ownerTask(command: CommandDefinition): TaskID | undefined {
  const defined = toolDefinitions[command.id]
  if (defined) return defined.task
  return integratedCommandIDs.has(command.id) ? undefined : "more"
}

function taskCommands(catalog: CommandDefinition[], task: TaskID): CommandDefinition[] {
  return catalog.filter((command) => isSupportedWorkbenchCommand(command) && ownerTask(command) === task)
    .sort((left, right) => compareToolOrder(left.id, right.id))
}

function toolName(command: CommandDefinition): string {
  return toolDefinitions[command.id]?.name ?? command.name
}

function toolDescription(command: CommandDefinition): string {
  return toolDefinitions[command.id]?.description ?? command.description
}

function companionOwnerCommandID(commandID: string): string {
	for (const page of Object.values(pageDefinitions)) if (page.commands.includes(commandID)) return page.anchor
	if (commandID === combatPresentationSpeedSetCommandID && selectedCommandID.value === activeCombatStatusCommandID) return activeCombatStatusCommandID
  if (loggingMutationCommandIDs.includes(commandID)) return loggingStatusCommandID
  if (commandID === windSetCommandID) return windStatusCommandID
  if ([shrineProgressCommandID, codexQuestStateSetAllCommandID].includes(commandID)) return shrineStatusCommandID
  if (partyRosterMutationCommandIDs.includes(commandID) || commandID === partyKarmaSetCommandID) return partyCharacterInspectCommandID
  if (commandID === partyCharacterInspectCommandID) return partyCharacterInspectCommandID
  if (commandID === partyEquipmentLoadoutCommandID) return partyEquipmentLoadoutCommandID
  if (commandID === partyCharacterCommandID) return partyCharacterCommandID
  if ([calendarSetTimeCommandID, calendarSetDayCommandID].includes(commandID)) return calendarStatusCommandID
  if ([searchItemStackAddCommandID, searchProgressResetCommandID, searchOverlaySetCommandID, commandTestSearchCommandID].includes(commandID)) return searchStatusCommandID
  if ([questItemEnsureCommandID, questItemRemoveCommandID, questShardUseCommandID, usableInventoryPresetCommandID].includes(commandID)) return questItemStatusCommandID
  if (commandID === worldPositionSetCommandID || commandID === movementLandmarkTravelCommandID) return worldPositionStatusCommandID
  if (testSetupMutationCommandIDs.includes(commandID)) return testSetupStatusCommandID
  if (saveSnapshotCommandIDs.includes(commandID)) return saveSnapshotListCommandID
  if (observationCommandIDs.includes(commandID as ObservationCommandID)) return observationCommandID
  if ([moongateOverlaySetCommandID, moongateTravelCommandID].includes(commandID)) return moongateStatusCommandID
  if (commandID === combatEncounterStartCommandID) return combatEncounterOptionsCommandID
  if (merchantTesterOperationCommandIDs.includes(commandID)) return merchantTesterCatalogCommandID
  if ([monsterGenerationSetCommandID, monsterClearMapEnemiesCommandID, monsterSpawnWhirlpoolCommandID, monsterSpawnPirateCommandID].includes(commandID)) return monsterEcosystemStatusCommandID
  if (shipMutationCommandIDs.includes(commandID)) return vehicleStatusCommandID
  if (debugOptionsMutationCommandIDs.includes(commandID)) return debugOptionsStatusCommandID
  if (worldDiagnosticCommandIDs.includes(commandID)) return guardStatusCommandID
  if (inventoryLockerMutationCommandIDs.includes(commandID)) return inventoryLockerStatusCommandID
  if (spellInventoryMutationCommandIDs.includes(commandID)) return spellInventoryStatusCommandID
  if (activeCombatMutationCommandIDs.includes(commandID)) return activeCombatStatusCommandID
  return commandID
}

function navigationSurfaceCommands(owner: CommandDefinition): CommandDefinition[] {
  return commands.value.filter((candidate) => companionOwnerCommandID(candidate.id) === owner.id)
}

function openCommandPalette(): void {
  if (commandPaletteOpen.value) return
  commandPalettePreviousFocus = document.activeElement as HTMLElement | null
  commandPaletteOpen.value = true
  void nextTick(() => setWorkbenchBackgroundInert(true, ".command-palette-modal"))
}

function closeCommandPalette(): void {
  commandPaletteOpen.value = false
  setWorkbenchBackgroundInert(false, ".command-palette-modal")
  void nextTick(() => (commandPalettePreviousFocus ?? document.querySelector<HTMLElement>("[data-command-palette-launcher]"))?.focus())
  commandPalettePreviousFocus = null
}

function navigateToSurface(surface: NavigationSurface): void {
  navigationRecents.value = updateNavigationRecents(navigationRecents.value, surface.id)
  localStorage.setItem(navigationRecentsStorageKey, JSON.stringify(navigationRecents.value))
  activeWorkspace.value = surface.workspace
  if (surface.commandId === merchantTesterCatalogCommandID) merchantCatalogExplicitRefreshRequired = false
  selectedCommandID.value = surface.commandId
  void nextTick(autoExecuteSelectedQuery)
}

function activateNavigationSurface(surface: NavigationSurface): void {
  navigateToSurface(surface)
  closeCommandPalette()
}

function instrumentModeLabel(command: CommandDefinition): string {
  if (command.id === observationCommandID) return "GAME-WINDOW OBSERVATION WORKFLOWS"
  if (command.id === shrineStatusCommandID) return "LIVE STATE + EXPLICIT ALTERATION"
  if (command.id === combatEncounterOptionsCommandID) return "SYNCHRONIZED COMPOSER"
	if (command.id === merchantTesterCatalogCommandID) return "QUERY + PREVIEW + DESTRUCTIVE LIVE"
	if (command.id === worldPositionStatusCommandID) return "POSITION SNAPSHOT + EXPLICIT CONTROL"
  if (command.id === windStatusCommandID || command.id === calendarStatusCommandID || command.id === searchStatusCommandID
    || command.id === questItemStatusCommandID) return "LIVE TELEMETRY + EXPLICIT CONTROL"
  if (command.id === moongateStatusCommandID) return "LIVE TELEMETRY + ROUTING REFERENCE"
	if (command.id === monsterEcosystemStatusCommandID || command.id === vehicleStatusCommandID || command.id === debugOptionsStatusCommandID) return "LIVE TELEMETRY + EXPLICIT CONTROL"
  if (command.id === activeCombatStatusCommandID) return "LIVE TACTICAL TELEMETRY + REVIEWED CONTROL"
	if (command.id === partyCharacterCommandID) return "ACTIVE CHARACTER + PREVIEWED ALTERATION"
	if (command.id === partyEquipmentLoadoutCommandID) return "ATOMIC SIX-SLOT LOADOUT REVIEW"
	if (command.id === partyCharacterInspectCommandID) return "LIVE ROSTER + REVIEWED PARTY CONTROL"
	if (command.id === guardStatusCommandID) return "HIDDEN AUTHORITATIVE STATE"
  if (command.id === inventoryLockerStatusCommandID) return "LIVE INVENTORY + REVIEWED ALTERATIONS"
  if (command.id === spellInventoryStatusCommandID) return "LIVE INVENTORY + REVIEWED ALTERATIONS"
  return isMutation(command) ? "LIVE-SAFE MUTATION" : "READ-ONLY QUERY"
}

function instrumentExecutionModes(command: CommandDefinition): string {
	if (command.id !== merchantTesterCatalogCommandID) return command.executionModes.join(" · ")
	return navigationSurfaceCommands(command)
		.map((candidate) => `${candidate.id === merchantTesterCatalogCommandID ? "catalog" : candidate.id === merchantTesterPreviewCommandID ? "preview" : "live"}: ${candidate.executionModes.join(" / ")}`)
		.join(" · ")
}

function instrumentOwnsMutation(command: CommandDefinition): boolean {
  return isMutation(command) || command.id === windStatusCommandID || command.id === calendarStatusCommandID
    || command.id === searchStatusCommandID || command.id === questItemStatusCommandID || command.id === worldPositionStatusCommandID
     || command.id === combatEncounterOptionsCommandID || command.id === monsterEcosystemStatusCommandID
     || command.id === vehicleStatusCommandID || command.id === debugOptionsStatusCommandID
     || command.id === inventoryLockerStatusCommandID
     || command.id === spellInventoryStatusCommandID
     || command.id === activeCombatStatusCommandID
	 || command.id === partyCharacterInspectCommandID
	 || command.id === testSetupStatusCommandID
     || command.id === merchantTesterCatalogCommandID
    || command.id === shrineStatusCommandID
}

function instrumentWidthClass(command: CommandDefinition): string {
  if (command.id === windStatusCommandID || command.id === calendarStatusCommandID) return "instrument--narrow"
  if (command.id === partyCharacterCommandID || command.id === partyCharacterInspectCommandID || command.id === shrineStatusCommandID
    || command.id === searchStatusCommandID || command.id === questItemStatusCommandID || command.id === worldPositionStatusCommandID) return "instrument--editor"
  if (command.id === combatEncounterOptionsCommandID
	 || command.id === monsterEcosystemStatusCommandID || command.id === vehicleStatusCommandID || command.id === debugOptionsStatusCommandID) return "instrument--editor"
	if (command.id === guardStatusCommandID) return "instrument--reference"
  if (command.id === inventoryLockerStatusCommandID || command.id === spellInventoryStatusCommandID) return "instrument--reference"
	if (command.id === partyEquipmentLoadoutCommandID) return "instrument--reference"
  if (command.id === activeCombatStatusCommandID) return "instrument--reference"
	if (command.id === merchantTesterCatalogCommandID) return "instrument--reference"
	if (command.id === testSetupStatusCommandID) return "instrument--wide"
	if (command.id === saveSnapshotListCommandID) return "instrument--wide"
  if (command.id === observationCommandID) return "instrument--editor"
  return "instrument--form"
}

function runQueryByID(commandID: string, silent: boolean): void {
  const command = supportedCommands.value.find((candidate) => candidate.id === commandID)
  if (command) runCommand(command, silent)
}

function autoExecuteSelectedQuery(): void {
  const command = selectedCommand.value
  if (command && activePage.value?.anchor === command.id) {
    runPageQueries(activePage.value)
    return
  }
	if (!command) return
	if (command.id === merchantTesterCatalogCommandID
		&& (merchantCatalogExplicitRefreshRequired || instrumentRequestPending(merchantTesterCatalogCommandID))) return
	if (command.id === saveSnapshotListCommandID) {
		runQueryByID(saveSnapshotListCommandID, true)
		runQueryByID(testSetupStatusCommandID, true)
		return
	}
	if (command.id === partyCharacterCommandID) return
	if (isMutation(command)) return
	if (command.id === guardStatusCommandID) {
		for (const commandID of worldDiagnosticCommandIDs) runQueryByID(commandID, true)
		return
	}
  if (!integratedCommandIDs.has(command.id) || command.id === observationCommandID) return
if (command.id === combatEncounterOptionsCommandID && latestResults.value[command.id]) return
  runCommand(command, true)
}

function runSpecializedMutation(commandID: string, argumentsByID: Record<string, string>, onSent?: (requestID: string) => void): boolean {
  const command = supportedCommands.value.find((candidate) => candidate.id === commandID)
  const owner = pageOwnerFor(commandID) ?? companionOwnerCommandID(commandID)
  delete ownerErrors.value[owner]
  const unavailable = !isReady.value ? "Reconnect to the game before running this operation."
    : !command ? "This operation is not available in the current catalog."
    : commandUnavailableReason(command) || (commandPending(commandID) ? "Wait for the pending operation to finish." : "")
  if (unavailable || !command) {
    ownerErrors.value[owner] = unavailable
    return false
  }
  drafts.value[command.id] = argumentsByID
  if (command.id === partyCharacterCommandID) dirtyFields.value[command.id] = Object.keys(argumentsByID).filter((key) => key !== "character")
  const sent = runCommand(command, false, (requestID) => {
    const request = pending.value.get(requestID)
    if (request) request.ownerCommandID = owner
    onSent?.(requestID)
  })
  if (!sent) ownerErrors.value[owner] = Object.entries(formErrors.value[commandID] ?? {}).map(([field, error]) => `${field}: ${error}`).join(" ")
    || formMessages.value[commandID] || "The operation could not be sent. Check the connection and catalog constraints."
  return sent
}

function quickExitCombat(): void {
	const combat = currentCombat.value
	if (!combat?.active || !combat.encounterId || !combat.snapshotFingerprint) return
	runSpecializedMutation(activeCombatExitCommandID, {
		encounterId: combat.encounterId,
		expectedSnapshot: combat.snapshotFingerprint,
	})
}

// Quick actions only call commands whose own page runs them without a review step; everything else just opens its page.
function homeQuickActions(task: TaskID): HomeQuickAction[] {
  const ready = isReady.value
  switch (task) {
    case "goto": {
      if (!commands.value.some((command) => command.id === movementLandmarkTravelCommandID)) return []
      const preferred = preferredLandmarks.flatMap((value) => worldLandmarkChoices.value.filter((choice) => choice.value === value))
      return (preferred.length > 0 ? preferred : worldLandmarkChoices.value.slice(0, 3)).map((choice) => ({
        id: `landmark:${choice.value}`,
        label: choice.label,
        description: `Travel to ${choice.label}`,
        effect: { kind: "landmark", landmark: choice.value },
        disabled: !ready || commandPending(movementLandmarkTravelCommandID),
      }))
    }
    case "fight": {
      const actions: HomeQuickAction[] = []
      if (currentCombat.value?.active) actions.push({ id: "exit-combat", label: "Escape combat", description: "Discard the active debug encounter and return to its pre-combat location", effect: { kind: "exit-combat" }, disabled: quickCombatExitDisabled.value })
      return actions
    }
    case "party":
      if (!commands.value.some((command) => command.id === partyRestoreCommandID)) return []
      return [{ id: "party-restore", label: "Heal and cure everyone", description: "Restore every active member to full HP and MP with a good condition", effect: { kind: "party-restore" }, disabled: !ready || commandPending(partyRestoreCommandID) || Boolean(currentCombat.value?.active) }]
    case "time":
      if (!commands.value.some((command) => command.id === calendarSetTimeCommandID)) return []
      return timePresets.map((preset) => ({
        id: `time:${preset.hour}:${preset.minute}`,
        label: preset.label,
        description: `Set the time of day to ${String(preset.hour).padStart(2, "0")}:${String(preset.minute).padStart(2, "0")}`,
        effect: { kind: "time", hour: preset.hour, minute: preset.minute },
        disabled: !ready || commandPending(calendarSetTimeCommandID),
      }))
    case "items":
    case "checkpoints":
    case "more":
      return []
  }
}

function runHomeQuickAction(action: HomeQuickAction): void {
  const effect = action.effect
  if (effect.kind === "open") {
    const surface = navigationSurfaces.value.find((candidate) => candidate.commandId === effect.commandId)
    if (surface) navigateToSurface(surface)
    return
  }
  homeQuickState.value = undefined
  if (effect.kind === "exit-combat") {
    quickExitCombat()
    return
  }
  if (effect.kind === "party-restore") {
    homeRestorePending.value = true
    homeQuickState.value = { requestID: undefined }
    runQueryByID(partyCharacterInspectCommandID, true)
    return
  }
  const commandID = effect.kind === "landmark" ? movementLandmarkTravelCommandID : calendarSetTimeCommandID
  const argumentsByID: Record<string, string> = effect.kind === "landmark" ? { landmark: effect.landmark } : { hour: String(effect.hour), minute: String(effect.minute) }
  const sent = runSpecializedMutation(commandID, argumentsByID, (requestID) => { homeQuickState.value = { requestID } })
  if (!sent) homeQuickState.value = { error: ownerErrors.value[companionOwnerCommandID(commandID)] || "The action could not be sent." }
}

function setWorkbenchBackgroundInert(inert: boolean, modalSelector: string): void {
  for (const element of document.querySelectorAll<HTMLElement>(".workbench-shell > *")) {
    if (!element.matches(modalSelector)) element.inert = inert
  }
}

function inspectSaveSnapshot(reference: string): void {
  runSpecializedMutation(saveSnapshotGetCommandID, { reference })
}

function saveSnapshotListData(value: unknown): SaveSnapshotListData { return value as SaveSnapshotListData }
function saveSnapshotDetailData(value: unknown) { return (value as SaveSnapshotGetData).snapshot }

function setMonsterGeneration(value: { enabled: boolean; odds: number }): void {
  runSpecializedMutation(monsterGenerationSetCommandID, { enabled: String(value.enabled), odds: String(value.odds) })
}

function setCalendarTime(value: { hour: number; minute: number }): void {
  runSpecializedMutation(calendarSetTimeCommandID, { hour: String(value.hour), minute: String(value.minute) })
}

function setCalendarDay(value: { day: number }): void {
  runSpecializedMutation(calendarSetDayCommandID, { day: String(value.day) })
}

function addSearchItemStack(value: { x: number; y: number; item: string; quantity: number }): void {
  runSpecializedMutation(searchItemStackAddCommandID, {
    x: String(value.x),
    y: String(value.y),
    item: value.item,
    quantity: String(value.quantity),
  })
}

function resetSearchProgress(): void {
  runSpecializedMutation(searchProgressResetCommandID, { scope: "one_time_locations" })
}

function applyWorldPosition(value: WorldPositionApply): void {
  const argumentsByID: Record<string, string> = {
    placement: value.placement,
    location: String(value.locationId),
    expectedLocationId: String(value.expectedLocationId),
    expectedFloor: String(value.expectedFloor),
    expectedX: String(value.expectedX),
    expectedY: String(value.expectedY),
    expectedSourceFingerprint: value.expectedSourceFingerprint,
  }
  if (value.route !== undefined) argumentsByID.route = value.route
  if (value.floor !== undefined) argumentsByID.floor = String(value.floor)
  if (value.x !== undefined) argumentsByID.x = String(value.x)
  if (value.y !== undefined) argumentsByID.y = String(value.y)
  if (value.room !== undefined) argumentsByID.room = String(value.room)
  runPageSteps([{ commandID: worldPositionSetCommandID, argumentsByID }])
}

function commandUsesDynamicOptions(command: CommandDefinition): boolean {
  return (command.parameters ?? []).some((parameter) => Boolean(parameter.optionsProvider))
}

function isMutation(command: CommandDefinition): boolean {
  return command.executionModes.some((mode) => mode !== "query")
}

function commandUnavailableReason(command: CommandDefinition): string {
  if (awaitingOptionsRefresh.value && commandUsesDynamicOptions(command)) return "Refreshing dynamic options before another operation can run."
  for (const parameter of command.parameters ?? []) {
    if (!parameter.optionsProvider) continue
    if (optionSets.value[parameter.optionsProvider] === undefined) return "Dynamic options are unavailable in the active scene."
    if (optionUnavailableReasons.value[parameter.optionsProvider]) return optionUnavailableReasons.value[parameter.optionsProvider]
  }

  return ""
}

function commandApplyDisabled(command: CommandDefinition): boolean {
  if (!isReady.value || commandPending(command.id) || hasUnsupportedParameters(command, optionSets.value) || commandUnavailableReason(command)) return true
  if (command.id === partyCharacterCommandID) {
    return partyAlterationPreview(command, optionSets.value, draftFor(command), dirtyFieldsFor(command)).length === 0
  }

  return false
}

function clearCharacterAlterationErrors(commandID: string): void {
  const errors = { ...formErrors.value[commandID] }
  for (const fieldID of partyCharacterFieldIDs) delete errors[fieldID]
  formErrors.value[commandID] = errors
  delete formMessages.value[commandID]
}

function receiveProtocolError(message: Envelope): void {
  const payload = message.payload as { code?: string; message?: string }
  const pendingRequest = message.id ? pending.value.get(message.id) : undefined
  if (!pendingRequest && selectedCommandID.value) ownerErrors.value[selectedCommandID.value] = `${payload.code ?? "protocol_error"}: ${payload.message ?? "The connection received an invalid protocol message."}`
  if (message.id) {
    pending.value.delete(message.id)
    silentRequestIDs.delete(message.id)
  }
  appendResult({
    entryID: `protocol-${++localEntrySequence}`,
    sessionId: sessionID.value,
    requestId: message.id ?? "protocol",
    commandId: pendingRequest?.commandID ?? "protocol",
    ownerCommandID: pendingRequest?.ownerCommandID,
    sceneGeneration: sceneGeneration.value,
    success: false,
    errorCode: payload.code ?? "protocol_error",
    errorMessage: payload.message ?? "Unknown protocol error",
    receivedAt: new Date(),
  })
}

function reportLostPendingRequests(): void {
  connectionLost()
  for (const [requestID, request] of pending.value) {
    silentRequestIDs.delete(requestID)
    rememberProcessedRequest(requestID)
    appendResult({
      entryID: `lost-${++localEntrySequence}`,
      sessionId: sessionID.value,
      requestId: requestID,
      commandId: request.commandID,
      ownerCommandID: request.ownerCommandID,
      sceneGeneration: sceneGeneration.value,
      success: false,
      errorCode: "session_lost",
      errorMessage: "The previous session was not resumed; this command's outcome is unknown.",
      receivedAt: new Date(),
    })
  }
}

function appendResult(result: ResultEntry): void {
  results.value.push(result)
  latestResults.value = { ...latestResults.value, [result.commandId]: result }
  const changes = result.changes?.length ? ` ${result.changes.length} changes applied.` : ""
  liveStatus.value = `${commandName(result.commandId)} ${result.success ? "completed" : "failed"}.${changes}`
  if (results.value.length > resultLimit) {
    results.value.shift()
    trimmedResults.value += 1
  }
  if (followResults.value) void nextTick(scrollToLatest)
}

function rememberProcessedRequest(requestID: string): void {
  processedRequestIDs.add(requestID)
  processedRequestOrder.push(requestID)
  if (processedRequestOrder.length > resultLimit * 2) {
    const removed = processedRequestOrder.shift()
    if (removed) processedRequestIDs.delete(removed)
  }
}

function commandPending(commandID: string): boolean {
  return [...pending.value.values()].some((request) => request.commandID === commandID)
}

function instrumentRequestPending(ownerCommandID: string): boolean {
  return [...pending.value.values()].some((request) => companionOwnerCommandID(request.commandID) === ownerCommandID)
}

function commandPendingLabel(commandID: string): string {
  const request = [...pending.value.values()].find((candidate) => candidate.commandID === commandID)
  if (request && now.value - request.lastConfirmedAt >= operationStallMs) return "STALLED · RECONCILING"
  return request?.phase === "accepted" ? "ACCEPTED..." : "SENDING..."
}

function checkPendingOperations(): void {
  now.value = Date.now()
  for (const [requestID, request] of pending.value) {
    const unconfirmedFor = now.value - request.lastConfirmedAt
    if (unconfirmedFor >= operationReconnectMs) {
      socket.value?.close(1011, "operation status timeout")
      return
    }
    if (unconfirmedFor >= operationStallMs && !request.statusRequested) {
      const sent = send(envelope(requestID, "operation.status", { requestId: requestID }))
      if (sent) pending.value.set(requestID, { ...request, statusRequested: true })
    }
  }
	if (!isReady.value || document.hidden) return
  if (selectedCommand.value && commandUsesDynamicOptions(selectedCommand.value)) requestCatalog()
		for (const globalID of globalStatusCommandIDs) runQueryByID(globalID, true)
		if (activePage.value) {
			if (batch.value || pageMutationPending.value) return
			if (activeWorkspace.value === "party") requestCatalog()
			runPageQueries(activePage.value)
			return
		}
		if (!selectedCommand.value || globalStatusCommandIDs.includes(selectedCommand.value.id)) return
	if (instrumentRequestPending(selectedCommand.value.id)) return
	if (!polledMoreQueryIDs.has(selectedCommand.value.id)) return
  runCommand(selectedCommand.value, true)
}

function commandName(commandID: string): string {
  return commands.value.find((command) => command.id === commandID)?.name ?? commandID
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString([], { hour12: false })
}

function toggleHistory(): void {
  historyCollapsed.value = !historyCollapsed.value
}

function toggleHistoryDock(): void {
  stopHistoryResize?.()
  historyDock.value = historyDock.value === "bottom" ? "right" : "bottom"
}

function startHistoryResize(event: PointerEvent): void {
  if (historyCollapsed.value) return
  stopHistoryResize?.()
  const startX = event.clientX
  const startY = event.clientY
  const bounds = historyPanel.value?.getBoundingClientRect()
  const measuredWidth = bounds?.width ?? 0
  const measuredHeight = bounds?.height ?? 0
  const startWidth = measuredWidth > 0 ? measuredWidth : historyWidth.value
  const startHeight = measuredHeight > 0 ? measuredHeight : historyHeight.value
  const move = (moveEvent: PointerEvent) => {
    if (historyDock.value === "right") {
      const maximum = Math.max(360, Math.floor(window.innerWidth * 0.55))
      historyWidth.value = Math.min(maximum, Math.max(300, startWidth + startX - moveEvent.clientX))
    } else {
      const maximum = Math.max(160, Math.floor(window.innerHeight * 0.65))
      historyHeight.value = Math.min(maximum, Math.max(120, startHeight + startY - moveEvent.clientY))
    }
  }
  const stop = () => {
    window.removeEventListener("pointermove", move)
    window.removeEventListener("pointerup", stop)
    stopHistoryResize = undefined
  }
  stopHistoryResize = stop
  window.addEventListener("pointermove", move)
  window.addEventListener("pointerup", stop)
  event.preventDefault()
}

function shrineStatusData(value: unknown): ShrineStatusData {
  return value as ShrineStatusData
}

function calendarStatusData(value: unknown): CalendarStatusData {
  return value as CalendarStatusData
}

function loggingStatusData(value: unknown): LoggingStatusData {
  return value as LoggingStatusData
}

function searchStatusData(value: unknown): SearchStatusData {
  return value as SearchStatusData
}

function questItemStatusData(value: unknown): QuestItemStatusData {
  return value as QuestItemStatusData
}

function worldPositionStatusData(value: unknown): WorldPositionStatusData {
  return value as WorldPositionStatusData
}

function moongateStatusData(value: unknown): MoongateStatusData {
  return value as MoongateStatusData
}

function combatEncounterOptionsData(value: unknown): CombatEncounterOptionsData {
  return value as CombatEncounterOptionsData
}

function merchantTesterCatalogData(value: unknown): MerchantTesterCatalogData {
  return value as MerchantTesterCatalogData
}

function monsterEcosystemData(value: unknown): MonsterEcosystemData {
  return value as MonsterEcosystemData
}

function vehicleStatusData(value: unknown): VehicleStatusData {
  return value as VehicleStatusData
}

function debugOptionsStatusData(value: unknown): DebugOptionsStatusData {
  return value as DebugOptionsStatusData
}

function guardStatusData(value: unknown): GuardStatusData {
  return value as GuardStatusData
}

function shadowlordStatusData(value: unknown): ShadowlordStatusData {
  return value as ShadowlordStatusData
}

function deathStatusData(value: unknown): DeathStatusData {
  return value as DeathStatusData
}

function inventoryLockerStatusData(value: unknown): InventoryLockerStatusData {
  return value as InventoryLockerStatusData
}

function spellInventoryStatusData(value: unknown): SpellInventoryStatusData {
  return value as SpellInventoryStatusData
}

function activeCombatStatusData(value: unknown): ActiveCombatStatusData {
  return value as ActiveCombatStatusData
}

function observationResultData(value: unknown): ObservationResultData {
  return value as ObservationResultData
}

function windDirection(result: ResultEntry | undefined): string | undefined {
  return (result?.data as { direction?: string } | undefined)?.direction
}

function formatChangeValue(value: unknown): string {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean" ? String(value) : JSON.stringify(value)
}

function clearResults(): void {
  results.value = []
  trimmedResults.value = 0
}

function onStreamScroll(): void {
  if (!stream.value) return
  const distanceFromBottom = stream.value.scrollHeight - stream.value.scrollTop - stream.value.clientHeight
  followResults.value = distanceFromBottom < 24
}

function toggleFollow(): void {
  followResults.value = !followResults.value
  if (followResults.value) void nextTick(scrollToLatest)
}

function scrollToLatest(): void {
  if (!stream.value) return
  if (typeof stream.value.scrollTo === "function") {
    stream.value.scrollTo({ top: stream.value.scrollHeight })
  } else {
    stream.value.scrollTop = stream.value.scrollHeight
  }
}

function inputValue(event: Event): string {
  return (event.target as HTMLInputElement | HTMLSelectElement).value
}

function onKeyDown(event: KeyboardEvent): void {
  const activeElement = document.activeElement as HTMLElement | null
  if ((event.ctrlKey || event.metaKey) && event.key.toLocaleLowerCase() === "k") {
    event.preventDefault()
    openCommandPalette()
    return
  }
  if (commandPaletteOpen.value) {
    if (event.key === "F5") event.preventDefault()
    return
  }
  if (event.ctrlKey && event.key === "`") {
    event.preventDefault()
    stream.value?.focus()
    return
  }
  if (["ArrowDown", "ArrowUp", "ArrowRight", "ArrowLeft"].includes(event.key) && activeElement?.dataset.commandSelect) {
    const buttons = availableCommands.value
      .map((command) => document.querySelector<HTMLButtonElement>(`button[data-command-select="${command.id}"]`))
      .filter((button): button is HTMLButtonElement => button !== null && !button.disabled)
    if (buttons.length === 0) return
    const currentIndex = buttons.indexOf(activeElement as HTMLButtonElement)
    const offset = event.key === "ArrowDown" || event.key === "ArrowRight" ? 1 : -1
    const nextButton = buttons[(currentIndex + offset + buttons.length) % buttons.length]
    nextButton?.focus()
    const command = availableCommands.value.find((candidate) => candidate.id === nextButton?.dataset.commandSelect)
    if (command) selectCommand(command)
    event.preventDefault()
    return
  }
  if (event.key !== "F5" || event.ctrlKey || event.altKey || event.metaKey || !selectedCommand.value) return
  event.preventDefault()
  if (selectedCommand.value.id === observationCommandID) return
  if (activePage.value) { autoExecuteSelectedQuery(); return }
  if (event.shiftKey) {
    formMessages.value[selectedCommand.value.id] = "Staging is not available for this instrument."
    return
  }
  if (selectedCommand.value.id === merchantTesterCatalogCommandID) merchantCatalogExplicitRefreshRequired = false
  if (isMutation(selectedCommand.value)) {
    ownerErrors.value[selectedCommand.value.id] = "F5 only refreshes telemetry. Use the explicit operation controls to stage or apply changes."
    return
  }
  runCommand(selectedCommand.value)
}

function updatePageVisibility(): void {
  pageHidden.value = document.hidden
}

onMounted(() => {
  operationTimer = window.setInterval(checkPendingOperations, 1000)
  window.addEventListener("keydown", onKeyDown)
  document.addEventListener("visibilitychange", updatePageVisibility)
  connect()
})
onBeforeUnmount(() => {
  setWorkbenchBackgroundInert(false, ".command-palette-modal")
  connectionGeneration += 1
  window.clearTimeout(reconnectTimer)
  window.clearTimeout(connectionTimer)
  window.clearInterval(operationTimer)
  stopHistoryResize?.()
  window.removeEventListener("keydown", onKeyDown)
  document.removeEventListener("visibilitychange", updatePageVisibility)
  socket.value?.close()
})
</script>

<template>
  <main class="workbench-shell" :class="`history-dock-${historyDock}`">
    <WorkbenchTopBar
      :connection-state="connectionState"
      :connection-label="connectionLabel"
      :scene-generation="sceneGeneration"
      :close-detail="lastCloseReason ? `${lastCloseReason} · ${retryDetail}` : ''"
      :where="topBarWhere"
      :clock="topBarClock"
      :party="topBarParty"
      :mode="topBarMode"
      :tasks="taskDefinitions"
      :active-view="activeWorkspace"
      :exit-combat-disabled="quickCombatExitDisabled"
      @open-palette="openCommandPalette"
      @select-view="selectWorkspace"
      @exit-combat="quickExitCombat"
      @retry="retryNow"
    />
    <p class="sr-only" aria-live="polite">{{ liveStatus }}</p>

    <section class="switchboard" aria-label="Workbench tasks">
      <WorkbenchHome
        v-if="activeWorkspace === 'home'"
        :cards="homeCards"
        :recents="homeRecentSurfaces"
        :feedback="homeFeedback"
        @open-task="selectWorkspace"
        @open-surface="navigateToSurface"
        @quick="runHomeQuickAction"
      />
      <div v-else-if="activePage" class="task-page" :data-task="activeWorkspace">
        <section class="instrument-bank" :aria-label="taskDefinition(activeWorkspace).label">
          <section v-if="ownerErrors[activePage.anchor]" class="operation-banner failure" role="alert" data-owner-error>{{ ownerErrors[activePage.anchor] }}</section>
          <section v-else-if="latestOperationResult" class="operation-banner" :class="latestOperationResult.success ? 'success' : 'failure'" aria-live="polite" data-last-operation>
            <header><strong>LAST OPERATION</strong><span>{{ commandName(latestOperationResult.commandId) }} · {{ formatTime(latestOperationResult.receivedAt) }}</span></header>
            <p>{{ latestOperationResult.success ? latestOperationResult.summary : `${latestOperationResult.errorCode} :: ${latestOperationResult.errorMessage}` }}</p>
          </section>
          <p v-if="batchNotice" class="pg-status" :class="batchNotice.success ? 'success' : 'failure'" role="status" data-page-notice>{{ batchNotice.text }}</p>
          <p v-if="!isReady" class="disabled-reason">EXECUTION DISABLED UNTIL CONNECTION IS READY</p>
          <GoToPage
            v-if="activeWorkspace === 'goto'"
            :key="`${sessionID}:${sceneGeneration}`"
            :data="lastSuccessfulWorldPositionResult?.data ? worldPositionStatusData(lastSuccessfulWorldPositionResult.data) : undefined"
            :landmarks="worldLandmarkChoices"
            :pending="commandPending(worldPositionSetCommandID) || batch !== undefined"
            :pending-landmark="commandPending(movementLandmarkTravelCommandID)"
            :refreshing="commandPending(worldPositionStatusCommandID)"
            :stale="Boolean(lastSuccessfulWorldPositionResult && latestWorldPositionResult?.success === false)"
            :free-move="freeMoveEnabled"
            :free-move-pending="commandPending(movementFreeMoveSetCommandID)"
            :can-free-move="commands.some((command) => command.id === movementFreeMoveSetCommandID)"
            @refresh="runQueryByID(worldPositionStatusCommandID, false)"
            @apply="applyWorldPosition"
            @travel-landmark="(landmark) => runPageSteps([{ commandID: movementLandmarkTravelCommandID, argumentsByID: { landmark } }])"
            @free-move="(enabled) => runPageSteps([{ commandID: movementFreeMoveSetCommandID, argumentsByID: { enabled: String(enabled) } }])"
            @fight-room="openFightForRoom"
          />
          <PartyPage
            v-else-if="activeWorkspace === 'party'"
            :roster="latestPartyCharacterInspectResult?.data as PartyCharacterInspectData | undefined"
            :locker="latestInventoryLockerResult?.data ? inventoryLockerStatusData(latestInventoryLockerResult.data) : undefined"
            :option-sets="optionSets"
            :commands="commands"
            :busy="pageBusy"
            @apply="runPageSteps"
          />
          <ItemsPage
            v-else-if="activeWorkspace === 'items'"
            :locker="latestInventoryLockerResult?.data ? inventoryLockerStatusData(latestInventoryLockerResult.data) : undefined"
            :spells="latestSpellInventoryResult?.data ? spellInventoryStatusData(latestSpellInventoryResult.data) : undefined"
            :quest="latestQuestItemResult?.data ? questItemStatusData(latestQuestItemResult.data) : undefined"
            :commands="commands"
            :busy="pageBusy"
            @apply="runPageSteps"
          />
          <TimePage
            v-else-if="activeWorkspace === 'time'"
            :calendar="currentCalendar"
            :wind-direction="windDirection(latestResults[windStatusCommandID])"
            :moongate="latestMoongateResult?.data ? moongateStatusData(latestMoongateResult.data) : undefined"
            :busy="pageBusy"
            :travel-pending="commandPending(moongateTravelCommandID)"
            :overlay-pending="commandPending(moongateOverlaySetCommandID)"
            @apply="runPageSteps"
            @travel="(step) => runPageSteps([step])"
            @overlay="(enabled) => runPageSteps([{ commandID: moongateOverlaySetCommandID, argumentsByID: { enabled: String(enabled) } }])"
          />
          <FightPage
            v-else-if="activeWorkspace === 'fight'"
            :combat="currentCombat"
            :options="latestCombatEncounterOptionsResult?.data ? combatEncounterOptionsData(latestCombatEncounterOptionsResult.data) : undefined"
            :commands="commands"
            :busy="pageBusy"
            :start-pending="commandPending(combatEncounterStartCommandID)"
            :preselect="fightPreselect"
            @apply="runPageSteps"
          />
          <SaveLibrary
            v-else-if="activeWorkspace === 'checkpoints'"
            :data="saveList"
            :detail="latestSaveDetailResult?.data ? saveSnapshotDetailData(latestSaveDetailResult.data) : undefined"
            :pending-list="commandPending(saveSnapshotListCommandID)"
            :pending-detail="commandPending(saveSnapshotGetCommandID)"
            :pending-mutation="pageBusy"
            :current-scene="latestTestSetupStatusResult?.data ? {
              locationName: (latestTestSetupStatusResult.data as TestSetupStatusData).locationName,
              floor: (latestTestSetupStatusResult.data as TestSetupStatusData).floor,
              date: (latestTestSetupStatusResult.data as TestSetupStatusData).date,
              time: (latestTestSetupStatusResult.data as TestSetupStatusData).time,
              partyMembers: (latestTestSetupStatusResult.data as TestSetupStatusData).party.length,
            } : undefined"
            @refresh="runQueryByID(saveSnapshotListCommandID, false)"
            @inspect="inspectSaveSnapshot"
            @capture="(value) => runPageSteps([{ commandID: saveSnapshotCaptureCommandID, argumentsByID: value }])"
            @load="(value) => runPageSteps([{ commandID: saveSnapshotLoadCommandID, argumentsByID: value }])"
            @delete="(value) => runPageSteps([{ commandID: saveSnapshotDeleteCommandID, argumentsByID: value }])"
          />
        </section>
      </div>
      <div v-else class="task-page" :data-task="activeWorkspace">
        <header class="task-header">
          <div><h1>{{ taskDefinition(activeWorkspace).label }}</h1><p>{{ taskDefinition(activeWorkspace).blurb }}</p></div>
        </header>
        <nav v-if="availableCommands.length > 1" class="tool-picker" :aria-label="`${taskDefinition(activeWorkspace).label} tools`">
          <section v-for="group in toolGroups" :key="group.name ?? 'tools'" class="tool-group">
            <h2 v-if="group.name">{{ group.name }}</h2>
            <div class="tool-buttons">
              <button
                v-for="command in group.commands"
                :key="command.id"
                type="button"
                class="tool-option"
                :data-command-select="command.id"
                :class="{ selected: command.id === selectedCommandID }"
                :aria-current="command.id === selectedCommandID ? 'true' : undefined"
                @click="selectCommand(command)"
                @keydown.enter.prevent="activateCommand(command)"
              >{{ toolName(command) }}</button>
            </div>
          </section>
        </nav>
        <p v-if="availableCommands.length === 0" class="empty-state">
          {{ isReady ? "CATALOG REQUESTED" : "WAITING FOR CONTROL PLANE" }}
        </p>

        <section class="instrument-bank" aria-labelledby="instrument-title">
          <div v-if="activeWorkspace !== 'checkpoints'" class="section-heading">
            <span aria-hidden="true">▸</span>
            <h2 id="instrument-title">Active Instrument</h2>
            <small>{{ selectedCommand?.category ?? "STANDBY" }}</small>
          </div>

        <article
          v-if="selectedCommand"
          class="instrument"
          :class="instrumentWidthClass(selectedCommand)"
        >
          <div v-if="activeWorkspace !== 'checkpoints'" class="instrument-index">{{ selectedCommand.id }} / {{ instrumentExecutionModes(selectedCommand) }}</div>
          <div v-if="activeWorkspace !== 'checkpoints'" class="instrument-title-row">
            <h3>{{ toolName(selectedCommand) }}</h3>
            <span class="execution-mode" :class="{ mutation: instrumentOwnsMutation(selectedCommand) }">
              {{ instrumentModeLabel(selectedCommand) }}
            </span>
          </div>
          <p v-if="activeWorkspace !== 'checkpoints'">{{ toolDescription(selectedCommand) }}</p>
          <code v-if="activeWorkspace !== 'checkpoints'">{{ selectedCommand.aliases?.join(" · ") }}</code>
          <section v-if="ownerErrors[selectedCommand.id]" class="operation-banner failure" role="alert" data-owner-error>{{ ownerErrors[selectedCommand.id] }}</section>
          <section v-else-if="latestOperationResult" class="operation-banner" :class="latestOperationResult.success ? 'success' : 'failure'" aria-live="polite">
            <header>
              <strong>LAST OPERATION</strong>
              <span>{{ commandName(latestOperationResult.commandId) }} · {{ formatTime(latestOperationResult.receivedAt) }}</span>
            </header>
            <p>{{ latestOperationResult.success ? latestOperationResult.summary : `${latestOperationResult.errorCode} :: ${latestOperationResult.errorMessage}` }}</p>
            <ul v-if="latestOperationResult.changes?.length">
              <li v-for="change in latestOperationResult.changes.slice(0, 3)" :key="change.path">
                <code>{{ change.path }}</code><span>{{ formatChangeValue(change.before) }} → {{ formatChangeValue(change.after) }}</span>
              </li>
            </ul>
            <details v-if="activeWorkspace === 'more' && latestOperationResult.commandId === selectedCommand.id && latestOperationResult.data">
              <summary>Structured result</summary>
              <pre>{{ JSON.stringify(latestOperationResult.data, null, 2) }}</pre>
            </details>
          </section>
          <section v-if="selectedQueryFailure" class="operation-banner failure" data-query-load-failure role="alert">
            <header><strong>REFERENCE LOAD FAILED</strong><span>{{ commandName(selectedQueryFailure.commandId) }} · {{ selectedQueryFailure.commandId }}</span></header>
            <p>{{ selectedQueryFailure.errorCode }} :: {{ selectedQueryFailure.errorMessage }}</p>
          </section>
          <TestSetups
            v-if="selectedCommand.id === testSetupStatusCommandID"
            :data="latestTestSetupStatusResult?.data as TestSetupStatusData | undefined"
            :updated-at="latestTestSetupStatusResult?.receivedAt"
            :pending-status="commandPending(testSetupStatusCommandID)"
            :pending-hole-up="commandPending(commandTestHoleUpCommandID)"
            :pending-kill-party="commandPending(deathKillPartyCommandID)"
            :pending-death-sequence="commandPending(deathTestSequenceCommandID)"
            @refresh="runQueryByID(testSetupStatusCommandID, false)"
            @run="(operation) => runSpecializedMutation(operation.commandID, operation.argumentsByID)"
          />
          <LoggingConsole
            v-else-if="selectedCommand.id === loggingStatusCommandID"
            :data="latestLoggingResult?.data ? loggingStatusData(latestLoggingResult.data) : undefined"
            :updated-at="latestLoggingResult?.receivedAt"
            :pending="commandPending(loggingStatusCommandID) || loggingMutationCommandIDs.some(commandPending)"
            @mutate="(mutation) => runSpecializedMutation(mutation.commandID, mutation.argumentsByID)"
            @refresh="runQueryByID(loggingStatusCommandID, false)"
          />
          <SearchItemStacksInspector
            v-else-if="selectedCommand.id === searchStatusCommandID"
            :data="latestSearchResult?.data ? searchStatusData(latestSearchResult.data) : undefined"
            :updated-at="latestSearchResult?.receivedAt"
            :add-pending="commandPending(searchItemStackAddCommandID)"
            :reset-pending="commandPending(searchProgressResetCommandID)"
            :overlay-pending="commandPending(searchOverlaySetCommandID)"
            :search-pending="commandPending(commandTestSearchCommandID)"
            @add="addSearchItemStack"
            @reset="resetSearchProgress"
            @set-overlay="(enabled) => runSpecializedMutation(searchOverlaySetCommandID, { enabled: String(enabled) })"
            @search="(direction) => runSpecializedMutation(commandTestSearchCommandID, { direction })"
          />
          <QuestInventoryInspector
            v-else-if="selectedCommand.id === questItemStatusCommandID"
            :data="latestQuestItemResult?.data ? questItemStatusData(latestQuestItemResult.data) : undefined"
            :updated-at="latestQuestItemResult?.receivedAt"
            :pending="commandPending(questItemEnsureCommandID) || commandPending(questItemRemoveCommandID) || commandPending(questShardUseCommandID)"
            @ensure="(item) => runSpecializedMutation(questItemEnsureCommandID, { item })"
            @remove="(item) => runSpecializedMutation(questItemRemoveCommandID, { item })"
			@use-shard="(argumentsByID) => runSpecializedMutation(questShardUseCommandID, argumentsByID)"
          />
          <ShrineProgressInspector
            v-else-if="selectedCommand.id === shrineStatusCommandID"
            :data="latestShrineStatusResult?.data ? shrineStatusData(latestShrineStatusResult.data) : undefined"
            :selected-index="selectedShrineIndex"
            :updated-at="latestShrineStatusResult?.receivedAt"
            :pending="commandPending(shrineProgressCommandID)"
            :preset-pending="commandPending(codexQuestStateSetAllCommandID)"
            @select="selectShrine"
            @apply="(argumentsByID) => runSpecializedMutation(shrineProgressCommandID, argumentsByID)"
            @preset="(argumentsByID) => runSpecializedMutation(codexQuestStateSetAllCommandID, argumentsByID)"
          />
          <MerchantTester
            v-else-if="selectedCommand.id === merchantTesterCatalogCommandID"
            :catalog="latestMerchantTesterCatalogResult?.data ? merchantTesterCatalogData(latestMerchantTesterCatalogResult.data) : undefined"
            :catalog-scene-generation="latestMerchantTesterCatalogResult?.sceneGeneration"
            :commands="commands"
            :scene-generation="sceneGeneration"
            :pending-preview="commandPending(merchantTesterPreviewCommandID)"
            :pending-live="commandPending(merchantTesterLiveCommandID)"
            @preview="(argumentsByID) => runSpecializedMutation(merchantTesterPreviewCommandID, argumentsByID)"
            @live="(argumentsByID) => runSpecializedMutation(merchantTesterLiveCommandID, argumentsByID)"
          />
          <MonsterEcosystemConsole
            v-else-if="selectedCommand.id === monsterEcosystemStatusCommandID"
            :data="latestMonsterEcosystemResult?.data ? monsterEcosystemData(latestMonsterEcosystemResult.data) : undefined"
            :pending-generation="commandPending(monsterGenerationSetCommandID)"
            :pending-clear="commandPending(monsterClearMapEnemiesCommandID)"
            :pending-spawn="commandPending(monsterSpawnWhirlpoolCommandID) || commandPending(monsterSpawnPirateCommandID)"
            @set-generation="setMonsterGeneration"
            @clear="(value) => runSpecializedMutation(monsterClearMapEnemiesCommandID, {
              expectedLocationId: String(value.expectedLocationId),
              expectedFloor: String(value.expectedFloor),
              expectedEnemyCount: String(value.expectedEnemyCount),
            })"
            @spawn-whirlpool="runSpecializedMutation(monsterSpawnWhirlpoolCommandID, {})"
            @spawn-pirate="runSpecializedMutation(monsterSpawnPirateCommandID, {})"
          />
		  <DebugOptionsConsole
			v-else-if="selectedCommand.id === debugOptionsStatusCommandID"
			:data="latestDebugOptionsResult?.data ? debugOptionsStatusData(latestDebugOptionsResult.data) : undefined"
			:updated-at="latestDebugOptionsResult?.receivedAt"
			:pending="debugOptionsMutationCommandIDs.some(commandPending)"
			@set-option="(commandID, enabled) => runSpecializedMutation(commandID, { enabled: String(enabled) })"
		  />
		  <ShipsConsole
            v-else-if="selectedCommand.id === vehicleStatusCommandID"
            :data="latestVehicleStatusResult?.data ? vehicleStatusData(latestVehicleStatusResult.data) : undefined"
            :pending-damage="commandPending(shipDamageRawCommandID)"
            :pending-repair="commandPending(shipRepairFullCommandID)"
			:pending-skiffs="commandPending(shipSkiffSetCommandID)"
			:commands="commands"
			:pending-spawn="commandPending(vehicleSpawnCommandID)"
			:pending-force-board="commandPending(vehicleForceBoardCommandID)"
            @damage="(amount) => runSpecializedMutation(shipDamageRawCommandID, { amount: String(amount) })"
            @repair="runSpecializedMutation(shipRepairFullCommandID, {})"
			@set-skiffs="(quantity) => runSpecializedMutation(shipSkiffSetCommandID, { quantity: String(quantity) })"
			@spawn="(argumentsByID) => runSpecializedMutation(vehicleSpawnCommandID, argumentsByID)"
			@force-board="(type) => runSpecializedMutation(vehicleForceBoardCommandID, { type })"
          />
		  <WorldDiagnostics
			v-else-if="selectedCommand.id === guardStatusCommandID"
			:guards="latestGuardStatusResult?.data ? guardStatusData(latestGuardStatusResult.data) : undefined"
			:shadowlords="latestShadowlordStatusResult?.data ? shadowlordStatusData(latestShadowlordStatusResult.data) : undefined"
			:death="latestDeathStatusResult?.data ? deathStatusData(latestDeathStatusResult.data) : undefined"
			:pending="worldDiagnosticCommandIDs.some(commandPending)"
			@refresh="worldDiagnosticCommandIDs.forEach((commandID) => runQueryByID(commandID, false))"
		  />
          <ObservationPane
            v-else-if="selectedCommand.id === observationCommandID"
            :data="latestObservationResult?.data ? observationResultData(latestObservationResult.data) : undefined"
            :command-id="latestObservationResult?.commandId as ObservationCommandID | undefined"
            :updated-at="latestObservationResult?.receivedAt"
            :pending="observationPending"
            @run="(commandID) => runSpecializedMutation(commandID, {})"
          />
          <form v-else :data-command-form="selectedCommand.id" novalidate @submit.prevent="runCommand(selectedCommand)">
            <template v-for="parameter in selectedCommand.parameters ?? []" :key="parameter.id">
              <div class="parameter-field">
                <label :for="`${selectedCommand.id}-${parameter.id}`">
                  {{ parameter.name }}<span v-if="parameter.required" aria-hidden="true"> *</span>
                </label>
                <small v-if="parameter.description" :id="`${selectedCommand.id}-${parameter.id}-help`">{{ parameter.description }}</small>

                <select
                  v-if="parameter.kind === 'enum' || parameter.kind === 'boolean' || parameter.kind === 'character'"
                  :id="`${selectedCommand.id}-${parameter.id}`"
                  :value="draftFor(selectedCommand)[parameter.id] ?? ''"
                  :required="parameter.required"
                  :aria-describedby="describedBy(selectedCommand, parameter.id)"
                  :aria-invalid="Boolean(errorFor(selectedCommand.id, parameter.id))"
                  @change="updateDraft(selectedCommand, parameter.id, inputValue($event))"
                >
                  <option value="">{{ parameter.required ? 'SELECT A VALUE' : 'UNSET' }}</option>
                  <template v-if="parameter.kind === 'boolean'">
                    <option value="true">ON</option>
                    <option value="false">OFF</option>
                  </template>
                  <option v-for="choice in choicesForParameter(parameter, optionSets)" :key="choice.value" :value="choice.value">{{ choice.label }}</option>
                </select>

                <input
                  v-else-if="parameter.kind === 'integer'"
                  :id="`${selectedCommand.id}-${parameter.id}`"
                  type="number"
                  :step="parameter.step ?? 1"
                  :min="parameter.minimum"
                  :max="parameter.maximum"
                  :value="draftFor(selectedCommand)[parameter.id] ?? ''"
                  :required="parameter.required"
                  :aria-describedby="describedBy(selectedCommand, parameter.id)"
                  :aria-invalid="Boolean(errorFor(selectedCommand.id, parameter.id))"
                  @input="updateDraft(selectedCommand, parameter.id, inputValue($event))"
                />

                <input
                  v-else-if="parameter.kind === 'string'"
                  :id="`${selectedCommand.id}-${parameter.id}`"
                  type="text"
                  :pattern="parameter.pattern"
                  :value="draftFor(selectedCommand)[parameter.id] ?? ''"
                  :required="parameter.required"
                  :aria-describedby="describedBy(selectedCommand, parameter.id)"
                  :aria-invalid="Boolean(errorFor(selectedCommand.id, parameter.id))"
                  @input="updateDraft(selectedCommand, parameter.id, inputValue($event))"
                />

                <p v-else class="schema-error">UNSUPPORTED CONTROL: {{ parameter.kind }}</p>
                <p v-if="errorFor(selectedCommand.id, parameter.id)" :id="`${selectedCommand.id}-${parameter.id}-error`" class="field-error">
                  {{ errorFor(selectedCommand.id, parameter.id) }}
                </p>
              </div>
            </template>

            <p v-if="formMessages[selectedCommand.id]" class="form-error" aria-live="polite">{{ formMessages[selectedCommand.id] }}</p>
            <p v-if="commandUnavailableReason(selectedCommand)" class="disabled-reason" role="status">
              {{ commandUnavailableReason(selectedCommand) }}
            </p>
            <div v-if="activeWorkspace === 'more' || isMutation(selectedCommand)" class="instrument-actions" :class="{ 'has-reset': Boolean(selectedCommand.parameters?.length) }">
              <button
                type="submit"
                :data-command-id="selectedCommand.id"
                :disabled="commandApplyDisabled(selectedCommand)"
              >
                {{ commandPending(selectedCommand.id) ? commandPendingLabel(selectedCommand.id) : isMutation(selectedCommand) ? "APPLY CHANGES" : "RUN QUERY" }}
              </button>
              <button v-if="selectedCommand.parameters?.length" type="button" class="reset-button" :disabled="commandPending(selectedCommand.id)" @click="resetCommand(selectedCommand)">
                RESET
              </button>
            </div>
          </form>
          <p v-if="!isReady" class="disabled-reason">EXECUTION DISABLED UNTIL CONNECTION IS READY</p>
        </article>
        <p v-else class="empty-state">SELECT AN INSTRUMENT</p>
        </section>
      </div>
    </section>

    <section
      ref="historyPanel"
      class="history-panel"
      :class="{ collapsed: historyCollapsed }"
      :style="historyPanelStyle"
      :data-state="historyCollapsed ? 'collapsed' : 'expanded'"
      :data-dock="historyDock"
      aria-labelledby="history-title"
    >
      <div v-show="!historyCollapsed" class="history-resize-handle" role="separator" aria-label="Resize global history" :aria-orientation="historyDock === 'right' ? 'vertical' : 'horizontal'" @pointerdown="startHistoryResize" />
      <div class="section-heading result-heading">
        <span aria-hidden="true">▤</span>
        <h2 id="history-title">Global History</h2>
        <div class="stream-actions">
          <button type="button" :aria-expanded="!historyCollapsed" :data-state="historyCollapsed ? 'collapsed' : 'expanded'" data-history-action="collapse" @click="toggleHistory">{{ historyCollapsed ? 'EXPAND HISTORY' : 'COLLAPSE HISTORY' }}</button>
          <button type="button" :data-state="historyDock" data-history-action="dock" @click="toggleHistoryDock">DOCK {{ historyDock === 'bottom' ? 'RIGHT' : 'BOTTOM' }}</button>
          <button type="button" role="switch" :aria-checked="followResults" :data-state="followResults ? 'active' : 'off'" data-history-action="follow" @click="toggleFollow">FOLLOW {{ followResults ? "ON" : "OFF" }}</button>
          <button type="button" :data-state="results.length ? 'ready' : 'unavailable'" data-history-action="clear" :disabled="results.length === 0" @click="clearResults">CLEAR HISTORY</button>
        </div>
      </div>
      <div v-show="!historyCollapsed" ref="stream" class="result-stream" role="region" aria-label="Global command history" tabindex="0" @scroll="onStreamScroll">
        <p v-if="trimmedResults" class="trimmed">{{ trimmedResults }} OLDER RESULTS TRIMMED</p>
        <article v-for="result in results" :key="result.entryID" class="result-entry" :data-state="result.success ? 'success' : 'failure'" :data-history-request="result.requestId">
          <header>
            <time>{{ formatTime(result.receivedAt) }}</time>
            <strong>{{ commandName(result.commandId) }}</strong>
            <span :class="result.success ? 'success' : 'failure'">{{ result.success ? "SUCCESS" : "FAILED" }}</span>
          </header>
          <p v-if="result.summary" class="summary">{{ result.summary }}</p>
          <p v-if="!result.success" class="error-line">{{ result.errorCode }} :: {{ result.errorMessage }}</p>
          <footer>REQ {{ result.requestId }} · SCENE {{ result.sceneGeneration }}</footer>
        </article>
        <div v-if="results.length === 0" class="empty-history">NO COMMAND HISTORY</div>
      </div>
    </section>

    <CommandPalette v-if="commandPaletteOpen" :surfaces="navigationSurfaces" :recents="navigationRecents" @close="closeCommandPalette" @activate="activateNavigationSurface" />

    <footer class="keyboard-line">CTRL/⌘K GO TO · TAB NAVIGATE · ENTER ACTIVATE · F5 REFRESH · RESULTS RETAINED {{ results.length }}/{{ resultLimit }}</footer>
  </main>
</template>
