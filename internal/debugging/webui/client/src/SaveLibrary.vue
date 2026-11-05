<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { invalidateInstrumentReview, useInstrumentDraft } from "./instrument_session"

import { saveSlug, type SaveSnapshotDetail, type SaveSnapshotListData, type SaveSnapshotSummary } from "./saves"

type Owner = "personal" | "project"

const props = defineProps<{
  data?: SaveSnapshotListData
  detail?: SaveSnapshotDetail
  pendingList: boolean
  pendingDetail: boolean
  pendingMutation: boolean
  currentScene?: { locationName: string; floor: number; date: string; time: string; partyMembers: number }
}>()

const emit = defineEmits<{
  refresh: []
  inspect: [reference: string]
  capture: [value: { source: Owner; id: string; name: string; description: string }]
  load: [value: { reference: string; expectedRevision: string }]
  delete: [value: { reference: string; expectedRevision: string }]
}>()

const captureSource = useInstrumentDraft<Owner>("saves.source", "personal")
const captureName = useInstrumentDraft("saves.name", "")
const captureNote = useInstrumentDraft("saves.description", "")
const list = useInstrumentDraft<Owner>("saves.list", "personal")
const review = ref<{ kind: "load" | "delete"; reference: string }>()
const reviewedSnapshot = ref<SaveSnapshotDetail>()
const confirmation = ref("")
invalidateInstrumentReview(() => { review.value = undefined; reviewedSnapshot.value = undefined })

const captureID = computed(() => saveSlug(captureName.value))
const captureValid = computed(() => Boolean(captureName.value.trim() && captureID.value && props.data?.captureEligibility.canCapture))
const captureBlocked = computed(() => {
  if (!props.data) return "Waiting for the checkpoint list."
  if (!props.data.captureEligibility.canCapture) return props.data.captureEligibility.reason || "A checkpoint cannot be saved right now."
  if (!captureName.value.trim()) return "Give the checkpoint a name."
  if (!captureID.value) return "Use at least one letter or digit in the name."
  return ""
})
const visible = computed(() => (props.data?.snapshots ?? []).filter((snapshot) => snapshot.source === list.value))
const shared = computed(() => (props.data?.snapshots ?? []).filter((snapshot) => snapshot.source === "project").length)
const mine = computed(() => (props.data?.snapshots ?? []).filter((snapshot) => snapshot.source === "personal").length)
const reviewed = computed(() => reviewedSnapshot.value?.summary)
const deleteValid = computed(() => Boolean(reviewed.value && confirmation.value === reviewed.value.name && reviewed.value.eligibility.canDelete))

// The review shows the checkpoint exactly as it was fetched when the person asked for it.
watch(() => props.detail, (detail) => {
  const open = review.value
  if (!open || detail?.summary.reference !== open.reference) return
  if (!reviewedSnapshot.value) reviewedSnapshot.value = JSON.parse(JSON.stringify(detail)) as SaveSnapshotDetail
  else if (reviewedSnapshot.value.summary.revision !== detail.summary.revision) cancel()
}, { immediate: true })

function begin(kind: "load" | "delete", snapshot: SaveSnapshotSummary): void {
  review.value = { kind, reference: snapshot.reference }
  reviewedSnapshot.value = undefined
  confirmation.value = ""
  emit("inspect", snapshot.reference)
}

function cancel(): void {
  review.value = undefined
  reviewedSnapshot.value = undefined
  confirmation.value = ""
}

function submitCapture(): void {
  if (!captureValid.value || props.pendingMutation) return
  emit("capture", { source: captureSource.value, id: captureID.value, name: captureName.value.trim(), description: captureNote.value.trim() })
}

function confirmReview(): void {
  const snapshot = reviewed.value
  const open = review.value
  if (!snapshot || !open || props.pendingMutation) return
  if (open.kind === "load" && snapshot.eligibility.canLoad && snapshot.compatibility.compatible) emit("load", { reference: snapshot.reference, expectedRevision: snapshot.revision })
  else if (open.kind === "delete" && deleteValid.value) emit("delete", { reference: snapshot.reference, expectedRevision: snapshot.revision })
  else return
  cancel()
}

function clock(hour: number, minute: number): string {
  return `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`
}
</script>

