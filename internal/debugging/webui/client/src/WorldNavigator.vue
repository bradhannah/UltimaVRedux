<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type {
  WorldPositionDestinationData,
  WorldPositionPlacementTargetData,
  WorldPositionSetupData,
  WorldPositionStatusData,
} from "./approved_tool_views"
import type { Choice } from "./protocol"

const exactPlacement = "exact"
const worldEntrancePlacement = "worldEntrance"

type PlacementMode = typeof exactPlacement | typeof worldEntrancePlacement | string

interface WorldPositionDraft {
  locationId: number
  floor: number
  x: number
  y: number
}

interface WorldPositionApply {
  placement: string
  route?: string
  locationId: number
  floor?: number
  x?: number
  y?: number
  expectedLocationId: number
  expectedFloor: number
  expectedX: number
  expectedY: number
  expectedSourceFingerprint: string
}

const props = defineProps<{
  data?: WorldPositionStatusData
  updatedAt?: Date
  pending: boolean
  refreshing?: boolean
  stale?: boolean
  initialCoordinates?: { x: number; y: number }
  landmarks: Choice[]
  pendingLandmark: boolean
}>()

const emit = defineEmits<{
  apply: [value: WorldPositionApply]
  refresh: []
  travelLandmark: [landmark: string]
}>()

const draft = useInstrumentDraft<WorldPositionDraft | undefined>("world.draft", undefined)
const selectedLocationId = useInstrumentDraft<number | undefined>("world.location", undefined)
const locationQuery = useInstrumentDraft("world.query", "")
const locationPickerOpen = ref(false)
const activeLocationId = ref<number>()
const locationInput = ref<HTMLInputElement>()
const placement = useInstrumentDraft<PlacementMode>("world.placement", exactPlacement)
const selectedRoute = useInstrumentDraft("world.route", "")
const editing = useInstrumentDraft("world.editing", false)
const appliedTarget = ref<WorldPositionDraft>()
const consumedInitialCoordinates = useInstrumentDraft<{ x: number; y: number } | undefined>("world.consumedCoordinates", undefined)
const landmarkDraft = useInstrumentDraft("world.landmark", "")

const selectedDestination = computed(() => destinationFor(selectedLocationId.value))
const currentDestination = computed(() => destinationFor(props.data?.locationId))
const matchingDestinations = computed(() => {
  const query = locationQuery.value.trim().toLocaleLowerCase()
  if (!query || query === selectedDestination.value?.locationName.toLocaleLowerCase()) return props.data?.destinations ?? []
  return (props.data?.destinations ?? [])
    .map((destination, index) => ({ destination, index, rank: locationMatchRank(destination, query) }))
    .filter((match) => match.rank >= 0)
    .sort((left, right) => left.rank - right.rank || left.index - right.index)
    .map((match) => match.destination)
})
const activeLocation = computed(() => matchingDestinations.value.find((destination) => destination.locationId === activeLocationId.value)
  ?? matchingDestinations.value[0])
const activeLocationOptionID = computed(() => activeLocation.value
  ? `world-destination-${activeLocation.value.locationId}`
  : undefined)
const placementTargets = computed(() => selectedDestination.value?.placementTargets ?? [])
const routes = computed(() => {
  const choices = new Map<string, string>()
  for (const target of placementTargets.value) choices.set(target.route, target.routeLabel)
  return [...choices].map(([value, label]) => ({ value, label }))
})
const selectedRouteTargets = computed(() => placementTargets.value.filter((target) => target.route === selectedRoute.value))
const selectedPlacementTarget = computed(() => selectedRouteTargets.value.find((target) => target.placement === placement.value))
const exactDestination = computed(() => destinationFor(draft.value?.locationId))
const effectiveTarget = computed<WorldPositionDraft | undefined>(() => {
  if (placement.value === exactPlacement) return draft.value
  if (placement.value === worldEntrancePlacement) {
    const setup = selectedDestination.value?.worldEntrance
    return setup ? { locationId: 0, ...setup } : undefined
  }
  const target = selectedPlacementTarget.value
  return target?.available && target.setup ? { locationId: target.locationId, ...target.setup } : undefined
})
const valid = computed(() => {
  if (placement.value !== exactPlacement) return Boolean(effectiveTarget.value)
  const value = draft.value
  const destination = exactDestination.value
  return Boolean(value && destination
    && destination.floors.includes(value.floor)
    && Number.isInteger(value.x) && value.x >= destination.bounds.minimumX && value.x <= destination.bounds.maximumX
    && Number.isInteger(value.y) && value.y >= destination.bounds.minimumY && value.y <= destination.bounds.maximumY)
})
const changed = computed(() => Boolean(props.data && effectiveTarget.value
  && (placementTargets.value.length > 0 && placement.value !== exactPlacement
    || !matchesStatus(effectiveTarget.value, props.data))))
