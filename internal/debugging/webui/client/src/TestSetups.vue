<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { invalidateInstrumentReview, useInstrumentDraft } from "./instrument_session"

import type { TestSetupStatusData } from "./approved_tool_views"

const props = defineProps<{
  data?: TestSetupStatusData
  updatedAt?: Date
  pendingStatus: boolean
  pendingHoleUp: boolean
  pendingKillParty: boolean
  pendingDeathSequence: boolean
}>()

const emit = defineEmits<{
  refresh: []
  run: [operation: { commandID: string; argumentsByID: Record<string, string> }]
}>()

const campingHours = useInstrumentDraft("test-setup.hours", 8)
const campingGuard = useInstrumentDraft("test-setup.guard", "")
const forceWanderer = useInstrumentDraft("test-setup.wanderer", false)
const forceAmbush = useInstrumentDraft("test-setup.ambush", false)
const armedDeath = ref<{ operation: "kill-party" | "death-sequence"; fingerprint: string }>()

function cancelReview(): void {
  armedDeath.value = undefined
}
invalidateInstrumentReview(cancelReview)
watch([() => props.data?.partyFingerprint, () => props.data?.wipeReview?.fingerprint], cancelReview, { flush: "sync" })

const blocked = computed(() => !props.data || props.data.combatActive || props.data.dialogActive
  || props.data.gameplayWorkflowActive || props.data.scriptedSceneActive || props.data.finaleActive || props.data.allPartyDead)

function stageDeath(operation: "kill-party" | "death-sequence"): void {
  if (armedDeath.value?.operation !== operation) {
    armedDeath.value = { operation, fingerprint: props.data?.wipeReview.fingerprint ?? "" }
    return
  }
  emit("run", {
    commandID: operation === "kill-party" ? "death.kill-party" : "death.test-sequence",
    argumentsByID: { expectedFingerprint: armedDeath.value.fingerprint },
  })
  armedDeath.value = undefined
}
</script>

<template>
  <section class="approved-view test-setups" aria-label="Test setups" data-test-setups-workspace @keydown.esc="cancelReview">
    <header class="approved-view-header">
      <div><span>REPEATABLE TEST WORKFLOWS</span><h3>Test Setups</h3></div>
      <button type="button" :disabled="pendingStatus" @click="emit('refresh')">{{ pendingStatus ? 'SYNCING...' : 'REFRESH' }}</button>
    </header>

    <section class="test-setups-body" data-test-setups-screen>
      <p class="test-setups-note">TEST SETUPS ARE BUILT-IN UTILITIES THAT ACT ON THE LIVE GAME. THEY ARE NOT SAVED OR SCRIPTED.</p>
      <section v-if="armedDeath" class="locker-review" data-quick-review>
        <p>REVIEWED {{ armedDeath.operation }}: PARTY {{ armedDeath.fingerprint }}</p>
        <button type="button" @click="cancelReview">CANCEL REVIEW</button>
      </section>
      <template v-if="data">
        <div class="test-setups-context">
          <div><span>LOCATION</span><strong>{{ data.locationName }}</strong><small>{{ data.mapType }} · FLOOR {{ data.floor }}</small></div>
          <div><span>TIME</span><strong>{{ data.time }}</strong><small>{{ data.date }}</small></div>
          <div><span>FOOD</span><strong>{{ data.food }}</strong><small>{{ data.allPartyDead ? 'PARTY DEAD' : 'PARTY ACTIVE' }}</small></div>
          <div><span>CONTEXT</span><strong>{{ blocked ? 'BLOCKED' : 'READY' }}</strong><small>{{ data.gameplayTerminal ? 'TERMINAL FINALE' : data.finaleActive ? 'FINALE HANDOFF' : data.combatActive ? 'COMBAT' : data.dialogActive ? 'DIALOG' : data.gameplayWorkflowActive ? 'WORKFLOW' : data.scriptedSceneActive ? 'SCRIPTED SCENE' : 'SAFE TO RUN' }}</small></div>
        </div>
        <section class="test-setups-party">
          <header><strong>ACTIVE PARTY</strong><small>{{ updatedAt ? `SYNC ${updatedAt.toLocaleTimeString([], { hour12: false })}` : 'WAITING' }}</small></header>
          <div v-for="member in data.party" :key="member.partyPosition" class="test-setups-party-row"><strong>P{{ member.partyPosition }} · {{ member.name }}</strong><span>HP {{ member.currentHp }}/{{ member.maximumHp }}</span><span>MP {{ member.currentMp }}</span><span>XP {{ member.experience }}</span><small>{{ member.status }}</small></div>
        </section>
        <div class="test-setups-grid">
          <section class="test-setups-card hole-up-card">
            <header><strong>HOLE UP</strong><small>PRODUCTION CAMPING WORKFLOW</small></header>
            <p>Queue camping with explicit encounter branches against this reviewed active party.</p>
            <label>HOURS<input v-model.number="campingHours" type="number" min="1" max="24" /></label>
            <label>GUARD<select v-model="campingGuard"><option value="">No guard</option><option v-for="member in data.party" :key="member.rosterIndex" :value="String(member.rosterIndex)">{{ member.name }}</option></select></label>
            <label><input v-model="forceWanderer" type="checkbox" /> FORCE WANDERER</label>
            <label><input v-model="forceAmbush" type="checkbox" /> FORCE AMBUSH</label>
            <button type="button" :disabled="blocked || pendingHoleUp || campingHours < 1 || campingHours > 24" @click="emit('run', { commandID: 'command-test.hole-up', argumentsByID: { hours: String(campingHours), forceWanderer: String(forceWanderer), forceAmbush: String(forceAmbush), expectedPartyFingerprint: data.partyFingerprint, ...(campingGuard ? { guardRosterIndex: campingGuard } : {}) } })">{{ pendingHoleUp ? 'QUEUEING...' : 'QUEUE CAMPING' }}</button>
          </section>
          <section class="test-setups-card danger-card death-wipe-card">
            <header><strong>KILL ACTIVE PARTY</strong><small>ATOMIC WIPE ONLY</small></header>
            <p>Set the reviewed active party to zero HP and Dead without starting resurrection.</p>
            <small>{{ data.wipeReview.members.length }} MEMBERS · REVIEW {{ data.wipeReview.fingerprint.slice(0, 12) }}</small>
            <button type="button" class="danger-button" :disabled="blocked || pendingKillParty || data.wipeReview.members.length === 0" @click="stageDeath('kill-party')">{{ pendingKillParty ? 'WIPING...' : armedDeath?.operation === 'kill-party' ? 'CONFIRM DESTRUCTIVE WIPE' : 'REVIEW PARTY WIPE' }}</button>
          </section>
          <section class="test-setups-card danger-card death-sequence-card">
            <header><strong>DEATH / RESURRECTION</strong><small>PRODUCTION WORKFLOW</small></header>
            <p>Atomically wipe the reviewed party, then queue the production resurrection workflow.</p>
            <small>{{ data.wipeReview.members.map((member) => `${member.name} ${member.hp} HP`).join(' · ') }}</small>
            <button type="button" class="danger-button" :disabled="blocked || pendingDeathSequence || data.wipeReview.members.length === 0" @click="stageDeath('death-sequence')">{{ pendingDeathSequence ? 'QUEUEING...' : armedDeath?.operation === 'death-sequence' ? 'CONFIRM WIPE + WORKFLOW' : 'REVIEW DEATH SEQUENCE' }}</button>
          </section>
        </div>
      </template>
      <p v-else class="empty-state">WAITING FOR TEST SETUP STATUS</p>
    </section>
  </section>
</template>
