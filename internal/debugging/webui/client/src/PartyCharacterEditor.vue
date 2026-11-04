<script setup lang="ts">
import { computed } from "vue"

import type { CommandDraft, FormErrors, OptionSets } from "./command_forms"
import {
  characterChoices,
  partyAlterationPreview,
  positionChoices,
  selectedCharacterMetadata,
  type PartyCharacterMetadata,
} from "./party_character_editor"
import type { Choice, CommandDefinition, ParameterDefinition } from "./protocol"

const props = defineProps<{
  command: CommandDefinition
  draft: CommandDraft
  dirtyFields: string[]
  optionSets: OptionSets
  errors: FormErrors
  message?: string
  unavailableReason?: string
  pendingLabel?: string
  applyDisabled: boolean
}>()

const emit = defineEmits<{
  update: [parameterID: string, value: string]
  submit: []
  reset: []
}>()

const characters = computed(() => characterChoices(props.command, props.optionSets))
const selected = computed(() => selectedCharacterMetadata(props.command, props.optionSets, props.draft))
const positions = computed(() => positionChoices(props.command, props.optionSets, props.draft))
const preview = computed(() => partyAlterationPreview(props.command, props.optionSets, props.draft, props.dirtyFields))
const attributes = computed(() => ["strength", "dexterity", "intelligence"]
  .map((id) => props.command.parameters?.find((parameter) => parameter.id === id))
  .filter((parameter): parameter is ParameterDefinition => parameter !== undefined))
const statuses = computed(() => props.command.parameters?.find((parameter) => parameter.id === "status")?.choices ?? [])
const resources = computed(() => ["currentHp", "currentMp", "experience"]
  .map((id) => props.command.parameters?.find((parameter) => parameter.id === id))
  .filter((parameter): parameter is ParameterDefinition => parameter !== undefined))
const levelParameter = computed(() => props.command.parameters?.find((parameter) => parameter.id === "level"))
const levelChoices = computed(() => {
	const minimum = levelParameter.value?.minimum ?? 1
	const maximum = levelParameter.value?.maximum ?? 8
	return Array.from({ length: maximum - minimum + 1 }, (_, index) => minimum + index)
})
function metadata(choice: Choice): PartyCharacterMetadata | undefined {
  return choice.metadata as PartyCharacterMetadata | undefined
}

function dirty(fieldID: string): boolean {
  return props.dirtyFields.includes(fieldID)
}

function inputValue(event: Event): string {
  return (event.target as HTMLInputElement).value
}

function resourceMaximum(parameter: ParameterDefinition): number | undefined {
	if (parameter.id === "currentHp") return dirty("level") ? Number(props.draft.level) * 30 : selected.value?.maximumHp
	if (parameter.id === "currentMp") return dirty("intelligence") ? Number(props.draft.intelligence) : selected.value?.intelligence
	return parameter.maximum
}

</script>

