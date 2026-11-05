<script setup lang="ts">
import { computed, watch } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type { WorldPositionDestinationData, WorldPositionPlacementTargetData, WorldPositionStatusData } from "./approved_tool_views"
import type { WorldPositionApply } from "./page_steps"
import type { Choice } from "./protocol"

type Kind = "all" | "town" | "dungeon" | "landmark" | "recent"
type ModeID = "start" | "entrance" | "exact" | "dungeon_outside_entrance" | "dungeon_inside_start" | "dungeon_outside_room" | "dungeon_underworld_exit"

interface Place {
  key: string
  name: string
  detail: string
  kind: Exclude<Kind, "all" | "recent">
  kindLabel: string
  destination?: WorldPositionDestinationData
  landmark?: string
}

interface Mode {
  id: ModeID
  label: string
}

const props = defineProps<{
  data?: WorldPositionStatusData
  landmarks: Choice[]
  pending: boolean
  pendingLandmark: boolean
  refreshing: boolean
  stale: boolean
  freeMove: boolean
  freeMovePending: boolean
  canFreeMove: boolean
}>()

const emit = defineEmits<{
  apply: [value: WorldPositionApply]
  travelLandmark: [landmark: string]
  freeMove: [enabled: boolean]
  fightRoom: [value: { locationName: string; room: number }]
  refresh: []
}>()

const search = useInstrumentDraft("goto.search", "")
const kind = useInstrumentDraft<Kind>("goto.kind", "all")
const selectedKey = useInstrumentDraft("goto.place", "")
const modeDraft = useInstrumentDraft<ModeID | "">("goto.mode", "")
const levelDraft = useInstrumentDraft<number | undefined>("goto.level", undefined)
const routeDraft = useInstrumentDraft("goto.route", "")
const roomDraft = useInstrumentDraft<number | undefined>("goto.room", undefined)
const cellX = useInstrumentDraft<number | undefined>("goto.x", undefined)
const cellY = useInstrumentDraft<number | undefined>("goto.y", undefined)
const recent = useInstrumentDraft<string[]>("goto.recent", [])

const places = computed<Place[]>(() => {
  const list: Place[] = []
  for (const destination of props.data?.destinations ?? []) {
    const dungeon = destination.mapType === "dungeon"
    const world = destination.mapType === "large"
    list.push({
      key: `dest:${destination.locationId}`,
      name: destination.locationName,
      detail: dungeon ? `Dungeon · ${destination.floors.length} levels` : world ? "Overworld and Underworld maps" : `Town or castle · ${destination.floors.length > 1 ? `${destination.floors.length} floors` : "one floor"}`,
      kind: dungeon ? "dungeon" : world ? "landmark" : "town",
      kindLabel: dungeon ? "Dungeon" : world ? "Map" : "Town",
      destination,
    })
  }
  for (const landmark of props.landmarks) {
    const shrine = /shrine/i.test(landmark.label)
    list.push({ key: `land:${landmark.value}`, name: landmark.label, detail: shrine ? "Shrine · travel there" : "Landmark · travel there", kind: "landmark", kindLabel: shrine ? "Shrine" : "Landmark", landmark: landmark.value })
  }
  return list
})
const kinds: { id: Kind; label: string }[] = [
  { id: "all", label: "All" }, { id: "town", label: "Towns & castles" }, { id: "dungeon", label: "Dungeons" },
  { id: "landmark", label: "Shrines & landmarks" }, { id: "recent", label: "Recent" },
]
const visiblePlaces = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  const base = kind.value === "recent"
    ? recent.value.flatMap((key) => places.value.filter((place) => place.key === key))
    : places.value.filter((place) => kind.value === "all" || place.kind === kind.value)
  return base.filter((place) => !query || `${place.name} ${place.kindLabel}`.toLocaleLowerCase().includes(query))
})
const place = computed(() => places.value.find((candidate) => candidate.key === selectedKey.value))
const destination = computed(() => place.value?.destination)
const dungeon = computed(() => destination.value?.mapType === "dungeon")

