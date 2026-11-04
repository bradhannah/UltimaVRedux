<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { invalidateInstrumentReview, useInstrumentDraft } from "./instrument_session"

import type { ShrineStatusData } from "./shrine_views"

const props = defineProps<{
  data?: ShrineStatusData
  selectedIndex: number
  updatedAt?: Date
  pending: boolean
  presetPending: boolean
}>()

const emit = defineEmits<{
  select: [index: number]
  apply: [argumentsByID: Record<string, string>]
  preset: [argumentsByID: Record<string, string>]
}>()

interface ReviewedPreset {
  targetState: "not_assigned" | "completed"
  label: string
  fingerprint: string
}

const questChoices = [
  { value: "not_assigned", label: "Not Assigned" },
  { value: "assigned", label: "Assigned / Seeking Codex" },
  { value: "ready_to_complete", label: "Ready to Complete" },
  { value: "offering", label: "Completed / Offering" },
]
const questDraft = useInstrumentDraft("shrines.quest", "")
const ruinedDraft = useInstrumentDraft("shrines.ruined", "")
const questEditing = useInstrumentDraft("shrines.questEditing", false)
const ruinedEditing = useInstrumentDraft("shrines.ruinedEditing", false)
const draftIndex = useInstrumentDraft<number | undefined>("shrines.draftIndex", undefined)
const reviewedPreset = ref<ReviewedPreset>()
invalidateInstrumentReview(() => { reviewedPreset.value = undefined })
const selected = computed(() => props.data?.shrines.find((shrine) => shrine.index === props.selectedIndex))
const baselineRuined = computed(() => selected.value?.ruined ? "true" : "false")
const questChanged = computed(() => questEditing.value && questDraft.value !== selected.value?.questState)
const ruinedChanged = computed(() => ruinedEditing.value && ruinedDraft.value !== baselineRuined.value)
const changedCount = computed(() => Number(questChanged.value) + Number(ruinedChanged.value))

watch(selected, (shrine) => {
  if (!shrine) return
  if (draftIndex.value !== shrine?.index) {
    draftIndex.value = shrine?.index
    reviewedPreset.value = undefined
    questDraft.value = shrine?.questState ?? ""
    ruinedDraft.value = shrine?.ruined ? "true" : "false"
    questEditing.value = false
    ruinedEditing.value = false
    return
  }
  if (!questEditing.value || questDraft.value === shrine?.questState) {
    questDraft.value = shrine?.questState ?? ""
    questEditing.value = false
  }
  const nextRuined = shrine?.ruined ? "true" : "false"
  if (!ruinedEditing.value || ruinedDraft.value === nextRuined) {
    ruinedDraft.value = nextRuined
    ruinedEditing.value = false
  }
}, { immediate: true })

watch(() => props.data?.fingerprint, (fingerprint) => {
  if (reviewedPreset.value && reviewedPreset.value.fingerprint !== fingerprint) reviewedPreset.value = undefined
})

function selectShrine(event: Event): void {
  emit("select", Number((event.target as HTMLSelectElement).value))
}

function selectQuestState(event: Event): void {
  const state = (event.target as HTMLSelectElement).value
  questDraft.value = state
  questEditing.value = state !== selected.value?.questState
}

function selectRuined(value: string): void {
  ruinedDraft.value = value
  ruinedEditing.value = value !== baselineRuined.value
}

function revert(): void {
  questDraft.value = selected.value?.questState ?? ""
  ruinedDraft.value = baselineRuined.value
  questEditing.value = false
  ruinedEditing.value = false
}

function apply(): void {
  if (!selected.value || changedCount.value === 0) return
  const argumentsByID: Record<string, string> = { index: String(selected.value.index) }
  if (questChanged.value) argumentsByID.questState = questDraft.value
  if (ruinedChanged.value) argumentsByID.ruined = ruinedDraft.value
  emit("apply", argumentsByID)
}

function stagePreset(targetState: ReviewedPreset["targetState"], label: string): void {
  if (!props.data) return
  reviewedPreset.value = { targetState, label, fingerprint: props.data.fingerprint }
}

function applyPreset(): void {
  if (!reviewedPreset.value) return
  emit("preset", {
    targetState: reviewedPreset.value.targetState,
    expectedFingerprint: reviewedPreset.value.fingerprint,
  })
  reviewedPreset.value = undefined
}
</script>

