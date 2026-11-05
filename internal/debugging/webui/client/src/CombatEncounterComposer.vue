<script setup lang="ts">
import { computed, ref, watch } from "vue"
import SpritePreview from "./SpritePreview.vue"
import { invalidateInstrumentReview, useInstrumentDraft } from "./instrument_session"

import type {
  CombatEncounterEntityData,
  CombatEncounterOptionsData,
  CombatEncounterPreviewData,
  CombatEncounterRoomData,
} from "./approved_tool_views"

const props = defineProps<{
  options?: CombatEncounterOptionsData
  pending: boolean
  /** A dungeon room to pick once (the token changes for each request), e.g. from the Go to page. */
  preselect?: { dungeon: number; room: number; token: number }
}>()

const emit = defineEmits<{
  start: [argumentsByID: Record<string, string>]
}>()

// Nothing (0xFF) and Arch (0x87) conceal what stands on them; ref:COMBAT-0001.
const concealingTiles = new Set([255, 135])
const directions = ["north", "west", "east", "south"] as const

const territory = useInstrumentDraft<"britannia" | "dungeon">("encounter.territory", "britannia")
const britanniaMap = useInstrumentDraft("encounter.map", 0)
const dungeon = useInstrumentDraft("encounter.dungeon", 0)
const dungeonRoom = useInstrumentDraft("encounter.room", 0)
const entryDirection = useInstrumentDraft("encounter.direction", "west")
const contents = useInstrumentDraft<"authored" | "custom">("encounter.contents", "authored")
const search = useInstrumentDraft("encounter.search", "")
const monsterFilter = useInstrumentDraft("encounter.monsterFilter", "")
const monsterType = useInstrumentDraft("encounter.monster", "")
const countMode = useInstrumentDraft<"automatic" | "exact" | "maximum">("encounter.mode", "automatic")
const exactCount = useInstrumentDraft("encounter.count", 1)
const friends = useInstrumentDraft("encounter.friends", true)
const seed = useInstrumentDraft("encounter.seed", "")
const editing = useInstrumentDraft("encounter.editing", false)
const reviewed = ref(false)
invalidateInstrumentReview(() => { reviewed.value = false })

const selectedDungeon = computed(() => props.options?.dungeons.find((item) => item.id === dungeon.value))
const selectedRoom = computed(() => selectedDungeon.value?.rooms.find((item) => item.room === dungeonRoom.value))
const selectedMap = computed(() => props.options?.britanniaMaps.find((item) => item.index === britanniaMap.value))
const selectedMonster = computed(() => props.options?.monsters.find((item) => String(item.id) === monsterType.value))
const preview = computed<CombatEncounterPreviewData | undefined>(() =>
  territory.value === "britannia" ? selectedMap.value?.preview : selectedRoom.value?.preview)
const authoredAvailable = computed(() => territory.value === "dungeon" && selectedRoom.value?.floor !== undefined)
const authored = computed(() => authoredAvailable.value && contents.value === "authored")
const validSeed = computed(() => seed.value === "" || (/^\d+$/.test(seed.value) && Number.isSafeInteger(Number(seed.value))))
const exactLimit = computed(() => Math.min(16, preview.value?.monsterSlots.length ?? 16))
const valid = computed(() => Boolean(preview.value) && validSeed.value && (authored.value || (Boolean(selectedMonster.value)
  && (countMode.value !== "exact" || (Number.isInteger(exactCount.value) && exactCount.value >= 1 && exactCount.value <= exactLimit.value)))))

const searchText = computed(() => search.value.trim().toLowerCase())
const visibleMaps = computed(() => (props.options?.britanniaMaps ?? [])
  .filter((item) => item.name.toLowerCase().includes(searchText.value)))
const visibleDungeons = computed(() => (props.options?.dungeons ?? []).map((item) => ({
  ...item,
  levels: roomLevels(item.name, item.rooms),
})).filter((item) => item.levels.length > 0))