<template>
  <form class="party-editor" :data-command-form="command.id" novalidate @submit.prevent="emit('submit')">
    <fieldset class="party-picker">
      <legend>Active party</legend>
      <button
        v-for="character in characters"
        :key="character.value"
        type="button"
        class="party-member"
        :data-party-character="character.value"
        :class="{ selected: character.value === draft.character }"
        :aria-pressed="character.value === draft.character"
        @click="emit('update', 'character', character.value)"
      >
        <span class="party-position">{{ metadata(character)?.partyPosition }}</span>
        <span class="party-identity"><strong>{{ metadata(character)?.name }}</strong><small>{{ metadata(character)?.class }}</small></span>
        <span class="party-status" :data-status="metadata(character)?.status">{{ metadata(character)?.status }}</span>
      </button>
    </fieldset>

    <template v-if="selected">
      <header class="character-summary">
        <div>
          <span>Editing</span>
          <h4>{{ selected.name }}</h4>
          <p>Level {{ selected.level }} {{ selected.class }} · Position {{ selected.partyPosition }}</p>
        </div>
        <dl>
          <div><dt>HP</dt><dd>{{ selected.currentHp }}/{{ selected.maximumHp }}</dd></div>
          <div><dt>MP</dt><dd>{{ selected.currentMp }}/{{ selected.intelligence }}</dd></div>
          <div><dt>Status</dt><dd>{{ selected.status }}</dd></div>
        </dl>
      </header>

      <div class="party-editor-grid">
        <section class="editor-group attribute-editor">
          <header><h4>Attributes</h4><small>Blank fields remain unchanged</small></header>
          <div v-for="parameter in attributes" :key="parameter.id" class="attribute-row" :class="{ dirty: dirty(parameter.id) }">
            <label :for="`${command.id}-${parameter.id}`">{{ parameter.name.slice(0, 3).toUpperCase() }}</label>
            <span class="baseline">{{ selected[parameter.id as 'strength' | 'dexterity' | 'intelligence'] }}</span>
            <input
              :id="`${command.id}-${parameter.id}`"
              type="number"
              step="1"
              :min="parameter.minimum"
              :max="parameter.maximum"
              :value="draft[parameter.id]"
			  :placeholder="`Unchanged · ${selected[parameter.id as 'strength' | 'dexterity' | 'intelligence']}`"
              :aria-invalid="Boolean(errors[parameter.id])"
              @input="emit('update', parameter.id, inputValue($event))"
            />
            <span class="dirty-marker">{{ dirty(parameter.id) ? 'EDITED' : 'UNCHANGED' }}</span>
            <p v-if="errors[parameter.id]" class="field-error">{{ errors[parameter.id] }}</p>
          </div>
        </section>

        <section class="editor-group resource-editor">
		  <header><h4>Resources</h4><small>Blank fields remain unchanged</small></header>
		  <div v-if="levelParameter" class="attribute-row" :class="{ dirty: dirty('level') }">
			<label :for="`${command.id}-level`">LEVEL</label>
			<span class="baseline">{{ selected.level }}</span>
			<select :id="`${command.id}-level`" :value="draft.level" :aria-invalid="Boolean(errors.level)" @change="emit('update', 'level', ($event.target as HTMLSelectElement).value)">
			  <option value="">Unchanged · {{ selected.level }}</option>
			  <option v-for="level in levelChoices" :key="level" :value="String(level)">{{ level }}</option>
			</select>
			<span class="dirty-marker">{{ dirty('level') ? 'EDITED' : 'UNCHANGED' }}</span>
			<p v-if="errors.level" class="field-error">{{ errors.level }}</p>
		  </div>
          <div v-for="parameter in resources" :key="parameter.id" class="attribute-row" :class="{ dirty: dirty(parameter.id) }">
            <label :for="`${command.id}-${parameter.id}`">{{ parameter.id === 'experience' ? 'XP' : parameter.name.replace('Current ', '') }}</label>
            <span class="baseline">{{ selected[parameter.id as 'currentHp' | 'currentMp' | 'experience'] }}</span>
            <input
              :id="`${command.id}-${parameter.id}`"
              type="number"
              step="1"
              :min="parameter.minimum"
              :max="resourceMaximum(parameter)"
              :value="draft[parameter.id]"
			  :placeholder="`Unchanged · ${selected[parameter.id as 'currentHp' | 'currentMp' | 'experience']}`"
              :aria-invalid="Boolean(errors[parameter.id])"
              @input="emit('update', parameter.id, inputValue($event))"
            />
            <span class="dirty-marker">{{ dirty(parameter.id) ? 'EDITED' : 'UNCHANGED' }}</span>
            <p v-if="errors[parameter.id]" class="field-error">{{ errors[parameter.id] }}</p>
          </div>
          <p class="control-note">Level sets minimum XP and maximum HP without healing or stat boosts. Direct XP edits do not force progression.</p>
		  <p class="control-note">Only edited fields are submitted and captured. HP zero, death, resurrection, and other combat mutations require the synchronized test setup tools.</p>
        </section>

        <section class="editor-group status-editor">
          <header><h4>Status</h4><small>Choose one safe state</small></header>
          <div class="segment-control" role="group" aria-label="Character status">
            <button
              v-for="status in statuses"
              :key="status.value"
              type="button"
              :data-status-value="status.value"
              :class="{ selected: draft.status === status.value, dirty: dirty('status') && draft.status === status.value }"
              :aria-pressed="draft.status === status.value"
              @click="emit('update', 'status', status.value)"
            >
              {{ status.label }}
            </button>
          </div>
        </section>

        <section class="editor-group order-editor">
          <header><h4>Party order</h4><small>Select the destination position</small></header>
          <div class="order-strip" role="group" aria-label="Party destination position">
            <button
              v-for="position in positions"
              :key="position.value"
              type="button"
              :data-party-position="position.value"
              :class="{ selected: draft.partyPosition === position.value, dirty: dirty('partyPosition') && draft.partyPosition === position.value }"
              :aria-pressed="draft.partyPosition === position.value"
              @click="emit('update', 'partyPosition', position.value)"
            >
              <span>{{ metadata(position)?.partyPosition }}</span>
              <strong>{{ metadata(position)?.name }}</strong>
            </button>
          </div>
        </section>

        <section class="editor-group compact-preview">
          <header><h4>Pending changes</h4><small>{{ preview.length }} effective</small></header>
          <dl v-if="preview.length">
            <template v-for="change in preview" :key="change.label">
              <dt>{{ change.label }}</dt><dd><span>{{ change.before }}</span><b>→</b><strong>{{ change.after }}</strong></dd>
            </template>
          </dl>
          <p v-else>Everything matches the current game state.</p>
        </section>
      </div>
    </template>

    <p v-if="message" class="form-error" aria-live="polite">{{ message }}</p>
    <p v-if="unavailableReason" class="disabled-reason" role="status">{{ unavailableReason }}</p>

    <footer class="party-editor-actions">
      <span>{{ dirtyFields.length ? `${dirtyFields.length} edited fields` : 'No unsaved edits' }}</span>
      <button type="button" class="reset-button" :disabled="Boolean(pendingLabel) || dirtyFields.length === 0" @click="emit('reset')">RESET</button>
      <button type="submit" :data-command-id="command.id" :disabled="applyDisabled">
        {{ pendingLabel || (dirtyFields.length ? `APPLY ${dirtyFields.length} CHANGES` : 'APPLY CHANGES') }}
      </button>
    </footer>
  </form>
</template>
