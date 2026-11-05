<script setup lang="ts">
import { computed } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type { CalendarStatusData, MoongateStatusData } from "./approved_tool_views"
import MoonPhaseGlyph from "./MoonPhaseGlyph.vue"
import PageChangeBar from "./PageChangeBar.vue"
import type { PageStep } from "./page_steps"

const props = defineProps<{
  calendar?: CalendarStatusData
  windDirection?: string
  moongate?: MoongateStatusData
  busy: boolean
  travelPending: boolean
  overlayPending: boolean
}>()

const emit = defineEmits<{ apply: [steps: PageStep[]]; travel: [step: PageStep]; overlay: [enabled: boolean] }>()

// Reasons come from the game's own daylight boundaries (night 20:00-05:00, dawn until 07:00, twilight from 17:00).
const timePresets = [
  { hour: 5, minute: 0, label: "05:00", why: "Dawn · night ends" },
  { hour: 8, minute: 0, label: "08:00", why: "Morning · full daylight" },
  { hour: 12, minute: 0, label: "12:00", why: "Noon · brightest sun" },
  { hour: 17, minute: 0, label: "17:00", why: "Evening · twilight begins" },
  { hour: 20, minute: 0, label: "20:00", why: "Dusk · night begins" },
  { hour: 0, minute: 0, label: "00:00", why: "Midnight · deepest night" },
]
const winds = ["Calm", "North", "East", "South", "West"]
const phaseNames = ["New Moon", "Crescent Waxing", "First Quarter", "Gibbous Waxing", "Full Moon", "Gibbous Waning", "Last Quarter", "Crescent Waning"]

const hourEdit = useInstrumentDraft<number | undefined>("time.hour", undefined)
const minuteEdit = useInstrumentDraft<number | undefined>("time.minute", undefined)
const dayEdit = useInstrumentDraft<number | undefined>("time.day", undefined)
const windEdit = useInstrumentDraft<string | undefined>("time.wind", undefined)

const hour = computed(() => hourEdit.value ?? props.calendar?.hour ?? 0)
const minute = computed(() => minuteEdit.value ?? props.calendar?.minute ?? 0)
const day = computed(() => dayEdit.value ?? props.calendar?.day ?? 1)
const hourValid = computed(() => Number.isInteger(hour.value) && hour.value >= 0 && hour.value <= 23)
const minuteValid = computed(() => Number.isInteger(minute.value) && minute.value >= 0 && minute.value <= 59)
const dayValid = computed(() => Number.isInteger(day.value) && day.value >= 1 && day.value <= 28)
const timeChanged = computed(() => Boolean(props.calendar) && (hour.value !== props.calendar?.hour || minute.value !== props.calendar?.minute))
const dayChanged = computed(() => Boolean(props.calendar) && day.value !== props.calendar?.day)
const windChanged = computed(() => windEdit.value !== undefined && windEdit.value.toLocaleLowerCase() !== (props.windDirection ?? "").toLocaleLowerCase())
const currentWind = computed(() => windEdit.value ?? props.windDirection ?? "")