const atDestinationDefault = computed(() => {
  const destination = selectedDestination.value
  const value = draft.value
  return placement.value === exactPlacement && Boolean(destination && value
    && value.locationId === destination.locationId
    && value.floor === destination.defaultSetup.floor
    && value.x === destination.defaultSetup.x
    && value.y === destination.defaultSetup.y)
})
const atSmallMapStart = computed(() => Boolean(props.data && selectedDestination.value
  && matchesStatus({ locationId: selectedDestination.value.locationId, ...selectedDestination.value.defaultSetup }, props.data)))
const presets = computed(() => {
  if (placement.value !== exactPlacement) return []
  const bounds = selectedDestination.value?.bounds
  if (!bounds) return []
  return [
    { label: "NW", x: bounds.minimumX, y: bounds.minimumY },
    { label: "CENTER", x: Math.floor((bounds.minimumX + bounds.maximumX) / 2), y: Math.floor((bounds.minimumY + bounds.maximumY) / 2) },
    { label: "NE", x: bounds.maximumX, y: bounds.minimumY },
    { label: "SW", x: bounds.minimumX, y: bounds.maximumY },
    { label: "SE", x: bounds.maximumX, y: bounds.maximumY },
  ]
})
const applyLabel = computed(() => {
  const destination = selectedDestination.value
  if (!destination) return "APPLY"
  if (placement.value === worldEntrancePlacement) return `TELEPORT TO ${destination.locationName.toLocaleUpperCase()} ENTRANCE`
  if (placement.value !== exactPlacement) return `TELEPORT: ${selectedPlacementTarget.value?.placementLabel.toLocaleUpperCase() ?? "DUNGEON ROUTE"}`
  if (destination.locationId === 0) return "TELEPORT TO WORLD POSITION"
  return `TELEPORT TO EXACT ${destination.locationName.toLocaleUpperCase()} POSITION`
})
const targetSummary = computed(() => {
  const target = effectiveTarget.value
  const destination = destinationFor(target?.locationId)
  if (!target || !destination) return "Draft target is unavailable"
  return `${destination.locationName} / ${target.floor} / ${target.x},${target.y}`
})

watch(() => props.data, (data) => {
  if (!data) return
  const preserveLocationPicker = locationPickerOpen.value
  const preservedLocationQuery = locationQuery.value
  if (appliedTarget.value && matchesStatus(appliedTarget.value, data)) {
    if (draft.value && matchesStatus(draft.value, data)) resetToStatus(data)
    appliedTarget.value = undefined
  } else if (!draft.value || !editing.value) {
    resetToStatus(data)
  }
  if (preserveLocationPicker) {
    locationQuery.value = preservedLocationQuery
    locationPickerOpen.value = true
  }
}, { immediate: true })

watch([() => props.data, () => props.initialCoordinates], ([data, coordinates]) => {
  if (!data || !coordinates || consumedInitialCoordinates.value === coordinates) return
  consumedInitialCoordinates.value = coordinates
  if (!draft.value) resetToStatus(data)
  placement.value = exactPlacement
  setCoordinates(coordinates.x, coordinates.y)
}, { immediate: true })

watch(matchingDestinations, (destinations) => {
  if (!destinations.some((destination) => destination.locationId === activeLocationId.value)) {
    activeLocationId.value = destinations[0]?.locationId
  }
})