function targetsFor(placement: string): WorldPositionPlacementTargetData[] {
  return (destination.value?.placementTargets ?? []).filter((target) => target.placement === placement)
}
const modes = computed<Mode[]>(() => {
  const target = destination.value
  if (!target) return []
  const list: Mode[] = []
  if (dungeon.value) {
    if (targetsFor("dungeon_outside_entrance").length) list.push({ id: "dungeon_outside_entrance", label: "Outside the entrance" })
    if (targetsFor("dungeon_inside_start").length) list.push({ id: "dungeon_inside_start", label: "Level start" })
    if (target.rooms?.length) list.push({ id: "dungeon_outside_room", label: "Outside a room" })
    list.push({ id: "exact", label: "Exact cell" })
    if (targetsFor("dungeon_underworld_exit").length) list.push({ id: "dungeon_underworld_exit", label: "Exit ladder" })
    return list
  }
  list.push({ id: "start", label: target.mapType === "large" ? "Default spot" : "Starting spot" })
  if (target.worldEntrance) list.push({ id: "entrance", label: "Outside, at the entrance" })
  list.push({ id: "exact", label: "Exact position" })
  return list
})
const mode = computed<ModeID | undefined>(() => modes.value.find((candidate) => candidate.id === modeDraft.value)?.id ?? modes.value[0]?.id)
const routeTargets = computed(() => mode.value && mode.value !== "exact" ? targetsFor(mode.value) : [])
const routes = computed(() => [...new Map(routeTargets.value.map((target) => [target.route, target.routeLabel])).entries()].map(([value, label]) => ({ value, label })))
const route = computed(() => routes.value.find((candidate) => candidate.value === routeDraft.value)?.value ?? routes.value[0]?.value)
const routeTarget = computed(() => routeTargets.value.find((target) => target.route === route.value))

const floors = computed(() => destination.value?.floors ?? [])
const level = computed(() => floors.value.includes(levelDraft.value ?? NaN) ? levelDraft.value as number : destination.value?.defaultSetup.floor ?? floors.value[0] ?? 0)
const rooms = computed(() => (destination.value?.rooms ?? []).filter((room) => room.floor === level.value))
const room = computed(() => rooms.value.find((candidate) => candidate.room === roomDraft.value) ?? rooms.value.find((candidate) => candidate.available))
const exact = computed(() => ({
  floor: level.value,
  x: cellX.value ?? destination.value?.defaultSetup.x ?? 0,
  y: cellY.value ?? destination.value?.defaultSetup.y ?? 0,
}))

function floorWord(floor: number): string {
  if (dungeon.value) return `level ${floor + 1}`
  if (destination.value?.mapType === "large") return floor === -1 ? "the Underworld" : "Britannia's surface"
  return floor === -1 ? "the basement" : floor === 0 ? "the ground floor" : `floor ${floor}`
}
function floorButton(floor: number): string {
  if (dungeon.value) return String(floor + 1)
  return destination.value?.mapType === "large" ? (floor === -1 ? "Underworld" : "Surface") : (floor === -1 ? "Basement" : floor === 0 ? "Ground" : String(floor))
}
function standAt(setup: { floor: number; x: number; y: number }, facing?: string): string {
  return `You will stand at cell ${setup.x},${setup.y} on ${floorWord(setup.floor)}${facing ? `, facing ${facing.toLocaleLowerCase()}` : ""}.`
}

interface Plan {
  sentence: string
  reason: string
  payload?: Omit<WorldPositionApply, "expectedLocationId" | "expectedFloor" | "expectedX" | "expectedY" | "expectedSourceFingerprint">
}
const plan = computed<Plan>(() => {
  const target = destination.value
  const status = props.data
  if (!target || !status) return { sentence: "", reason: "Choose a place first." }
  const name = target.locationName
  const here = (setup: { floor: number; x: number; y: number }, locationId: number): boolean =>
    status.locationId === locationId && status.floor === setup.floor && status.x === setup.x && status.y === setup.y
  const exactPlan = (locationId: number, setup: { floor: number; x: number; y: number }, text: string): Plan => ({
    sentence: text,
    reason: here(setup, locationId) ? "You are already standing there." : "",
    payload: { placement: "exact", locationId, ...setup },
  })
  switch (mode.value) {
    case "start": return exactPlan(target.locationId, target.defaultSetup, `You will arrive at ${name}'s starting spot, cell ${target.defaultSetup.x},${target.defaultSetup.y}.`)
    case "entrance": return target.worldEntrance ? exactPlan(0, target.worldEntrance, `You will stand outside ${name}, at cell ${target.worldEntrance.x},${target.worldEntrance.y} of the world map.`) : { sentence: "", reason: "This place has no outside entrance." }
    case "exact": {
      const bounds = target.bounds
      const valid = Number.isInteger(exact.value.x) && Number.isInteger(exact.value.y)
        && exact.value.x >= bounds.minimumX && exact.value.x <= bounds.maximumX && exact.value.y >= bounds.minimumY && exact.value.y <= bounds.maximumY
      const text = `You will stand at cell ${exact.value.x},${exact.value.y} on ${floorWord(exact.value.floor)} of ${name}.`
      return valid ? exactPlan(target.locationId, exact.value, text) : { sentence: text, reason: `Cells run from ${bounds.minimumX},${bounds.minimumY} to ${bounds.maximumX},${bounds.maximumY}.` }
    }
    case "dungeon_outside_room": {
      const chosen = room.value
      if (!chosen) return { sentence: "", reason: rooms.value.length === 0 ? `No rooms are listed on ${floorWord(level.value)}.` : "None of the rooms on this level can be reached from outside." }
      if (!chosen.available || !chosen.setup) return { sentence: "", reason: chosen.unavailableReason || `${chosen.label} cannot be approached from a free cell.` }
      return {
        sentence: `${standAt(chosen.setup, chosen.facingLabel)} Step forward to enter ${chosen.label}.`,
        reason: "",
        payload: { placement: "dungeon_outside_room", locationId: target.locationId, room: chosen.room },
      }
    }
    default: {
      const chosen = routeTarget.value
      if (!chosen) return { sentence: "", reason: "That arrival point is not available for this place." }
      if (!chosen.available) return { sentence: "", reason: chosen.unavailableReason || "That arrival point is not available right now." }
      return {
        sentence: chosen.setup ? standAt(chosen.setup, chosen.facingLabel) : `You will arrive at ${chosen.placementLabel.toLocaleLowerCase()}.`,
        reason: "",
        payload: { placement: chosen.placement, route: chosen.route, locationId: target.locationId },
      }
    }
  }
})