function clock(h: number, m: number): string {
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`
}
const daylight = computed(() => {
  const minutes = (props.calendar?.hour ?? 0) * 60 + (props.calendar?.minute ?? 0)
  if (minutes < 300 || minutes >= 1200) return "Night"
  if (minutes < 420) return "Dawn"
  if (minutes < 1020) return "Daylight"
  return "Twilight"
})

const changes = computed(() => {
  const parts: string[] = []
  if (timeChanged.value) parts.push(`time → ${clock(hour.value, minute.value)}`)
  if (dayChanged.value) parts.push(`day → ${day.value}`)
  if (windChanged.value) parts.push(`wind → ${windEdit.value}`)
  return parts
})
const invalidReason = computed(() => {
  if (!hourValid.value || !minuteValid.value) return "Hour must be 0-23 and minute 0-59."
  if (!dayValid.value) return "Day must be 1-28."
  return undefined
})

function discard(): void {
  hourEdit.value = undefined
  minuteEdit.value = undefined
  dayEdit.value = undefined
  windEdit.value = undefined
}

function applyAll(): void {
  if (invalidReason.value) return
  const steps: PageStep[] = []
  if (timeChanged.value) steps.push({ commandID: "calendar.set-time", argumentsByID: { hour: String(hour.value), minute: String(minute.value) } })
  if (dayChanged.value) steps.push({ commandID: "calendar.set-day", argumentsByID: { day: String(day.value) } })
  if (windChanged.value && windEdit.value) steps.push({ commandID: "environment.wind.set", argumentsByID: { direction: windEdit.value.toLocaleLowerCase() } })
  if (steps.length === 0) return
  discard()
  emit("apply", steps)
}

function numberInput(event: Event): number {
  return (event.target as HTMLInputElement).valueAsNumber
}

const gateDescription = computed(() => {
  const gate = props.moongate
  if (!gate) return ""
  if (gate.teleportationActive) return "A gate trip is already under way."
  if (gate.route?.kind === "moonstone" && gate.route.destination) return `For this phase the gates lead to ${gate.route.destination.locationName} (${gate.route.destination.x}, ${gate.route.destination.y}).`
  if (gate.route?.kind === "spirituality") return `For this phase the gate leads to the Shrine of ${gate.route.shrineName ?? "Spirituality"}.`
  return gate.unavailableReason ?? "No destination resolves for the current phase."
})
const travelBlocked = computed(() => {
  const gate = props.moongate
  if (!gate) return "Waiting for moongate status."
  if (gate.teleportationActive) return "A gate trip is already under way."
  if (!gate.playerOnActiveGate) return "Stand on a visible moongate to travel."
  if (!gate.route) return gate.unavailableReason ?? "No destination resolves for this phase."
  return ""
})

function travel(): void {
  const gate = props.moongate
  if (!gate?.route || travelBlocked.value) return
  emit("travel", { commandID: "moongate.travel", argumentsByID: { expectedSourceFingerprint: gate.sourceFingerprint, expectedRouteFingerprint: gate.route.fingerprint } })
}
</script>

<template>
  <section class="pg time-page" aria-label="Time and sky" data-time-page>
    <div class="pg-head"><h1>Time &amp; sky</h1></div>
    <p v-if="!calendar" class="pg-note">Waiting for the calendar.</p>
    <div class="pg-grid">
      <section class="pg-card" aria-labelledby="time-of-day-title">
        <h3 id="time-of-day-title">Time of day</h3>
        <p v-if="calendar" class="pg-muted" data-time-now>Now <b>{{ calendar.time }}</b> · {{ daylight }} · {{ calendar.date }}</p>
        <div class="pg-tiles" role="group" aria-label="Time presets">
          <button v-for="preset in timePresets" :key="preset.label" type="button" class="pg-tile" :data-time-preset="preset.label" :aria-pressed="hour === preset.hour && minute === preset.minute" :disabled="!calendar" @click="hourEdit = preset.hour; minuteEdit = preset.minute">
            <b>{{ preset.label }}</b><small>{{ preset.why }}</small>
          </button>
        </div>
        <div class="pg-field-row">
          <label class="pg-field">Hour<input type="number" min="0" max="23" :value="hour" :aria-invalid="!hourValid" data-time-hour @input="hourEdit = numberInput($event)" /></label>
          <label class="pg-field">Minute<input type="number" min="0" max="59" :value="minute" :aria-invalid="!minuteValid" data-time-minute @input="minuteEdit = numberInput($event)" /></label>
          <label class="pg-field">Day of month (1-28)<input type="number" min="1" max="28" :value="day" :aria-invalid="!dayValid" data-time-day @input="dayEdit = numberInput($event)" /></label>
        </div>
        <p v-if="calendar" class="pg-note">Month {{ calendar.month }}, year {{ calendar.year }} (read only: no command changes them).</p>
      </section>

      <section class="pg-card" aria-labelledby="sky-title">
        <h3 id="sky-title">Moon and moongates</h3>
        <div v-if="calendar" class="pg-chips" role="img" :aria-label="`Moon phase: ${calendar.moonPhase}`" data-moon-rail>
          <span v-for="(name, index) in phaseNames" :key="name" class="pg-chip" :aria-pressed="index === calendar.moonPhaseIndex" :title="name" style="cursor: default; display: inline-flex; align-items: center; gap: 6px"><MoonPhaseGlyph :phase-index="index" :label="name" :size="20" />{{ index }}</span>
        </div>
        <p v-if="calendar" class="pg-muted">Phase {{ calendar.moonPhaseIndex }}: {{ calendar.moonPhase }}. The phase follows the calendar day, so it moves when you change the day (read only).</p>
        <template v-if="moongate">
          <p class="pg-sentence" data-gate-sentence>{{ moongate.visible ? "A moongate is showing." : "No moongate is showing." }} {{ gateDescription }}</p>
          <div class="pg-actions">
            <button type="button" class="pg-btn" data-gate-travel :disabled="busy || travelPending || Boolean(travelBlocked)" @click="travel">{{ travelPending ? "Queueing…" : "Travel through the gate" }}</button>
            <button type="button" class="pg-btn" data-gate-overlay :disabled="overlayPending" @click="emit('overlay', !moongate.overlayEnabled)">{{ moongate.overlayEnabled ? "Hide moonstone marks" : "Show moonstone marks" }}</button>
          </div>
          <p v-if="travelBlocked" class="pg-note warn" role="status">{{ travelBlocked }}</p>
          <details>
            <summary class="pg-muted">Moonstones ({{ moongate.moonstones.length }})</summary>
            <table class="pg-note"><thead><tr><th scope="col">Phase</th><th scope="col">Cell</th><th scope="col">Map / level</th><th scope="col">State</th><th scope="col">Found</th></tr></thead>
              <tbody><tr v-for="stone in moongate.moonstones" :key="stone.index"><td>{{ stone.phase }}</td><td>{{ stone.x }}, {{ stone.y }}</td><td>{{ stone.map }} / {{ stone.level }}</td><td>{{ stone.state }}</td><td>{{ stone.found ? "yes" : "no" }}</td></tr></tbody></table>
          </details>
        </template>
        <div class="pg-field" role="group" aria-label="Wind direction">
          Wind (for sailing)
          <div class="pg-chips">
            <button v-for="wind in winds" :key="wind" type="button" class="pg-chip" :data-wind="wind.toLocaleLowerCase()" :aria-pressed="currentWind.toLocaleLowerCase() === wind.toLocaleLowerCase()" :disabled="!windDirection" @click="windEdit = wind">{{ wind }}</button>
          </div>
        </div>
      </section>
    </div>
    <PageChangeBar :count="changes.length" :summary="changes.join(' · ')" :busy="busy" :blocked="invalidReason" @discard="discard" @apply="applyAll" />
  </section>
</template>