function statusDraft(data: WorldPositionStatusData): WorldPositionDraft {
  return { locationId: data.locationId, floor: data.floor, x: data.x, y: data.y }
}

function resetToStatus(data = props.data): void {
  if (!data) return
  draft.value = statusDraft(data)
  selectedLocationId.value = data.locationId
  locationQuery.value = destinationFor(data.locationId)?.locationName ?? data.locationName
  placement.value = exactPlacement
  selectInitialRoute(destinationFor(data.locationId))
  editing.value = false
  appliedTarget.value = undefined
}

function resetToDestinationDefault(): void {
  const destination = selectedDestination.value
  if (!destination || placement.value !== exactPlacement) return
  draft.value = { locationId: destination.locationId, ...destination.defaultSetup }
  markEdited()
}

function quickTravel(): void {
  if (selectedDestination.value?.mapType !== "small" || props.pending || atSmallMapStart.value) return
  placement.value = exactPlacement
  resetToDestinationDefault()
  apply()
}

function matchesStatus(value: WorldPositionDraft, data: WorldPositionStatusData): boolean {
  return value.locationId === data.locationId && value.floor === data.floor && value.x === data.x && value.y === data.y
}

function destinationFor(locationId?: number): WorldPositionDestinationData | undefined {
  return props.data?.destinations.find((destination) => destination.locationId === locationId)
}

function selectInitialRoute(destination?: WorldPositionDestinationData): void {
  selectedRoute.value = destination?.placementTargets?.[0]?.route ?? ""
}

function selectLocation(destination: WorldPositionDestinationData): void {
  if (destination.locationId === selectedLocationId.value) {
    locationQuery.value = destination.locationName
    locationPickerOpen.value = false
    return
  }
  selectedLocationId.value = destination.locationId
  locationQuery.value = destination.locationName
  locationPickerOpen.value = false
  placement.value = exactPlacement
  draft.value = { locationId: destination.locationId, ...destination.defaultSetup }
  selectInitialRoute(destination)
  markEdited()
}

function locationMatchRank(destination: WorldPositionDestinationData, query: string): number {
  const name = destination.locationName.toLocaleLowerCase()
  const locationId = String(destination.locationId)
  if (name === query || locationId === query) return 0
  if (name.startsWith(query)) return 1
  if (name.split(/[^a-z0-9]+/).some((word) => word.startsWith(query))) return 2
  if (name.includes(query)) return 3
  if (destination.mapType.toLocaleLowerCase().includes(query)) return 4
  if (locationId.includes(query)) return 5
  return -1
}

function openLocationPicker(): void {
  const selectCurrentQuery = !locationPickerOpen.value
  locationPickerOpen.value = true
  activeLocationId.value = selectedLocationId.value
  if (selectCurrentQuery) void nextTick(() => locationInput.value?.select())
}

function closeLocationPicker(): void {
  const query = locationQuery.value.trim().toLocaleLowerCase()
  const exact = props.data?.destinations.find((destination) =>
    destination.locationName.toLocaleLowerCase() === query || String(destination.locationId) === query)
  if (exact && exact.locationId !== selectedLocationId.value) selectLocation(exact)
  else {
    locationQuery.value = selectedDestination.value?.locationName ?? ""
    locationPickerOpen.value = false
  }
}

function cancelLocationPicker(): void {
  locationQuery.value = selectedDestination.value?.locationName ?? ""
  locationPickerOpen.value = false
}

function toggleLocationPicker(): void {
  if (locationPickerOpen.value) {
    closeLocationPicker()
    return
  }
  locationQuery.value = ""
  locationPickerOpen.value = true
  activeLocationId.value = props.data?.destinations[0]?.locationId
  void nextTick(() => locationInput.value?.focus())
}

function filterLocationPicker(): void {
  locationPickerOpen.value = true
  const query = locationQuery.value.trim().toLocaleLowerCase()
  activeLocationId.value = query === selectedDestination.value?.locationName.toLocaleLowerCase()
    ? selectedLocationId.value
    : matchingDestinations.value[0]?.locationId
}

