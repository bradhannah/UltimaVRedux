<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type { InventoryLockerStatusData, QuestItemStatusData, SpellInventoryStatusData } from "./approved_tool_views"
import PageChangeBar from "./PageChangeBar.vue"
import { plural, type PageStep } from "./page_steps"
import type { CommandDefinition } from "./protocol"

type Kind = "supply" | "reagent" | "potion" | "scroll" | "equipment" | "quest" | "spell"
type TabID = "supplies" | "reagents" | "consumables" | "equipment" | "quest" | "spells"

interface Row {
  id: string
  kind: Kind
  key: string
  name: string
  have: number
  max: number
  note: string
}

interface StagedPreset {
  commandID: string
  label: string
  argumentsByID: Record<string, string>
  /** Which snapshot the preset was chosen against; it is dropped when that snapshot moves on. */
  snapshot: string
  source: "locker" | "spells"
  detail?: string
}

const props = defineProps<{
  locker?: InventoryLockerStatusData
  spells?: SpellInventoryStatusData
  quest?: QuestItemStatusData
  commands: CommandDefinition[]
  busy: boolean
}>()

const emit = defineEmits<{ apply: [steps: PageStep[]] }>()

const tabs: { id: TabID; label: string; kinds: Kind[] }[] = [
  { id: "supplies", label: "Supplies", kinds: ["supply"] },
  { id: "reagents", label: "Reagents", kinds: ["reagent"] },
  { id: "consumables", label: "Potions & scrolls", kinds: ["potion", "scroll"] },
  { id: "equipment", label: "Equipment stock", kinds: ["equipment"] },
  { id: "quest", label: "Quest items", kinds: ["quest"] },
  { id: "spells", label: "Prepared spells", kinds: ["spell"] },
]
const kindLabel: Record<Kind, string> = { supply: "Supply", reagent: "Reagent", potion: "Potion", scroll: "Scroll", equipment: "Equipment", quest: "Quest item", spell: "Spell" }

const tab = useInstrumentDraft<TabID>("items.tab", "supplies")
const search = useInstrumentDraft("items.search", "")
const targets = useInstrumentDraft<Record<string, string>>("items.targets", {})
const staged = useInstrumentDraft<StagedPreset | undefined>("items.preset", undefined)
const lightMinutes = useInstrumentDraft("items.lightMinutes", "1")
const effectTurns = useInstrumentDraft("items.effectTurns", "1")
const notice = ref("")

const rows = computed<Row[]>(() => {
  const list: Row[] = []
  for (const item of props.locker?.provisions ?? []) list.push({ id: `supply:${item.name}`, kind: "supply", key: item.name, name: item.name.replaceAll("_", " "), have: item.quantity, max: item.maximum, note: "" })
  for (const item of props.locker?.reagents ?? []) list.push({ id: `reagent:${item.id}`, kind: "reagent", key: String(item.id), name: item.name, have: item.quantity, max: item.maximum, note: "" })
  for (const item of props.locker?.potions ?? []) list.push({ id: `potion:${item.id}`, kind: "potion", key: String(item.id), name: `${item.name} potion`, have: item.quantity, max: item.maximum, note: "Can only be added" })
  for (const item of props.locker?.scrolls ?? []) list.push({ id: `scroll:${item.id}`, kind: "scroll", key: String(item.id), name: `${item.name} scroll`, have: item.quantity, max: item.maximum, note: "Can only be added" })
  for (const item of props.locker?.equipment ?? []) list.push({ id: `equipment:${item.id}`, kind: "equipment", key: String(item.id), name: item.name, have: item.quantity, max: item.maximum, note: `${item.category}${item.ammo ? ", ammo" : ""}` })
  for (const item of props.quest?.items ?? []) list.push({ id: `quest:${item.key}`, kind: "quest", key: item.key, name: item.name, have: item.quantity > 0 ? 1 : 0, max: 1, note: item.category })
  for (const item of props.spells?.spells ?? []) list.push({ id: `spell:${item.id}`, kind: "spell", key: String(item.id), name: item.name, have: item.quantity, max: item.maximum, note: `Circle ${item.circle}` })
  return list
})
const query = computed(() => search.value.trim().toLocaleLowerCase())
const visibleRows = computed(() => {
  if (query.value) return rows.value.filter((row) => `${row.name} ${row.note} ${kindLabel[row.kind]}`.toLocaleLowerCase().includes(query.value))
  const kinds = tabs.find((candidate) => candidate.id === tab.value)?.kinds ?? []
  return rows.value.filter((row) => kinds.includes(row.kind))
})