const landmarkPlan = computed(() => place.value?.landmark ? `You will be taken to ${place.value.name}.` : "")
const canGo = computed(() => !props.pending && !props.pendingLandmark && Boolean(place.value?.landmark || (plan.value.payload && !plan.value.reason)))
const blockedReason = computed(() => place.value?.landmark ? "" : plan.value.reason)

watch(() => destination.value?.locationId, () => { modeDraft.value = ""; levelDraft.value = undefined; roomDraft.value = undefined; routeDraft.value = ""; cellX.value = undefined; cellY.value = undefined })

function remember(): void {
  recent.value = [selectedKey.value, ...recent.value.filter((key) => key !== selectedKey.value)].slice(0, 8)
}
function go(): void {
  const status = props.data
  if (!canGo.value || !status) return
  remember()
  if (place.value?.landmark) { emit("travelLandmark", place.value.landmark); return }
  const payload = plan.value.payload
  if (!payload) return
  emit("apply", { ...payload, expectedLocationId: status.locationId, expectedFloor: status.floor, expectedX: status.x, expectedY: status.y, expectedSourceFingerprint: status.sourceFingerprint })
}
function fightRoom(): void {
  if (destination.value && room.value) emit("fightRoom", { locationName: destination.value.locationName, room: room.value.room })
}
function cell(event: Event): number {
  return (event.target as HTMLInputElement).valueAsNumber
}
</script>

