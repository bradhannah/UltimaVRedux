<script setup lang="ts">
import { computed, ref } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type { InventoryLockerStatusData, PartyCharacterInspectData, PartyRosterCharacterData } from "./approved_tool_views"
import type { OptionSets } from "./command_forms"
import PageChangeBar from "./PageChangeBar.vue"
import { plural, type PageStep } from "./page_steps"
import { loadoutSlots, equipmentForSlot, type LoadoutSlotID } from "./party_equipment_loadout"
import type { Choice, CommandDefinition } from "./protocol"

interface MemberEdit {
  fields: Record<string, string>
  slots: Partial<Record<LoadoutSlotID, string>>
}

const props = defineProps<{
  roster?: PartyCharacterInspectData
  locker?: InventoryLockerStatusData
  optionSets: OptionSets
  commands: CommandDefinition[]
  busy: boolean
}>()

const emit = defineEmits<{ apply: [steps: PageStep[]] }>()

const selectedIndex = useInstrumentDraft<number | undefined>("party.selected", undefined)
const edits = useInstrumentDraft<Record<string, MemberEdit>>("party.edits", {})
const karmaEdit = useInstrumentDraft<string | undefined>("party.karma", undefined)
const adding = ref(false)
const addChoice = useInstrumentDraft("party.addChoice", "")

const updateCommand = computed(() => props.commands.find((command) => command.id === "party.character.update"))
const loadoutCommand = computed(() => props.commands.find((command) => command.id === "party.equipment.loadout.update"))
const has = (commandID: string): boolean => props.commands.some((command) => command.id === commandID)
const active = computed(() => props.roster?.roster.filter((character) => character.active) ?? [])
const inactive = computed(() => props.roster?.roster.filter((character) => !character.active) ?? [])
const fingerprint = computed(() => props.roster?.fingerprint ?? "")
const selected = computed<PartyRosterCharacterData | undefined>(() => active.value[selectedIndex.value ?? 0] ?? active.value[0])
const selectedPosition = computed(() => Math.max(0, active.value.indexOf(selected.value as PartyRosterCharacterData)))

function characterChoices(): Choice[] {
  const provider = updateCommand.value?.parameters?.find((parameter) => parameter.id === "character")?.optionsProvider
  return provider ? props.optionSets[provider] ?? [] : []
}
/** The wire token that identifies the character at this active party position. */
function tokenFor(position: number): string | undefined {
  return characterChoices().find((choice) => (choice.metadata as { partyPosition?: number } | undefined)?.partyPosition === position + 1)?.value
}
const selectedToken = computed(() => tokenFor(selectedPosition.value))
const positionChoices = computed(() => {
  const provider = updateCommand.value?.parameters?.find((parameter) => parameter.id === "partyPosition")?.optionsProvider
  const choices = provider ? props.optionSets[provider] ?? [] : []
  // The Avatar stays first, and no one else can take position 1.
  return selectedPosition.value === 0 ? [] : choices.filter((choice) => choice.value !== "1")
})
const statusChoices = computed(() => updateCommand.value?.parameters?.find((parameter) => parameter.id === "status")?.choices ?? [])

function editFor(token: string | undefined): MemberEdit | undefined {
  return token ? edits.value[token] : undefined
}
function liveValue(character: PartyRosterCharacterData, field: string): string {
  switch (field) {
    case "strength": return String(character.strength)
    case "dexterity": return String(character.dexterity)
    case "intelligence": return String(character.intelligence)
    case "currentHp": return String(character.currentHp)
    case "currentMp": return String(character.currentMp)
    case "experience": return String(character.experience)
    case "level": return String(character.level)
    case "partyPosition": return String(Math.max(0, active.value.indexOf(character)) + 1)
    default: return character.status
  }
}
function valueOf(token: string | undefined, character: PartyRosterCharacterData, field: string): string {
  return editFor(token)?.fields[field] ?? liveValue(character, field)
}
function value(field: string): string {
  return selected.value ? valueOf(selectedToken.value, selected.value, field) : ""
}
function setField(field: string, next: string): void {
  const token = selectedToken.value
  const character = selected.value
  if (!token || !character) return
  const current = edits.value[token] ?? { fields: {}, slots: {} }
  const fields = { ...current.fields }
  if (next === liveValue(character, field)) delete fields[field]
  else fields[field] = next
  commit(token, { ...current, fields })
}
function setSlot(slot: LoadoutSlotID, next: string): void {
  const token = selectedToken.value
  if (!token) return
  const current = edits.value[token] ?? { fields: {}, slots: {} }
  const slots = { ...current.slots }
  if (next === "keep") delete slots[slot]
  else slots[slot] = next
  commit(token, { ...current, slots })
}
function commit(token: string, edit: MemberEdit): void {
  const next = { ...edits.value }
  if (Object.keys(edit.fields).length === 0 && Object.keys(edit.slots).length === 0) delete next[token]
  else next[token] = edit
  edits.value = next
}