function target(row: Row): string {
  return targets.value[row.id] ?? String(row.have)
}
function rowError(row: Row): string {
  const value = target(row)
  const parsed = Number(value)
  if (!value.trim() || !Number.isInteger(parsed) || parsed < 0 || parsed > row.max) return `Enter a whole number from 0 to ${row.max}.`
  if ((row.kind === "potion" || row.kind === "scroll") && parsed < row.have) return "Potions and scrolls can only be added."
  return ""
}
function isChanged(row: Row): boolean {
  return targets.value[row.id] !== undefined && target(row) !== String(row.have)
}
const changedRows = computed(() => rows.value.filter(isChanged))
const invalidRows = computed(() => changedRows.value.filter((row) => rowError(row)))

function setTarget(row: Row, value: string): void {
  staged.value = undefined
  notice.value = ""
  const next = { ...targets.value }
  if (value === String(row.have)) delete next[row.id]
  else next[row.id] = value
  targets.value = next
}
function step(row: Row, delta: number): void {
  const current = Number(target(row))
  setTarget(row, String(Math.min(row.max, Math.max(0, (Number.isFinite(current) ? current : row.have) + delta))))
}

// Rows keep their edits while the live numbers move, and drop edits the game now agrees with.
watch(rows, (current) => {
  const next: Record<string, string> = {}
  for (const row of current) {
    const value = targets.value[row.id]
    if (value !== undefined && value !== String(row.have)) next[row.id] = value
  }
  if (Object.keys(next).length !== Object.keys(targets.value).length) targets.value = next
})
watch([() => props.locker?.snapshotFingerprint, () => props.spells?.snapshotFingerprint], () => {
  const preset = staged.value
  if (!preset) return
  const live = preset.source === "locker" ? props.locker?.snapshotFingerprint : props.spells?.snapshotFingerprint
  if (live !== preset.snapshot) {
    staged.value = undefined
    notice.value = "The inventory changed after you picked that preset. Pick it again to review it against the new numbers."
  }
})

function presetChoices(commandID: string): { value: string; label: string }[] {
  return props.commands.find((candidate) => candidate.id === commandID)?.parameters?.find((parameter) => parameter.id === "preset")?.choices ?? []
}
const presets = computed(() => {
  const list: StagedPreset[] = []
  const lockerSnapshot = props.locker?.snapshotFingerprint ?? ""
  const fromCatalog = (commandID: string): void => {
    for (const choice of presetChoices(commandID)) list.push({ commandID, label: choice.label, argumentsByID: { preset: choice.value, expectedSnapshot: lockerSnapshot }, snapshot: lockerSnapshot, source: "locker" })
  }
  const show = (kinds: Kind[]): boolean => !query.value && (tabs.find((candidate) => candidate.id === tab.value)?.kinds ?? []).some((kind) => kinds.includes(kind))
  if (!props.locker) return []
  if (show(["supply"])) fromCatalog("inventory.provision.preset")
  if (show(["reagent"])) fromCatalog("inventory.reagent.preset")
  if (show(["potion", "scroll"])) { fromCatalog("inventory.potion.preset"); fromCatalog("inventory.scroll.preset") }
  if (show(["equipment"])) {
    fromCatalog("inventory.equipment.preset")
    fromCatalog("inventory.equipment.grant-preset")
    list.push({ commandID: "inventory.equipment.clear", label: "Clear all equipment", argumentsByID: { expectedSnapshot: lockerSnapshot }, snapshot: lockerSnapshot, source: "locker", detail: "destroys all carried equipment and clears every party member's gear" })
  }
  if (show(["spell"]) && props.spells) list.push({ commandID: "inventory.spell.preset-max", label: "All spells ×99 (Nox left out)", argumentsByID: { expectedSnapshot: props.spells.snapshotFingerprint }, snapshot: props.spells.snapshotFingerprint, source: "spells" })
  if (!query.value) {
    const usable = props.locker.usablePresetReview
    list.push({ commandID: "quest.inventory.preset-usable", label: "Everything a playtest needs", argumentsByID: { expectedFingerprint: usable.reviewFingerprint }, snapshot: lockerSnapshot, source: "locker", detail: `${plural(usable.changes.length, "exact change")}` })
  }
  return list
})
function choosePreset(preset: StagedPreset): void {
  targets.value = {}
  notice.value = ""
  staged.value = preset
}