function moveLocationSelection(offset: number): void {
  locationPickerOpen.value = true
  const currentIndex = matchingDestinations.value.findIndex((destination) => destination.locationId === activeLocation.value?.locationId)
  const nextIndex = Math.max(0, Math.min(currentIndex + offset, matchingDestinations.value.length - 1))
  activeLocationId.value = matchingDestinations.value[nextIndex]?.locationId
  scrollActiveLocationIntoView()
}

function onLocationKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape" && locationPickerOpen.value) cancelLocationPicker()
  else if (event.key === "ArrowDown") moveLocationSelection(1)
  else if (event.key === "ArrowUp") moveLocationSelection(-1)
  else if (event.key === "Home" && locationPickerOpen.value) {
    activeLocationId.value = matchingDestinations.value[0]?.locationId
    scrollActiveLocationIntoView()
  } else if (event.key === "End" && locationPickerOpen.value) {
    activeLocationId.value = matchingDestinations.value.at(-1)?.locationId
    scrollActiveLocationIntoView()
  } else if (event.key === "Enter" && locationPickerOpen.value) {
    if (activeLocation.value) selectLocation(activeLocation.value)
  } else return
  event.preventDefault()
}

function scrollActiveLocationIntoView(): void {
  void nextTick(() => {
    const option = activeLocationOptionID.value ? document.getElementById(activeLocationOptionID.value) : undefined
    if (typeof option?.scrollIntoView === "function") option.scrollIntoView({ block: "nearest" })
  })
}

function changePlacement(mode: PlacementMode): void {
  const destination = selectedDestination.value
  if (!destination || mode === placement.value) return
  if (mode === worldEntrancePlacement && !destination.worldEntrance) return
  if (mode !== exactPlacement && mode !== worldEntrancePlacement
    && !selectedRouteTargets.value.some((target) => target.placement === mode)) return
  placement.value = mode
  markEdited()
}

function changeRoute(event: Event): void {
  selectedRoute.value = (event.target as HTMLSelectElement).value
  if (placement.value !== exactPlacement && placement.value !== worldEntrancePlacement
    && !selectedRouteTargets.value.some((target) => target.placement === placement.value)) {
    placement.value = exactPlacement
  }
  markEdited()
}

function changeFloor(event: Event): void {
  if (!draft.value || placement.value !== exactPlacement) return
  draft.value = { ...draft.value, floor: Number((event.target as HTMLSelectElement).value) }
  markEdited()
}

function changeCoordinate(axis: "x" | "y", event: Event): void {
  if (!draft.value || placement.value !== exactPlacement) return
  draft.value = { ...draft.value, [axis]: (event.target as HTMLInputElement).valueAsNumber }
  markEdited()
}

function clampCoordinate(axis: "x" | "y"): void {
  const value = draft.value
  const bounds = exactDestination.value?.bounds
  if (!value || placement.value !== exactPlacement || !bounds || !Number.isFinite(value[axis])) return
  const minimum = axis === "x" ? bounds.minimumX : bounds.minimumY
  const maximum = axis === "x" ? bounds.maximumX : bounds.maximumY
  draft.value = { ...value, [axis]: Math.min(maximum, Math.max(minimum, Math.round(value[axis]))) }
  markEdited()
}

function setCoordinates(x: number, y: number): void {
  const bounds = exactDestination.value?.bounds
  if (!draft.value || placement.value !== exactPlacement || !bounds) return
  draft.value = {
    ...draft.value,
    x: Math.min(bounds.maximumX, Math.max(bounds.minimumX, Math.round(x))),
    y: Math.min(bounds.maximumY, Math.max(bounds.minimumY, Math.round(y))),
  }
  markEdited()
}

function markEdited(): void {
  editing.value = true
  appliedTarget.value = undefined
}

function floorLabel(destination: WorldPositionDestinationData, floor: number): string {
  if (destination.mapType === "dungeon") return `Level ${floor + 1} (internal ${floor})`
  if (floor === -1) return destination.mapType === "large" ? "Underworld (-1)" : "Basement (-1)"
  if (destination.mapType === "large") return "Britannia Surface (0)"
  if (floor === 0) return "Ground Floor (0)"
  return `Floor ${floor}`
}