function roomLabel(room: CombatEncounterRoomData): string {
  return room.floor === undefined ? `Room ${room.room} · unplaced` : `Room ${room.room} · cell ${room.cell?.x},${room.cell?.y}`
}

function roomLevels(dungeonName: string, rooms: CombatEncounterRoomData[]): { label: string, rooms: CombatEncounterRoomData[] }[] {
  const matching = rooms.filter((room) => `${dungeonName} level ${(room.floor ?? -1) + 1} room ${room.room}`.toLowerCase().includes(searchText.value)
    || dungeonName.toLowerCase().includes(searchText.value))
  const levels = new Map<number, CombatEncounterRoomData[]>()
  for (const room of matching) levels.set(room.floor ?? -1, [...(levels.get(room.floor ?? -1) ?? []), room])
  return [...levels.entries()].sort(([a], [b]) => (a < 0 ? 99 : a) - (b < 0 ? 99 : b))
    .map(([floor, levelRooms]) => ({ label: floor < 0 ? "Unplaced" : `Level ${floor + 1}`, rooms: levelRooms }))
}

const visibleMonsters = computed(() => {
  const filter = monsterFilter.value.trim().toLowerCase()
  return (props.options?.monsters ?? []).filter((item) => item.name.toLowerCase().includes(filter))
})

const plannedSlots = computed(() => {
  const slots = preview.value?.monsterSlots.length ?? 0
  const maxGroup = selectedMonster.value?.maxGroup ?? 0
  if (countMode.value === "exact") return { filled: Math.min(exactCount.value || 0, slots), possible: 0 }
  if (countMode.value === "maximum") return { filled: Math.min(maxGroup, slots), possible: 0 }
  return { filled: 0, possible: Math.min(maxGroup, slots) }
})

interface MapCell {
  key: string
  tile: number
  marker: string
  sprite?: number
  classes: Record<string, boolean>
  title: string
}

const cells = computed<MapCell[]>(() => {
  const current = preview.value
  if (!current) return []
  const markers = new Map<string, { marker: string, sprite?: number, kind: string, title: string, concealed?: boolean }>()
  ;(current.party[entryDirection.value] ?? []).forEach((position, index) => {
    markers.set(`${position.x},${position.y}`, { marker: `P${index + 1}`, kind: "party", title: `Party slot ${index + 1}` })
  })
  if (authored.value) {
    for (const entity of current.entities) {
      markers.set(`${entity.x},${entity.y}`, {
        marker: entity.sprite < 0 ? "?" : "", sprite: entity.sprite < 0 ? undefined : entity.sprite,
        kind: entity.kind, title: `${entity.name}${entity.concealed ? " · hidden" : ""}`, concealed: entity.concealed,
      })
    }
  } else {
    current.monsterSlots.forEach((position, index) => {
      const filled = index < plannedSlots.value.filled
      const possible = index < plannedSlots.value.possible
      if (!filled && !possible) return
      markers.set(`${position.x},${position.y}`, { marker: `M${index + 1}`, kind: filled ? "slot" : "possible-slot", title: `Monster slot ${index + 1}` })
    })
  }
  const triggers = new Set(authored.value ? current.triggers.map((position) => `${position.x},${position.y}`) : [])
  return current.tiles.map((tile, index) => {
    const x = index % 11
    const y = Math.floor(index / 11)
    const key = `${x},${y}`
    const mark = markers.get(key)
    return {
      key, tile, marker: mark?.marker ?? "", sprite: mark?.sprite,
      classes: {
        concealing: concealingTiles.has(tile), trigger: triggers.has(key),
        [mark?.kind ?? "terrain"]: true, hidden: Boolean(mark?.concealed),
      },
      title: `${x},${y}${mark ? ` · ${mark.title}` : ""}${triggers.has(key) ? " · push trigger" : ""}`,
    }
  })
})