const count = computed(() => staged.value ? 1 : changedRows.value.length)
const summary = computed(() => {
  if (staged.value) return `${staged.value.label}${staged.value.detail ? ` (${staged.value.detail})` : ""}`
  const parts = changedRows.value.slice(0, 4).map((row) => `${row.name} ${row.have} → ${target(row)}`)
  return changedRows.value.length > 4 ? `${parts.join(", ")} and ${changedRows.value.length - 4} more` : parts.join(", ")
})
const blocked = computed(() => invalidRows.value.length > 0 ? `${invalidRows.value[0].name}: ${rowError(invalidRows.value[0])}` : undefined)

function stepFor(row: Row): PageStep {
  const wanted = target(row)
  const expectedQuantity = String(row.have)
  switch (row.kind) {
    case "supply": return { commandID: "inventory.provision.set", argumentsByID: { provision: row.key, targetQuantity: wanted, expectedQuantity } }
    case "reagent": return { commandID: "inventory.reagent.set", argumentsByID: { reagent: row.key, targetQuantity: wanted, expectedQuantity } }
    case "equipment": return { commandID: "inventory.equipment.set", argumentsByID: { equipment: row.key, targetQuantity: wanted, expectedQuantity } }
    case "spell": return { commandID: "inventory.spell.set", argumentsByID: { spell: row.key, targetQuantity: wanted, expectedQuantity } }
    case "potion": return { commandID: "inventory.potion.add", argumentsByID: { potion: row.key, amount: String(Number(wanted) - row.have) } }
    case "scroll": return { commandID: "inventory.scroll.add", argumentsByID: { scroll: row.key, amount: String(Number(wanted) - row.have) } }
    case "quest": return { commandID: wanted === "1" ? "quest.item.ensure" : "quest.item.remove", argumentsByID: { item: row.key } }
  }
}
function discard(): void {
  targets.value = {}
  staged.value = undefined
  notice.value = ""
}
function applyAll(): void {
  if (blocked.value) return
  const preset = staged.value
  const steps = preset ? [{ commandID: preset.commandID, argumentsByID: preset.argumentsByID }] : changedRows.value.map(stepFor)
  discard()
  if (steps.length > 0) emit("apply", steps)
}

function effectBase(): Record<string, string> {
  const spells = props.spells
  return spells ? { expectedEffect: spells.activeSpell.effectType, expectedTurns: String(spells.activeSpell.turnsRemaining), expectedLightMinutes: String(spells.magicLight.minutesRemaining) } : {}
}
function setLight(minutes: number): void {
  if (!props.spells || !Number.isInteger(minutes) || minutes < 0 || minutes > 9999) { notice.value = "Light minutes must be a whole number from 0 to 9999."; return }
  emit("apply", [{ commandID: "spell.effect.set-remaining", argumentsByID: { effectKind: "magic_light", remaining: String(minutes), ...effectBase() } }])
}
function setTurns(turns: number): void {
  if (!props.spells?.activeSpell.active || !Number.isInteger(turns) || turns < 1 || turns > 9999) { notice.value = "Turns must be a whole number from 1 to 9999 and a duration spell must be active."; return }
  emit("apply", [{ commandID: "spell.effect.set-remaining", argumentsByID: { effectKind: "active_spell", remaining: String(turns), ...effectBase() } }])
}
function activate(effect: "light" | "time_stop"): void {
  if (props.spells) emit("apply", [{ commandID: "spell.effect.activate", argumentsByID: { effect, ...effectBase() } }])
}
</script>