function worldPlaneLabel(floor: number): string {
  return floor === -1 ? "Underworld" : "Britannia Surface"
}

function targetSetup(target?: WorldPositionPlacementTargetData): WorldPositionSetupData | undefined {
  return target?.available ? target.setup : undefined
}

function apply(): void {
  const source = props.data
  const target = effectiveTarget.value
  if (!source || !draft.value || !target || !valid.value || !changed.value || props.pending) return
  const reviewed = {
    expectedLocationId: source.locationId,
    expectedFloor: source.floor,
    expectedX: source.x,
    expectedY: source.y,
    expectedSourceFingerprint: source.sourceFingerprint,
  }
  if (placement.value === exactPlacement || placement.value === worldEntrancePlacement) {
    appliedTarget.value = { ...target }
    emit("apply", { placement: exactPlacement, ...target, ...reviewed })
    return
  }
  emit("apply", {
    placement: placement.value,
    route: selectedRoute.value,
    locationId: selectedLocationId.value ?? draft.value.locationId,
    ...reviewed,
  })
}
</script>

<template>
  <section class="approved-view world-navigator" aria-label="World position navigator">
    <header class="approved-view-header">
      <div><span>ATOMIC WORLD SETUP</span><h3>World Navigator</h3></div>
      <small>{{ updatedAt ? `CAPTURED ${updatedAt.toLocaleTimeString([], { hour12: false })}` : 'WAITING' }}</small>
    </header>

    <template v-if="data && draft && selectedDestination">
      <div class="approved-split world-layout">
        <section class="position-card">
          <header class="position-snapshot-heading">
            <span>POSITION SNAPSHOT</span>
            <button type="button" data-world-position-refresh :disabled="pending" :aria-disabled="pending || refreshing" @click="!refreshing && emit('refresh')">REFRESH STATUS</button>
          </header>
          <h4>{{ data.locationName }}</h4><strong>{{ data.x }}, {{ data.y }}</strong>
          <dl>
            <div><dt>Map</dt><dd>{{ data.mapType }} #{{ data.locationId }}</dd></div>
            <div><dt>Floor</dt><dd>{{ floorLabel(currentDestination ?? selectedDestination, data.floor) }}</dd></div>
            <div><dt>Bounds</dt><dd>{{ data.bounds.minimumX }},{{ data.bounds.minimumY }} to {{ data.bounds.maximumX }},{{ data.bounds.maximumY }}</dd></div>
          </dl>
          <p>Live status rebases mutation preconditions without replacing an active destination draft.</p>
          <p v-if="stale" class="field-error" role="status">Refresh failed. Showing the last successful position snapshot.</p>
        </section>

        <form class="approved-panel navigator-form" @submit.prevent="apply">
          <header><strong>COMPLETE DESTINATION DRAFT</strong><small>ONE REVIEWED OPERATION</small></header>
          <div class="navigator-destination-controls">
            <div class="world-destination-field">
              <div class="world-destination-heading">
                <label for="world-destination-input">Location</label>
                <button v-if="selectedDestination.mapType === 'small'" type="button" data-world-position-quick :title="`Teleport to ${selectedDestination.locationName} starting spot`" :aria-label="`Teleport to ${selectedDestination.locationName} starting spot`" :disabled="pending || atSmallMapStart" @click="quickTravel">QUICK</button>
              </div>
              <div class="world-destination-combobox">
                <input
                  id="world-destination-input"
                  ref="locationInput"
                  v-model="locationQuery"
                  type="search"
                  role="combobox"
                  aria-label="World destination"
                  aria-autocomplete="list"
                  :aria-controls="locationPickerOpen ? 'world-destination-options' : undefined"
                  :aria-expanded="locationPickerOpen"
                  :aria-activedescendant="locationPickerOpen ? activeLocationOptionID : undefined"
                  autocomplete="off"
                  @focus="openLocationPicker"
                  @click="openLocationPicker"
                  @input="filterLocationPicker"
                  @keydown="onLocationKeydown"
                  @blur="closeLocationPicker"
                />
                <button type="button" aria-label="Show all world destinations" aria-haspopup="listbox" :aria-controls="locationPickerOpen ? 'world-destination-options' : undefined" :aria-expanded="locationPickerOpen" @mousedown.prevent @click="toggleLocationPicker">LIST</button>
                <div v-if="locationPickerOpen" id="world-destination-options" role="listbox" aria-label="World destinations">
                  <button
                    v-for="destination in matchingDestinations"
                    :id="`world-destination-${destination.locationId}`"
                    :key="destination.locationId"
                    type="button"
                    role="option"
                    tabindex="-1"
                    :aria-selected="destination.locationId === selectedLocationId"
                    :class="{ active: destination.locationId === activeLocation?.locationId }"
                    :data-world-destination="destination.locationId"
                    @mousedown.prevent
                    @mouseenter="activeLocationId = destination.locationId"
                    @click="selectLocation(destination)"
                  ><strong>{{ destination.locationName }}</strong><small>{{ destination.mapType }} #{{ destination.locationId }}</small></button>
                  <p v-if="matchingDestinations.length === 0" class="empty-state">NO MATCHING LOCATIONS</p>
                </div>
              </div>
            </div>
          </div>

          <fieldset class="navigator-placement">
            <legend>Placement</legend>
            <label><input type="radio" name="world-placement" value="exact" :checked="placement === exactPlacement" @change="changePlacement(exactPlacement)" /> Arbitrary exact position</label>
            <label v-if="selectedDestination.worldEntrance"><input type="radio" name="world-placement" value="worldEntrance" :checked="placement === worldEntrancePlacement" @change="changePlacement(worldEntrancePlacement)" /> On its world-map entrance tile</label>
          </fieldset>

          <template v-if="placementTargets.length > 0">
            <div class="navigator-destination-controls">
              <label>Entry route
                <select :value="selectedRoute" aria-label="Dungeon entry route" @change="changeRoute">
                  <option v-for="route in routes" :key="route.value" :value="route.value">{{ route.label }}</option>
                </select>
              </label>
            </div>
            <fieldset class="navigator-placement dungeon-route-targets">
              <legend>Authoritative route targets</legend>
              <label v-for="target in selectedRouteTargets" :key="`${target.route}:${target.placement}`">
                <input
                  type="radio"
                  name="world-placement"
                  :value="target.placement"
                  :checked="placement === target.placement"
                  :disabled="!target.available"
                  @change="changePlacement(target.placement)"
                />
                {{ target.placementLabel }}<small v-if="!target.available">{{ target.unavailableReason }}</small>
              </label>
            </fieldset>
          </template>

          <template v-if="placement === exactPlacement">
            <div class="navigator-destination-controls">
              <label>Floor
                <select :value="draft.floor" aria-label="Destination floor" @change="changeFloor">
                  <option v-for="floor in selectedDestination.floors" :key="floor" :value="floor">{{ floorLabel(selectedDestination, floor) }}</option>
                </select>
              </label>
            </div>
            <div class="coordinate-controls">
              <label>X<input :value="draft.x" type="number" step="1" :min="selectedDestination.bounds.minimumX" :max="selectedDestination.bounds.maximumX" :aria-invalid="!valid" @input="changeCoordinate('x', $event)" @change="clampCoordinate('x')" /></label>
              <span>x</span>
              <label>Y<input :value="draft.y" type="number" step="1" :min="selectedDestination.bounds.minimumY" :max="selectedDestination.bounds.maximumY" :aria-invalid="!valid" @input="changeCoordinate('y', $event)" @change="clampCoordinate('y')" /></label>
            </div>
            <div class="preset-strip compact">
              <button v-for="preset in presets" :key="preset.label" type="button" @click="setCoordinates(preset.x, preset.y)">{{ preset.label }}<small>{{ preset.x }},{{ preset.y }}</small></button>
            </div>
          </template>

          <section v-else-if="placement === worldEntrancePlacement && selectedDestination.worldEntrance" class="world-entrance-summary">
            <strong>{{ selectedDestination.locationName.toLocaleUpperCase() }} WORLD ENTRANCE</strong>
            <dl>
              <div><dt>World map</dt><dd>{{ worldPlaneLabel(selectedDestination.worldEntrance.floor) }}</dd></div>
              <div><dt>Floor</dt><dd>{{ selectedDestination.worldEntrance.floor }}</dd></div>
              <div><dt>Entrance tile</dt><dd>{{ selectedDestination.worldEntrance.x }}, {{ selectedDestination.worldEntrance.y }}</dd></div>
            </dl>
            <small>Resolved from the original world-location table.</small>
          </section>

          <section v-else-if="selectedPlacementTarget" class="world-entrance-summary dungeon-route-summary">
            <strong>{{ selectedPlacementTarget.placementLabel.toLocaleUpperCase() }}</strong>
            <dl v-if="targetSetup(selectedPlacementTarget)">
              <div><dt>Route</dt><dd>{{ selectedPlacementTarget.routeLabel }}</dd></div>
              <div><dt>Target authority</dt><dd>{{ selectedPlacementTarget.locationId === 0 ? 'Large map' : selectedDestination.locationName }}</dd></div>
              <div><dt>Floor</dt><dd>{{ targetSetup(selectedPlacementTarget)?.floor }}</dd></div>
              <div><dt>Tile</dt><dd>{{ targetSetup(selectedPlacementTarget)?.x }}, {{ targetSetup(selectedPlacementTarget)?.y }}</dd></div>
              <div v-if="selectedPlacementTarget.facingLabel"><dt>Facing</dt><dd>{{ selectedPlacementTarget.facingLabel }}</dd></div>
              <div v-if="selectedPlacementTarget.lastDirectionLabel"><dt>Last direction</dt><dd>{{ selectedPlacementTarget.lastDirectionLabel }}</dd></div>
            </dl>
            <p v-else class="disabled-reason" role="status">{{ selectedPlacementTarget.unavailableReason }}</p>
            <small>Coordinates and dungeon orientation are resolved by the game process when Apply runs.</small>
          </section>

          <p v-if="!valid" class="field-error">Choose an available authoritative target or valid exact floor and coordinates.</p>
          <footer>
            <span>{{ changed ? `${data.locationName} / ${data.floor} / ${data.x},${data.y} -> ${targetSummary}` : 'Draft matches position snapshot' }}</span>
            <div>
              <button type="button" class="reset-button" :disabled="pending || !editing" @click="resetToStatus()">RESET TO CURRENT</button>
              <button v-if="placement === exactPlacement" type="button" :disabled="pending || atDestinationDefault" @click="resetToDestinationDefault">RESET EXACT DEFAULT</button>
              <button type="submit" :disabled="!valid || !changed || pending">{{ pending ? 'APPLYING...' : applyLabel }}</button>
            </div>
          </footer>
        </form>
      </div>

      <section class="travel-lanes" aria-label="Landmark setup lane">
        <header><strong>LANDMARK LANE</strong><small>SEPARATE PHYSICAL ROUTING</small></header>
        <div>
          <span>SHRINE / QUEST / FLAME</span>
          <select v-model="landmarkDraft" aria-label="Physical landmark"><option value="">SELECT LANDMARK</option><option v-for="landmark in landmarks" :key="landmark.value" :value="landmark.value">{{ landmark.label }}</option></select>
          <button type="button" :disabled="!landmarkDraft || pendingLandmark" @click="emit('travelLandmark', landmarkDraft)">{{ pendingLandmark ? 'TRAVELLING...' : 'SET UP' }}</button>
          <small>Physical shrines, quest landmarks, and usable positions directly south of each blue Flame. Spirituality still requires its moongate.</small>
        </div>
        <div><span>WATERFALL</span><button type="button" disabled>DEFERRED</button><small>The authentic two-tile hazard sequence is not implemented.</small></div>
      </section>
    </template>
    <p v-else class="empty-state">WAITING FOR WORLD POSITION</p>
  </section>
</template>