const authoredSummary = computed(() => {
  const groups = new Map<string, { entity: CombatEncounterEntityData, count: number, hidden: number }>()
  for (const entity of preview.value?.entities ?? []) {
    const group = groups.get(entity.name) ?? { entity, count: 0, hidden: 0 }
    group.count += 1
    group.hidden += entity.concealed ? 1 : 0
    groups.set(entity.name, group)
  }
  return [...groups.values()]
})

const battlefieldName = computed(() => territory.value === "britannia"
  ? selectedMap.value?.name ?? "Unknown map"
  : `${selectedDungeon.value?.name ?? "Unknown"} · ${selectedRoom.value?.floor === undefined ? "unplaced" : `Level ${(selectedRoom.value.floor) + 1}`} · Room ${dungeonRoom.value}`)
const whoSummary = computed(() => {
  if (authored.value) return "authored contents, loaded live"
  const name = selectedMonster.value?.name ?? "no monster"
  const count = countMode.value === "exact" ? `${exactCount.value}` : countMode.value
  return `${name} × ${count}${friends.value && selectedMonster.value?.friend ? ` with ${selectedMonster.value.friend.name}` : ""}`
})

watch(() => props.options, (options) => {
  if (!options || editing.value) return
  britanniaMap.value = options.britanniaMaps[0]?.index ?? 0
  dungeon.value = options.dungeons[0]?.id ?? 0
  dungeonRoom.value = options.dungeons[0]?.rooms[0]?.room ?? 0
  entryDirection.value = options.entryDirections[0]?.name ?? "west"
  monsterType.value = String(options.monsters[0]?.id ?? "")
  countMode.value = (options.countModes[0]?.name as typeof countMode.value | undefined) ?? "automatic"
}, { immediate: true })

const appliedPreselect = ref(0)
watch([() => props.preselect, () => props.options], ([request, options]) => {
  if (!request || !options || appliedPreselect.value === request.token) return
  appliedPreselect.value = request.token
  selectRoom(request.dungeon, request.room)
}, { immediate: true })

function alter(action: () => void): void {
  action()
  editing.value = true
  reviewed.value = false
}

function inputString(event: Event): string {
  return (event.target as HTMLInputElement).value
}

function selectRoom(dungeonID: number, room: number): void {
  alter(() => {
    territory.value = "dungeon"
    dungeon.value = dungeonID
    dungeonRoom.value = room
  })
}

function stepExact(delta: number): void {
  alter(() => {
    countMode.value = "exact"
    exactCount.value = Math.min(exactLimit.value, Math.max(1, (exactCount.value || 1) + delta))
  })
}

function reviewOrStart(): void {
  if (!valid.value) return
  if (!reviewed.value) {
    reviewed.value = true
    return
  }
  const argumentsByID: Record<string, string> = {
    territory: territory.value,
    entryDirection: entryDirection.value,
    contents: authored.value ? "authored" : "custom",
  }
  if (territory.value === "britannia") argumentsByID.britanniaMap = String(britanniaMap.value)
  else {
    argumentsByID.dungeon = String(dungeon.value)
    argumentsByID.dungeonRoom = String(dungeonRoom.value)
  }
  if (!authored.value) {
    argumentsByID.monsterType = monsterType.value
    argumentsByID.countMode = countMode.value
    argumentsByID.friends = String(friends.value)
    if (countMode.value === "exact") argumentsByID.exactCount = String(exactCount.value)
  }
  if (seed.value !== "") argumentsByID.seed = seed.value
  emit("start", argumentsByID)
}
</script>