<template>
  <section class="pg save-library" aria-label="Checkpoints" data-saves-workspace>
    <div class="pg-head">
      <div><h1>Checkpoints</h1><p>A checkpoint is a snapshot of the whole game at one moment. Checkpoints are kept apart from your save slots, and loading one never changes them.</p></div>
      <button type="button" class="pg-btn" :disabled="pendingList" data-save-action="refresh" @click="emit('refresh')">{{ pendingList ? "Refreshing…" : "Refresh list" }}</button>
    </div>

    <div class="pg-split">
      <section class="pg-card" aria-label="Save this moment">
        <h3>Save this moment</h3>
        <form class="pg-card" style="padding: 0; border: 0; background: none" @submit.prevent="submitCapture">
          <label class="pg-field">Name<input v-model="captureName" required placeholder="Before Dungeon Entry" data-save-name /></label>
          <label class="pg-field">Note (optional)<input v-model="captureNote" placeholder="What this checkpoint is for" /></label>
          <div class="pg-field" role="group" aria-label="Who can use it">
            Who can use it
            <div class="pg-chips">
              <button type="button" class="pg-chip" data-save-owner="personal" :aria-pressed="captureSource === 'personal'" @click="captureSource = 'personal'">Just me</button>
              <button type="button" class="pg-chip" data-save-owner="project" :aria-pressed="captureSource === 'project'" @click="captureSource = 'project'">Shared with the project</button>
            </div>
          </div>
          <button type="submit" class="pg-btn primary" :disabled="pendingMutation || !captureValid" data-save-action="capture">{{ pendingMutation ? "Working…" : "Save checkpoint" }}</button>
          <p v-if="captureBlocked" class="pg-note warn" role="status" data-save-blocked>{{ captureBlocked }}</p>
        </form>
      </section>

      <section class="pg-card" aria-label="Your checkpoints">
        <div class="pg-tabs" role="tablist" aria-label="Checkpoint lists">
          <button type="button" role="tab" data-save-list="personal" :aria-selected="list === 'personal'" @click="list = 'personal'">Mine ({{ mine }})</button>
          <button type="button" role="tab" data-save-list="project" :aria-selected="list === 'project'" @click="list = 'project'">Shared with the project ({{ shared }})</button>
        </div>
        <p v-if="visible.length === 0" class="pg-note" :data-save-source="list">No {{ list === 'personal' ? 'personal' : 'shared' }} checkpoints yet.</p>
        <ul class="pg-list">
          <li v-for="snapshot in visible" :key="snapshot.reference" :data-save-reference="snapshot.reference">
            <div class="pg-list-item" style="cursor: default">
              <span><b>{{ snapshot.name }}</b><small>{{ snapshot.description || "No note" }} · saved {{ new Date(snapshot.createdAt).toLocaleString() }}</small></span>
              <span class="pg-actions" style="flex-direction: row">
                <button type="button" class="pg-btn small" data-save-action="review-load" :disabled="pendingMutation || pendingDetail || !snapshot.eligibility.canLoad || !snapshot.compatibility.compatible" :title="snapshot.compatibility.compatible ? undefined : snapshot.compatibility.reason" @click="begin('load', snapshot)">Load…</button>
                <button type="button" class="pg-btn small warn" data-save-action="review-delete" :disabled="pendingMutation || pendingDetail || !snapshot.eligibility.canDelete" @click="begin('delete', snapshot)">Delete…</button>
              </span>
            </div>
          </li>
        </ul>
      </section>

      <section v-if="review" class="pg-card" :aria-label="review.kind === 'load' ? 'Load review' : 'Delete review'" :data-save-review="review.kind">
        <p v-if="!reviewedSnapshot" class="pg-note">Reading the checkpoint…</p>
        <template v-else-if="review.kind === 'load'">
          <h3>Load “{{ reviewedSnapshot.summary.name }}”?</h3>
          <dl class="pg-reviewgrid">
            <dt></dt><dd class="pg-muted">Now</dd><dd class="pg-muted">After loading</dd>
            <dt class="pg-muted">Where</dt><dd>{{ currentScene ? `${currentScene.locationName} · floor ${currentScene.floor}` : "Not known yet" }}</dd><dd class="after">{{ reviewedSnapshot.locationName }} · floor {{ reviewedSnapshot.floor }}</dd>
            <dt class="pg-muted">When</dt><dd>{{ currentScene ? `${currentScene.date} · ${currentScene.time}` : "Not known yet" }}</dd><dd class="after">Year {{ reviewedSnapshot.year }}, month {{ reviewedSnapshot.month }}, day {{ reviewedSnapshot.day }} · {{ clock(reviewedSnapshot.hour, reviewedSnapshot.minute) }}</dd>
            <dt class="pg-muted">Party</dt><dd>{{ currentScene ? `${currentScene.partyMembers} members` : "Not known yet" }}</dd><dd>{{ reviewedSnapshot.partyMembers }} members</dd>
          </dl>
          <p class="pg-note">Moon: {{ reviewedSnapshot.moonPhase }}. Compatibility: {{ reviewedSnapshot.summary.compatibility.compatible ? "compatible with this build" : (reviewedSnapshot.summary.compatibility.reason || "not compatible") }}.</p>
          <div class="pg-actions">
            <button type="button" class="pg-btn" @click="cancel">Cancel</button>
            <button type="button" class="pg-btn primary" data-save-action="confirm-load" :disabled="pendingMutation" @click="confirmReview">Load checkpoint</button>
          </div>
        </template>
        <template v-else>
          <h3>Delete “{{ reviewedSnapshot.summary.name }}”?</h3>
          <p class="pg-note">This removes the checkpoint for good. Type its name to confirm.</p>
          <label class="pg-field">Name<input v-model="confirmation" autocomplete="off" :placeholder="reviewedSnapshot.summary.name" /></label>
          <div class="pg-actions">
            <button type="button" class="pg-btn" @click="cancel">Cancel</button>
            <button type="button" class="pg-btn warn" data-save-action="confirm-delete" :disabled="!deleteValid || pendingMutation" @click="confirmReview">Delete checkpoint</button>
          </div>
        </template>
      </section>
    </div>
  </section>
</template>