function parameterRange(field: string): { min?: number; max?: number } {
  const parameter = updateCommand.value?.parameters?.find((candidate) => candidate.id === field)
  return { min: parameter?.minimum, max: parameter?.maximum }
}
function maxHpOf(token: string | undefined, character: PartyRosterCharacterData): number {
  const level = Number(valueOf(token, character, "level"))
  return Number.isInteger(level) && level >= 1 && level <= 8 ? level * 30 : character.maximumHp
}
function maxMpOf(token: string | undefined, character: PartyRosterCharacterData): number {
  return Number(valueOf(token, character, "intelligence"))
}
function maxHp(): number {
  return selected.value ? maxHpOf(selectedToken.value, selected.value) : 0
}
function maxMp(): number {
  return selected.value ? maxMpOf(selectedToken.value, selected.value) : 0
}
function errorOf(token: string | undefined, character: PartyRosterCharacterData, field: string): string {
  if (field === "status" || field === "partyPosition") return ""
  const raw = valueOf(token, character, field)
  const parsed = Number(raw)
  if (!raw.trim() || !Number.isInteger(parsed)) return "Enter a whole number."
  if (field === "currentHp") return parsed < 1 || parsed > maxHpOf(token, character) ? `Must be between 1 and ${maxHpOf(token, character)}.` : ""
  if (field === "currentMp") return parsed < 0 || parsed > maxMpOf(token, character) ? `Must be between 0 and ${maxMpOf(token, character)}.` : ""
  const { min, max } = parameterRange(field)
  if (min !== undefined && parsed < min) return `Must be at least ${min}.`
  if (max !== undefined && parsed > max) return `Must be at most ${max}.`
  return ""
}
function fieldError(field: string): string {
  return selected.value ? errorOf(selectedToken.value, selected.value, field) : ""
}

function slotOptions(slot: LoadoutSlotID): { value: string; label: string }[] {
  return equipmentForSlot(props.locker, slot).map((item) => ({
    value: `item:${item.id}`,
    label: item.quantity > 0 ? `${item.name} (carried ${item.quantity})` : `${item.name} (will be created)`,
  }))
}
function currentSlotName(character: PartyRosterCharacterData, slot: LoadoutSlotID): string {
  const label = loadoutSlots.find((candidate) => candidate.id === slot)?.label.toLocaleLowerCase()
  const equipped = props.locker?.equipped.find((entry) => entry.rosterIndex === character.rosterIndex)
  return equipped?.slots.find((entry) => entry.slot.toLocaleLowerCase() === label)?.equipmentName || ""
}
function slotValue(slot: LoadoutSlotID): string {
  return editFor(selectedToken.value)?.slots[slot] ?? "keep"
}
function slotLabel(token: string, slot: LoadoutSlotID): string {
  const choice = edits.value[token]?.slots[slot]
  if (choice === "unequip") return "empty"
  const id = Number(choice?.replace("item:", ""))
  return equipmentForSlot(props.locker, slot).find((item) => item.id === id)?.name ?? "?"
}

function loadoutValue(slot: LoadoutSlotID, choice: string): string | undefined {
  if (choice === "unequip") return "unequip"
  const id = Number(choice.replace("item:", ""))
  const stock = equipmentForSlot(props.locker, slot).find((item) => item.id === id)?.quantity ?? 0
  const wanted = `${stock > 0 ? "inventory" : "manufacture"}:${id}`
  const parameter = loadoutCommand.value?.parameters?.find((candidate) => candidate.id === slot)
  return parameter?.choices?.some((candidate) => candidate.value === wanted) ? wanted : undefined
}