<template>
  <section class="pg items-page" aria-label="Items and spells" data-items-page>
    <div class="pg-head">
      <div><h1>Items &amp; spells</h1><p>Everything the party carries, in one place. Changes wait in the bar below until you apply them.</p></div>
      <label class="pg-field" style="flex: 0 1 420px">Search everything the party can carry
        <input v-model="search" type="search" placeholder="Black pearl, keys, halberd, In Lor…" data-items-search />
      </label>
    </div>
    <div class="pg-tabs" role="tablist" aria-label="Kinds of item">
      <button v-for="candidate in tabs" :key="candidate.id" type="button" role="tab" :data-items-tab="candidate.id" :aria-selected="!query && tab === candidate.id" @click="tab = candidate.id; search = ''">{{ candidate.label }}</button>
    </div>
    <p v-if="query" class="pg-note" data-items-search-note>Showing matches from every kind. Clear the search to go back to the tabs.</p>
    <div v-if="presets.length" class="pg-actions" role="group" aria-label="Presets">
      <span class="pg-muted">Presets</span>
      <button v-for="preset in presets" :key="preset.commandID + preset.label" type="button" class="pg-chip" data-items-preset :aria-pressed="staged?.label === preset.label" :disabled="busy" @click="choosePreset(preset)">{{ preset.label }}</button>
    </div>
    <p v-if="notice" class="pg-note warn" role="status" data-items-notice>{{ notice }}</p>

    <p v-if="rows.length === 0" class="pg-note">Waiting for the inventory.</p>
    <p v-else-if="visibleRows.length === 0" class="pg-note">Nothing matches “{{ search }}”.</p>
    <section v-else class="pg-rows" :aria-label="query ? 'Search results' : tabs.find((candidate) => candidate.id === tab)?.label">
      <div class="pg-row head"><span>Item</span><span>Carried</span><span>New amount</span><span></span></div>
      <div v-for="row in visibleRows" :key="row.id" class="pg-row" :class="{ changed: isChanged(row) }" :data-item-row="row.id">
        <span>{{ row.name }}<small>{{ [query ? kindLabel[row.kind] : '', row.note].filter(Boolean).join(' · ') }}</small></span>
        <span>{{ row.have }}</span>
        <span class="pg-stepper">
          <button type="button" class="pg-btn" aria-label="Less" :disabled="busy" @click="step(row, -1)">−</button>
          <input type="number" min="0" :max="row.max" :value="target(row)" :aria-label="`${row.name} new amount`" :aria-invalid="isChanged(row) && Boolean(rowError(row))" @input="setTarget(row, ($event.target as HTMLInputElement).value)" />
          <button type="button" class="pg-btn" aria-label="More" :disabled="busy" @click="step(row, 1)">+</button>
        </span>
        <button type="button" class="pg-btn small" :disabled="busy || row.have === row.max" @click="setTarget(row, String(row.max))">Max {{ row.max }}</button>
      </div>
    </section>

    <section v-if="!query && tab === 'spells' && spells" class="pg-card" aria-label="Light and duration spells" data-items-effects>
      <h3>Light and duration spells</h3>
      <p class="pg-muted">Magical light: {{ spells.magicLight.active ? `${spells.magicLight.minutesRemaining} minutes left` : "off" }}. Duration spell: {{ spells.activeSpell.active ? `${spells.activeSpell.friendlyName}, ${spells.activeSpell.turnsRemaining} turns left` : "none active" }}.</p>
      <div class="pg-actions">
        <button type="button" class="pg-btn" :disabled="busy" @click="activate('light')">Cast Light (240)</button>
        <button type="button" class="pg-btn" :disabled="busy" @click="activate('time_stop')">Cast Time Stop (20)</button>
        <button type="button" class="pg-btn" :disabled="busy || spells.magicLight.minutesRemaining === 0" @click="setLight(0)">Put the light out</button>
      </div>
      <div class="pg-field-row">
        <div class="pg-with-button"><label class="pg-field">Light minutes<input v-model="lightMinutes" type="number" min="0" max="9999" /></label><button type="button" class="pg-btn" :disabled="busy" @click="setLight(Number(lightMinutes))">Set</button></div>
        <div class="pg-with-button"><label class="pg-field">Duration spell turns<input v-model="effectTurns" type="number" min="1" max="9999" /></label><button type="button" class="pg-btn" :disabled="busy || !spells.activeSpell.active" @click="setTurns(Number(effectTurns))">Set</button></div>
      </div>
    </section>

    <PageChangeBar :count="count" :summary="summary" :busy="busy" :blocked="blocked" @discard="discard" @apply="applyAll" />
  </section>
</template>