<template>
  <form class="approved-view encounter-composer" aria-label="Combat encounter composer" @submit.prevent="reviewOrStart">
    <header class="approved-view-header">
      <div><span>SYNCHRONIZED COMBAT TRANSITION</span><h3>Encounter Composer</h3></div>
      <small>{{ pending ? 'STARTING...' : reviewed ? 'REVIEW STAGED' : 'DRAFT' }}</small>
    </header>

    <div v-if="options" class="encounter-layout">
      <section class="approved-panel encounter-browser" aria-label="Battlefield">
        <header><strong>1 · Where</strong><small>BATTLEFIELD</small></header>
        <label>Search maps and rooms<input :value="search" type="search" placeholder="covetous 6, glade…" @input="search = inputString($event)" /></label>
        <div class="segment-control" role="group" aria-label="Territory">
          <button v-for="choice in ['britannia', 'dungeon']" :key="choice" type="button" :class="{ selected: territory === choice }" :aria-pressed="territory === choice" @click="alter(() => territory = choice as typeof territory)">{{ choice }}</button>
        </div>
        <ul v-if="territory === 'britannia'" class="encounter-list">
          <li v-for="item in visibleMaps" :key="item.index">
            <button type="button" :class="{ selected: britanniaMap === item.index }" :aria-pressed="britanniaMap === item.index" @click="alter(() => britanniaMap = item.index)">{{ item.name }}</button>
          </li>
        </ul>
        <div v-else class="encounter-list">
          <details v-for="item in visibleDungeons" :key="item.id" :open="item.id === dungeon || searchText !== ''">
            <summary>{{ item.name }}</summary>
            <template v-for="level in item.levels" :key="level.label">
              <h4>{{ level.label }}</h4>
              <ul>
                <li v-for="room in level.rooms" :key="room.room">
                  <button type="button" :class="{ selected: dungeon === item.id && dungeonRoom === room.room }" :aria-pressed="dungeon === item.id && dungeonRoom === room.room" @click="selectRoom(item.id, room.room)">
                    {{ roomLabel(room) }}<small v-if="room.preview.entities.length">{{ room.preview.entities.length }} authored</small>
                  </button>
                </li>
              </ul>
            </template>
          </details>
        </div>
      </section>

      <section class="approved-panel encounter-map" aria-label="Map preview">
        <header><strong>{{ battlefieldName }}</strong><small>PARTY ENTERS FROM {{ entryDirection.toUpperCase() }}</small></header>
        <div class="encounter-board">
          <button v-for="direction in directions" :key="direction" type="button" :class="['entry-edge', direction, { selected: entryDirection === direction }]" :aria-pressed="entryDirection === direction" :aria-label="`Party enters from the ${direction}`" @click="alter(() => entryDirection = direction)">{{ direction }}</button>
          <div class="encounter-grid" role="img" :aria-label="`${battlefieldName} preview`">
            <div v-for="cell in cells" :key="cell.key" class="encounter-cell" :class="cell.classes" :title="cell.title">
              <SpritePreview :index="cell.tile" :size="28" />
              <SpritePreview v-if="cell.sprite !== undefined" class="encounter-occupant" :index="cell.sprite" :size="28" />
              <b v-if="cell.marker">{{ cell.marker }}</b>
            </div>
          </div>
        </div>
        <p class="encounter-legend"><span class="party">P1–P6 party</span><span class="slot">M monster slot</span><span class="concealing">dashed: hides occupants</span><span class="trigger">yellow: push trigger</span></p>
      </section>

      <section class="approved-panel encounter-contents" aria-label="Contents">
        <header><strong>2 · Who</strong><small>{{ authored ? 'AUTHORED ROOM' : 'ONE MONSTER FAMILY' }}</small></header>
        <div class="segment-control" role="group" aria-label="Contents source">
          <button type="button" :class="{ selected: authored }" :aria-pressed="authored" :disabled="!authoredAvailable" @click="alter(() => contents = 'authored')">Authored room</button>
          <button type="button" :class="{ selected: !authored }" :aria-pressed="!authored" @click="alter(() => contents = 'custom')">Custom monsters</button>
        </div>
        <template v-if="authored">
          <p class="encounter-note">Loads the room exactly as walking in would: its monsters, objects and push triggers, always live.</p>
          <ul class="authored-list">
            <li v-for="group in authoredSummary" :key="group.entity.name">
              <SpritePreview v-if="group.entity.sprite >= 0" :index="group.entity.sprite" :size="20" />
              <span>{{ group.entity.name }} × {{ group.count }}</span>
              <small v-if="group.hidden">{{ group.hidden }} hidden</small>
            </li>
            <li v-if="preview?.triggers.length"><span>Push triggers × {{ preview.triggers.length }}</span></li>
          </ul>
        </template>
        <template v-else>
          <p v-if="!authoredAvailable" class="encounter-note">{{ territory === 'dungeon' ? 'This room is not placed on any dungeon level, so only custom monsters are available.' : 'Authored contents exist only for dungeon rooms.' }}</p>
          <label>Monster<input :value="monsterFilter" type="search" placeholder="Filter monsters" @input="monsterFilter = inputString($event)" /></label>
          <ul class="encounter-list monster-list">
            <li v-for="item in visibleMonsters" :key="item.id">
              <button type="button" :class="{ selected: monsterType === String(item.id) }" :aria-pressed="monsterType === String(item.id)" @click="alter(() => monsterType = String(item.id))">{{ item.name }}<small v-if="item.friend">+ {{ item.friend.name }}</small></button>
            </li>
          </ul>
          <dl v-if="selectedMonster" class="monster-stat-grid"><dt>STR / DEX / INT</dt><dd>{{ selectedMonster.strength }} / {{ selectedMonster.dexterity }} / {{ selectedMonster.intelligence }}</dd><dt>ARM / DMG / HP</dt><dd>{{ selectedMonster.armour }} / {{ selectedMonster.damage }} / {{ selectedMonster.hitPoints }}</dd><dt>Max group</dt><dd>{{ selectedMonster.maxGroup }}</dd></dl>
          <div class="count-control" role="group" aria-label="Count">
            <button type="button" :class="{ selected: countMode === 'automatic' }" :aria-pressed="countMode === 'automatic'" @click="alter(() => countMode = 'automatic')">Auto</button>
            <button type="button" :class="{ selected: countMode === 'maximum' }" :aria-pressed="countMode === 'maximum'" @click="alter(() => countMode = 'maximum')">Max</button>
            <span :class="{ selected: countMode === 'exact' }">
              <button type="button" aria-label="One fewer" @click="stepExact(-1)">−</button>
              <label>Exact<input :value="exactCount" type="number" min="1" :max="exactLimit" :aria-invalid="countMode === 'exact' && (exactCount < 1 || exactCount > exactLimit)" @focus="alter(() => countMode = 'exact')" @input="alter(() => { countMode = 'exact'; exactCount = Number(inputString($event)) })" /></label>
              <button type="button" aria-label="One more" @click="stepExact(1)">+</button>
            </span>
          </div>
          <label class="check-control"><input :checked="friends" type="checkbox" @change="alter(() => friends = ($event.target as HTMLInputElement).checked)" /> Include authentic friend type</label>
        </template>
        <label>Deterministic seed <span>OPTIONAL</span><input :value="seed" type="text" inputmode="numeric" pattern="[0-9]*" :aria-invalid="!validSeed" placeholder="SESSION RNG" @input="alter(() => seed = inputString($event))" /></label>
      </section>
    </div>
    <p v-else class="empty-state">WAITING FOR ENCOUNTER OPTIONS</p>

    <footer class="composer-actions">
      <span>{{ reviewed ? 'Review: ' : '' }}Party enters <b>{{ battlefieldName }}</b> from the <b>{{ entryDirection }}</b> · {{ whoSummary }} · {{ seed ? `seed ${seed}` : 'session RNG' }}</span>
      <button v-if="reviewed" type="button" :disabled="pending" @click="reviewed = false">BACK</button>
      <button type="submit" :class="{ 'danger-button': reviewed }" :disabled="!options || !valid || pending">{{ pending ? 'STARTING...' : reviewed ? 'CONFIRM START' : 'START ENCOUNTER' }}</button>
    </footer>
  </form>
</template>