const editedMembers = computed(() => Object.entries(edits.value).flatMap(([token, edit]) => {
  const position = Number((characterChoices().find((choice) => choice.value === token)?.metadata as { partyPosition?: number } | undefined)?.partyPosition) - 1
  const character = active.value[position]
  return character ? [{ token, edit, character, position }] : []
}).sort((left, right) => left.position - right.position))
const changeCount = computed(() => editedMembers.value.reduce((sum, member) => sum + Object.keys(member.edit.fields).length + Object.keys(member.edit.slots).length, 0))
const summary = computed(() => editedMembers.value.map(({ token, edit, character }) => {
  const parts = [
    ...Object.entries(edit.fields).map(([field, next]) => `${fieldLabel(field)} ${liveValue(character, field)} → ${next}`),
    ...Object.keys(edit.slots).map((slot) => `${slotName(slot as LoadoutSlotID)} → ${slotLabel(token, slot as LoadoutSlotID)}`),
  ]
  return `${character.name}: ${parts.join(", ")}`
}).join(" · "))
const invalid = computed(() => {
  for (const { token, edit, character } of editedMembers.value) {
    for (const field of Object.keys(edit.fields)) {
      const problem = errorOf(token, character, field)
      if (problem) return `${character.name}: ${fieldLabel(field)}. ${problem}`
    }
    for (const [slot, choice] of Object.entries(edit.slots)) {
      if (loadoutValue(slot as LoadoutSlotID, choice as string) === undefined) return `${character.name}: ${slotName(slot as LoadoutSlotID)} cannot be changed to that item.`
    }
  }
  return undefined
})

const fieldLabels: Record<string, string> = { strength: "Strength", dexterity: "Dexterity", intelligence: "Intelligence", currentHp: "HP", currentMp: "MP", experience: "Experience", level: "Level", status: "Condition", partyPosition: "Party position" }
function fieldLabel(field: string): string {
  return fieldLabels[field] ?? field
}
function slotName(slot: LoadoutSlotID): string {
  return loadoutSlots.find((candidate) => candidate.id === slot)?.label ?? slot
}
function discard(): void {
  edits.value = {}
}
function applyAll(): void {
  if (invalid.value) return
  const steps: PageStep[] = []
  for (const { token, edit } of editedMembers.value) {
    const fields = Object.entries(edit.fields)
    if (fields.length > 0) steps.push({ commandID: "party.character.update", argumentsByID: { character: token, ...Object.fromEntries(fields) } })
    const slots = Object.entries(edit.slots).flatMap(([slot, choice]) => {
      const wanted = loadoutValue(slot as LoadoutSlotID, choice as string)
      return wanted ? [[slot, wanted] as const] : []
    })
    if (slots.length > 0) steps.push({ commandID: "party.equipment.loadout.update", argumentsByID: { character: token, expectedSnapshot: "$locker", ...Object.fromEntries(slots) } })
  }
  edits.value = {}
  if (steps.length > 0) emit("apply", steps)
}

/** Fills the fingerprint arguments a party-wide command declares from the live roster and inventory. */
function wideArguments(commandID: string): Record<string, string> {
  const parameters = props.commands.find((command) => command.id === commandID)?.parameters ?? []
  const argumentsByID: Record<string, string> = {}
  for (const parameter of parameters) {
    if (parameter.id === "expectedFingerprint") argumentsByID[parameter.id] = fingerprint.value
    else if (parameter.id === "expectedSnapshot") argumentsByID[parameter.id] = props.locker?.snapshotFingerprint ?? ""
  }
  return argumentsByID
}
function wide(commandID: string): void {
  if (!props.roster || props.busy) return
  emit("apply", [{ commandID, argumentsByID: wideArguments(commandID) }])
}
const wideActions = computed(() => [
  { commandID: "party.restore", label: "Heal and cure everyone", disabled: active.value.length === 0 },
  { commandID: "party.maximize-levels", label: "Max level everyone", disabled: active.value.length === 0 },
  { commandID: "party.fill", label: "Fill party to six", disabled: inactive.value.length === 0 || active.value.length >= 6 },
].filter((action) => has(action.commandID)))

