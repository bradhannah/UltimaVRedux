<script setup lang="ts">
import { computed, watch } from "vue"
import type { CommandDefinition } from "./protocol"
import { validateParameter } from "./command_forms"
import { useInstrumentDraft } from "./instrument_session"

const props = defineProps<{ commandId: string; command?: CommandDefinition; value: number; disabled: boolean }>()
const emit = defineEmits<{ apply: [argumentsByID: Record<string, string>] }>()
// Keep raw DOM strings so retained drafts and validation see exactly what the
// stepped selector holds.
const draft = useInstrumentDraft<string>(`preference.${props.commandId}`, "")
const previous = useInstrumentDraft<number | undefined>(`preference.${props.commandId}.previous`, undefined)
const parameter = computed(() => props.command?.parameters?.[0])
const error = computed(() => parameter.value ? validateParameter(parameter.value, draft.value) : "Preference metadata unavailable.")
watch(() => props.value, (value) => {
  if (previous.value === undefined || draft.value === String(previous.value)) draft.value = String(value)
  previous.value = value
}, { immediate: true })
function apply(): void {
  if (props.disabled || error.value || !parameter.value || Number(draft.value) === props.value) return
  emit("apply", { [parameter.value.id]: draft.value })
}
</script>

<template>
  <section class="instrument-setting" :data-preference="commandId">
    <div><strong>{{ command?.name }}</strong><p>{{ command?.description }}</p></div>
    <select :value="draft" @change="draft = ($event.target as HTMLSelectElement).value" :aria-label="command?.name" :disabled="disabled">
      <option v-for="choice in parameter?.choices ?? []" :key="choice.value" :value="choice.value">{{ choice.label }}</option>
    </select>
    <button type="button" :disabled="disabled || Boolean(error) || Number(draft) === value" @click="apply">APPLY {{ draft }}</button>
    <small v-if="error" role="status">{{ error }}</small>
  </section>
</template>