<template>
  <section class="pg goto-page" aria-label="Go to" data-goto-page>
    <div class="pg-head">
      <div><h1>Go to</h1><p v-if="data" data-goto-here>Now at {{ data.locationName }}, cell {{ data.x }},{{ data.y }}, {{ data.mapType === 'dungeon' ? `level ${data.floor + 1}` : `floor ${data.floor}` }}.</p></div>
      <button type="button" class="pg-btn small" data-world-position-refresh :disabled="refreshing || pending" @click="emit('refresh')">Refresh position</button>
    </div>
    <p v-if="stale" class="pg-note warn" role="status">Refresh failed. Showing the last position the game reported.</p>
    <p v-if="!data" class="pg-note">Waiting for the position list.</p>

    <div v-else class="pg-split">
      <section class="pg-card" aria-label="Choose a place">
        <h3>1 · Where to?</h3>
        <label class="pg-field">Search places<input v-model="search" type="search" placeholder="Covetous, Honesty, Yew…" data-goto-search /></label>
        <div class="pg-chips" role="group" aria-label="Kind of place">
          <button v-for="candidate in kinds" :key="candidate.id" type="button" class="pg-chip" :data-goto-kind="candidate.id" :aria-pressed="kind === candidate.id" @click="kind = candidate.id">{{ candidate.label }}</button>
        </div>
        <ul class="pg-list" aria-label="Places">
          <li v-for="candidate in visiblePlaces" :key="candidate.key">
            <button type="button" class="pg-list-item" :data-goto-place="candidate.key" :aria-pressed="candidate.key === selectedKey" @click="selectedKey = candidate.key">
              <span><b>{{ candidate.name }}</b><small>{{ candidate.detail }}</small></span><span class="kind">{{ candidate.kindLabel }}</span>
            </button>
          </li>
          <li v-if="visiblePlaces.length === 0" class="pg-note">{{ kind === 'recent' ? "Places you go to will be listed here." : "No place matches." }}</li>
        </ul>
      </section>

      <section class="pg-card" aria-label="Arrive" data-goto-arrive>
        <template v-if="!place"><h3>2 · Arrive</h3><p class="pg-note">Pick a place on the left.</p></template>
        <template v-else-if="place.landmark">
          <h3>2 · {{ place.name }}<small>{{ place.kindLabel }}</small></h3>
          <p class="pg-sentence" data-goto-sentence>{{ landmarkPlan }}</p>
        </template>
        <template v-else-if="destination">
          <h3>2 · {{ destination.locationName }}<small>{{ dungeon ? `Dungeon · ${floors.length} levels` : place.detail }}</small></h3>
          <div v-if="floors.length > 1 && (mode === 'exact' || mode === 'dungeon_outside_room')" class="pg-field" role="group" :aria-label="dungeon ? 'Dungeon level' : 'Floor'">
            {{ dungeon ? "Level" : "Floor" }}
            <div class="pg-levels">
              <button v-for="floor in floors" :key="floor" type="button" class="pg-chip" :data-goto-level="floor" :aria-pressed="floor === level" @click="levelDraft = floor; roomDraft = undefined">{{ floorButton(floor) }}</button>
            </div>
          </div>
          <div v-if="modes.length > 1" class="pg-field" role="group" aria-label="Arrival point">
            Arrive at
            <div class="pg-chips">
              <button v-for="candidate in modes" :key="candidate.id" type="button" class="pg-chip" :data-goto-mode="candidate.id" :aria-pressed="candidate.id === mode" @click="modeDraft = candidate.id">{{ candidate.label }}</button>
            </div>
          </div>
          <div v-if="routes.length > 1" class="pg-field" role="group" aria-label="Route">
            Which map
            <div class="pg-chips"><button v-for="candidate in routes" :key="candidate.value" type="button" class="pg-chip" :aria-pressed="candidate.value === route" @click="routeDraft = candidate.value">{{ candidate.label }}</button></div>
          </div>
          <ul v-if="mode === 'dungeon_outside_room'" class="pg-list" :aria-label="`Rooms on ${floorWord(level)}`" data-goto-rooms>
            <li v-for="candidate in rooms" :key="candidate.room">
              <button type="button" class="pg-list-item" :data-goto-room="candidate.room" :aria-pressed="candidate.room === room?.room" @click="roomDraft = candidate.room">
                <span><b>{{ candidate.label }} · cell {{ candidate.roomX }},{{ candidate.roomY }}</b><small>{{ candidate.available ? (candidate.summary || "No fixed monsters listed") : (candidate.unavailableReason || "Cannot be approached from a free cell") }}</small></span>
              </button>
            </li>
            <li v-if="rooms.length === 0" class="pg-note">No rooms are listed on {{ floorWord(level) }}.</li>
          </ul>
          <div v-if="mode === 'exact'" class="pg-field-row">
            <label class="pg-field">Cell X<input type="number" :value="exact.x" :min="destination.bounds.minimumX" :max="destination.bounds.maximumX" data-goto-x @input="cellX = cell($event)" /></label>
            <label class="pg-field">Cell Y<input type="number" :value="exact.y" :min="destination.bounds.minimumY" :max="destination.bounds.maximumY" data-goto-y @input="cellY = cell($event)" /></label>
          </div>
          <p v-if="plan.sentence" class="pg-sentence" data-goto-sentence>{{ plan.sentence }}</p>
          <p v-if="blockedReason" class="pg-note warn" role="status" data-goto-reason>{{ blockedReason }}</p>
        </template>

        <label v-if="canFreeMove" class="pg-field" style="flex-direction: row; align-items: center; gap: 8px">
          <input type="checkbox" role="switch" :checked="freeMove" :disabled="freeMovePending" data-goto-free-move @change="emit('freeMove', ($event.target as HTMLInputElement).checked)" /> Walk through walls (free move), on right now
        </label>
        <div class="pg-actions">
          <button type="button" class="pg-btn primary" data-goto-go :disabled="!canGo" @click="go">{{ pending || pendingLandmark ? "Going…" : "Go there" }}</button>
          <button v-if="dungeon && mode === 'dungeon_outside_room' && room" type="button" class="pg-btn" data-goto-fight @click="fightRoom">Start this room's fight instead</button>
        </div>
      </section>
    </div>
  </section>
</template>