function add(): void {
  const rosterIndex = addChoice.value || String(inactive.value[0]?.rosterIndex ?? "")
  if (!rosterIndex || !props.roster) return
  adding.value = false
  emit("apply", [{ commandID: "party.member.add", argumentsByID: { rosterIndex, expectedFingerprint: fingerprint.value } }])
}
const karma = computed(() => karmaEdit.value ?? String(props.roster?.karma ?? 0))
const karmaValid = computed(() => /^\d+$/.test(karma.value) && Number(karma.value) <= 99)
function setKarma(): void {
  if (karmaValid.value && Number(karma.value) !== props.roster?.karma) emit("apply", [{ commandID: "party.karma.set", argumentsByID: { value: karma.value } }])
  karmaEdit.value = undefined
}

function hpPercent(character: PartyRosterCharacterData): number {
  return character.maximumHp > 0 ? Math.round(100 * character.currentHp / character.maximumHp) : 0
}
function numberFieldMax(field: string): number | undefined {
  return field === "currentHp" ? maxHp() : field === "currentMp" ? maxMp() : parameterRange(field).max
}
const statFields = [["strength", "Strength"], ["dexterity", "Dexterity"], ["intelligence", "Intelligence"]] as const
</script>

<template>
  <section class="pg party-page" aria-label="Party" data-party-page>
    <div class="pg-head">
      <h1>Party</h1>
      <div class="pg-actions" role="group" aria-label="Whole-party actions">
        <button v-for="action in wideActions" :key="action.commandID" type="button" class="pg-btn" :data-party-wide="action.commandID" :disabled="busy || action.disabled || !roster" @click="wide(action.commandID)">{{ action.label }}</button>
        <label class="pg-field" style="flex-direction: row; align-items: center; gap: 6px">Karma<input style="width: 72px" type="number" min="0" max="99" :value="karma" :aria-invalid="!karmaValid" data-party-karma @input="karmaEdit = ($event.target as HTMLInputElement).value" /></label>
        <button v-if="has('party.karma.set')" type="button" class="pg-btn small" data-party-karma-set :disabled="busy || !karmaValid || Number(karma) === roster?.karma" @click="setKarma">Set karma</button>
      </div>
    </div>
    <p v-if="!roster" class="pg-note">Waiting for the party roster.</p>

    <template v-else>
      <div class="pg-members" role="group" aria-label="Party members">
        <button v-for="(character, position) in active" :key="character.rosterIndex" type="button" class="pg-member" :data-party-member="position + 1" :aria-pressed="character === selected" @click="selectedIndex = position">
          <b>{{ character.name }}</b>
          <small>{{ character.class }} · level {{ character.level }}</small>
          <i class="pg-meter" :class="{ low: hpPercent(character) < 50 }" aria-hidden="true"><i :style="{ width: `${hpPercent(character)}%` }" /></i>
          <small>{{ hpPercent(character) }}% HP · {{ character.status }}</small>
        </button>
        <button v-if="inactive.length > 0 && active.length < 6 && has('party.member.add')" type="button" class="pg-member" style="border-style: dashed; justify-content: center; align-items: center" data-party-add-toggle :aria-expanded="adding" :disabled="busy" @click="adding = !adding">
          + Add a member<small>{{ inactive.slice(0, 2).map((character) => character.name).join(", ") }}{{ inactive.length > 2 ? ` and ${inactive.length - 2} more` : "" }} waiting</small>
        </button>
      </div>
      <div v-if="adding" class="pg-card" data-party-add-panel>
        <div class="pg-with-button">
          <label class="pg-field">Member to add<select :value="addChoice || String(inactive[0]?.rosterIndex ?? '')" @change="addChoice = ($event.target as HTMLSelectElement).value"><option v-for="character in inactive" :key="character.rosterIndex" :value="String(character.rosterIndex)">{{ character.name }} · {{ character.class }}</option></select></label>
          <button type="button" class="pg-btn primary" data-party-add :disabled="busy" @click="add">Add to the party</button>
        </div>
      </div>

      <div v-if="selected" class="pg-grid">
        <section class="pg-card" :aria-label="`${selected.name} stats`" data-party-stats>
          <h3>{{ selected.name }} · {{ selected.class }}</h3>
          <div class="pg-field-row">
            <label v-for="[field, label] in statFields" :key="field" class="pg-field">{{ label }}<input type="number" :value="value(field)" :min="parameterRange(field).min" :max="parameterRange(field).max" :aria-invalid="Boolean(fieldError(field))" :data-party-field="field" @input="setField(field, ($event.target as HTMLInputElement).value)" /></label>
          </div>
          <div class="pg-field-row">
            <label class="pg-field">Level<input type="number" :value="value('level')" min="1" max="8" :aria-invalid="Boolean(fieldError('level'))" data-party-field="level" @input="setField('level', ($event.target as HTMLInputElement).value)" /></label>
            <label class="pg-field">Experience<input type="number" :value="value('experience')" min="0" :aria-invalid="Boolean(fieldError('experience'))" data-party-field="experience" @input="setField('experience', ($event.target as HTMLInputElement).value)" /></label>
          </div>
          <label v-if="positionChoices.length" class="pg-field">Party position<select :value="value('partyPosition')" data-party-field="partyPosition" @change="setField('partyPosition', ($event.target as HTMLSelectElement).value)"><option v-for="choice in positionChoices" :key="choice.value" :value="choice.value">{{ choice.label }}</option></select></label>
          <p class="pg-note">Setting a level also sets the experience and maximum HP that level needs.</p>
        </section>

        <section class="pg-card" aria-label="Health">
          <h3>Health</h3>
          <div class="pg-with-button">
            <label class="pg-field">Hit points (of {{ maxHp() }})<input type="number" :value="value('currentHp')" min="1" :max="numberFieldMax('currentHp')" :aria-invalid="Boolean(fieldError('currentHp'))" data-party-field="currentHp" @input="setField('currentHp', ($event.target as HTMLInputElement).value)" /></label>
            <button type="button" class="pg-btn" data-party-full="currentHp" @click="setField('currentHp', String(maxHp()))">Full</button>
          </div>
          <p v-if="fieldError('currentHp')" class="pg-note warn" role="status">{{ fieldError('currentHp') }}</p>
          <div class="pg-with-button">
            <label class="pg-field">Magic points (of {{ maxMp() }})<input type="number" :value="value('currentMp')" min="0" :max="numberFieldMax('currentMp')" :aria-invalid="Boolean(fieldError('currentMp'))" data-party-field="currentMp" @input="setField('currentMp', ($event.target as HTMLInputElement).value)" /></label>
            <button type="button" class="pg-btn" data-party-full="currentMp" @click="setField('currentMp', String(maxMp()))">Full</button>
          </div>
          <p v-if="fieldError('currentMp')" class="pg-note warn" role="status">{{ fieldError('currentMp') }}</p>
          <div class="pg-field" role="group" aria-label="Condition">
            Condition
            <div class="pg-chips">
              <button v-for="choice in statusChoices" :key="choice.value" type="button" class="pg-chip" :data-party-status="choice.value" :aria-pressed="value('status') === choice.value" @click="setField('status', choice.value)">{{ choice.label }}</button>
            </div>
          </div>
        </section>

        <section class="pg-card" aria-label="Equipment">
          <h3>Equipment</h3>
          <p v-if="!locker || !loadoutCommand" class="pg-note">Waiting for the equipment list.</p>
          <template v-else>
            <div v-for="slot in loadoutSlots" :key="slot.id" class="pg-slot" :class="{ changed: slotValue(slot.id) !== 'keep' }">
              <span>{{ slot.label }}</span>
              <select :value="slotValue(slot.id)" :aria-label="slot.label" :data-party-slot="slot.id" @change="setSlot(slot.id, ($event.target as HTMLSelectElement).value)">
                <option value="keep">{{ currentSlotName(selected, slot.id) || "— empty" }} (keep)</option>
                <option value="unequip">— take it off</option>
                <option v-for="option in slotOptions(slot.id)" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </div>
          </template>
        </section>
      </div>
    </template>
    <PageChangeBar :count="changeCount" :summary="summary" :busy="busy" :blocked="invalid" @discard="discard" @apply="applyAll" />
    <span class="sr-only">{{ plural(changeCount, "pending change") }}</span>
  </section>
</template>