<template>
  <form class="shrine-progress-inspector" aria-label="Shrine progress inspector" @submit.prevent="apply">
    <header>
      <div><span>AUTHORITATIVE CURRENT GAME STATE</span><h3>Shrine Progress</h3></div>
      <small>{{ updatedAt ? `SYNC ${updatedAt.toLocaleTimeString([], { hour12: false })}` : 'WAITING' }}</small>
    </header>

    <div class="shrine-selector-row">
      <label for="shrine-progress-selector">Shrine</label>
      <select id="shrine-progress-selector" :value="selectedIndex" @change="selectShrine">
        <option v-for="shrine in data?.shrines ?? []" :key="shrine.index" :value="shrine.index">{{ shrine.virtueName }}</option>
      </select>
    </div>

    <template v-if="selected">
      <section>
        <h4>Current Live State</h4>
        <dl class="shrine-derived-state">
          <dt>Quest progression</dt><dd>{{ selected.questState.replaceAll('_', ' ').toLocaleUpperCase() }}</dd>
          <dt>Shrine condition</dt><dd>{{ selected.ruined ? 'RUINED' : 'RESTORED' }}</dd>
          <dt>Raw flags</dt><dd>completed={{ selected.completed }} · active={{ selected.active }} · ruined={{ selected.ruined }}</dd>
          <dt>First active shrine</dt><dd>{{ data?.firstActiveIndex === selected.index ? 'THIS SHRINE' : data?.firstActiveIndex === -1 ? 'NONE' : data?.shrines[data.firstActiveIndex]?.virtueName }}</dd>
          <dt>All completed</dt><dd>{{ data?.allCompleted ? 'YES' : 'NO' }}</dd>
        </dl>
      </section>

      <section>
        <h4>Alter Progression</h4>
        <div class="capped-control-row">
          <label for="shrine-quest-state">Quest state</label>
          <select id="shrine-quest-state" data-shrine-quest-state-select :value="questDraft" :class="{ dirty: questChanged }" @change="selectQuestState">
            <option v-for="choice in questChoices" :key="choice.value" :value="choice.value">{{ choice.label }}</option>
          </select>
        </div>
        <fieldset>
          <legend>Shrine condition</legend>
          <label><input type="radio" name="shrine-condition" value="false" data-shrine-condition="false" :checked="ruinedDraft === 'false'" @change="selectRuined('false')" /> Restored</label>
          <label><input type="radio" name="shrine-condition" value="true" data-shrine-condition="true" :checked="ruinedDraft === 'true'" @change="selectRuined('true')" /> Ruined</label>
        </fieldset>
      </section>

      <section class="shrine-presets">
        <h4>Reviewed All-Shrine Presets</h4>
        <p>These presets change quest flags for all eight shrines. Shrine condition and gameplay rewards are preserved.</p>
        <div>
          <button type="button" data-shrine-preset="not_assigned" :disabled="!data || pending || presetPending" @click="stagePreset('not_assigned', 'Clear All')">REVIEW: CLEAR ALL</button>
          <button type="button" data-shrine-preset="completed" :disabled="!data || pending || presetPending" @click="stagePreset('completed', 'Prepare Endgame')">REVIEW: PREPARE ENDGAME</button>
        </div>
      </section>

      <section v-if="reviewedPreset" class="shrine-preset-review locker-review" aria-live="polite">
        <div>
          <p>Apply <strong>{{ reviewedPreset.label }}</strong> to all {{ data?.shrines.length ?? 0 }} reviewed shrine quests.</p>
          <small>Target: {{ reviewedPreset.targetState.replaceAll('_', ' ') }}. The backend must match fingerprint {{ reviewedPreset.fingerprint }} before changing any shrine.</small>
        </div>
        <button type="button" data-shrine-preset-confirm :disabled="presetPending" @click="applyPreset">{{ presetPending ? 'APPLYING...' : `APPLY: ${reviewedPreset.label}` }}</button>
        <button type="button" :disabled="presetPending" @click="reviewedPreset = undefined">CANCEL</button>
      </section>

      <section class="shrine-pending-change">
        <h4>Pending Changes</h4>
        <dl v-if="changedCount">
          <template v-if="questChanged"><dt>Quest state</dt><dd>{{ selected.questState.replaceAll('_', ' ') }} → {{ questDraft.replaceAll('_', ' ') }}</dd></template>
          <template v-if="ruinedChanged"><dt>Condition</dt><dd>{{ selected.ruined ? 'Ruined' : 'Restored' }} → {{ ruinedDraft === 'true' ? 'Ruined' : 'Restored' }}</dd></template>
        </dl>
        <p v-else>No differences from current live state.</p>
        <p class="state-surgery-note">State surgery only. Applying does not award stats or karma and does not replay shrine presentation.</p>
      </section>

      <footer>
        <button type="submit" data-command-id="shrine.progress.update" :disabled="changedCount === 0 || pending || presetPending">{{ pending ? 'APPLYING...' : 'APPLY PROGRESS' }}</button>
        <button type="button" :disabled="changedCount === 0 || pending || presetPending" @click="revert">REVERT</button>
        <span>{{ changedCount ? `${changedCount} field${changedCount === 1 ? '' : 's'} changed` : 'No pending changes' }}</span>
      </footer>
    </template>

    <p v-else class="empty-state">WAITING FOR SHRINE STATE</p>
  </form>
</template>
