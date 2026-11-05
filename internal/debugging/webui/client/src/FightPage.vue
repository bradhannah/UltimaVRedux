<script setup lang="ts">
import { computed, ref } from "vue"
import { useInstrumentDraft } from "./instrument_session"

import type { ActiveCombatStatusData, CombatEncounterOptionsData } from "./approved_tool_views"
import CombatEncounterComposer from "./CombatEncounterComposer.vue"
import type { PageStep } from "./page_steps"
import PreferenceNumberControl from "./PreferenceNumberControl.vue"
import type { CommandDefinition } from "./protocol"

type Monster = ActiveCombatStatusData["monsters"][number]

const props = defineProps<{
  combat?: ActiveCombatStatusData
  options?: CombatEncounterOptionsData
  commands: CommandDefinition[]
  busy: boolean
  startPending: boolean
  preselect?: { dungeon: number; room: number; token: number }
}>()

const emit = defineEmits<{ apply: [steps: PageStep[]] }>()

const spawnOpen = ref(false)
const spawnType = useInstrumentDraft("fight.spawnType", "")
const spawnMode = useInstrumentDraft("fight.spawnMode", "automatic")
const spawnCount = useInstrumentDraft("fight.spawnCount", 1)
const spawnFriends = useInstrumentDraft("fight.spawnFriends", true)
const spawnSeed = useInstrumentDraft("fight.spawnSeed", "")
const rngSeed = useInstrumentDraft("fight.seed", 42)

const active = computed(() => props.combat?.active === true)
const has = (commandID: string): boolean => props.commands.some((command) => command.id === commandID)
const blocked = computed(() => {
  const data = props.combat
  if (!data?.active) return ""
  if (data.workflowActive) return "The game is in the middle of something (a message or an animation). Wait for it to finish."
  if (data.map?.pendingAdvanceTurn || data.map?.activeMonsterIndex !== -1) return "A monster is acting right now. Try again in a moment."
  return ""
})
const disabled = computed(() => props.busy || Boolean(blocked.value))
const living = computed(() => (props.combat?.monsters ?? []).filter((monster) => !monster.dead))
const hiddenCount = computed(() => living.value.filter((monster) => monster.invisible).length)
const turnName = computed(() => props.combat?.party.find((member) => member.currentActor)?.name
  ?? props.combat?.monsters.find((monster) => monster.currentActor)?.name ?? "")
const title = computed(() => {
  const map = props.combat?.map
  if (!map) return "Fight"
  const room = props.combat?.fixedDungeonRoom ? ` · Room ${props.combat.dungeonRoom}` : ""
  return `Fight · ${map.name}${room}`
})
const cells = computed(() => (props.combat?.map?.cells ?? []).map((cell) => {
  const monster = cell.occupantKind === "monster" ? props.combat?.monsters.find((entry) => entry.id === cell.occupantId) : undefined
  const member = cell.occupantKind === "party" ? props.combat?.party.find((entry) => String(entry.index) === cell.occupantId) : undefined
  return {
    key: `${cell.x},${cell.y}`,
    label: member ? `P${member.index + 1}` : monster ? "M" : "",
    classes: { party: Boolean(member), monster: Boolean(monster), current: Boolean(member?.currentActor || monster?.currentActor) },
    title: cell.occupantName ? `${cell.x},${cell.y} · ${cell.occupantName}` : `${cell.x},${cell.y}`,
  }
}))

function base(): Record<string, string> {
  return { encounterId: props.combat?.encounterId ?? "", expectedSnapshot: props.combat?.snapshotFingerprint ?? "" }
}
function run(commandID: string, extra: Record<string, string> = {}): void {
  if (disabled.value || !props.combat?.encounterId) return
  emit("apply", [{ commandID, argumentsByID: { ...base(), ...extra } }])
}
function hurt(monster: Monster): void {
  run("combat.monster.damage-raw", { monsterId: monster.id, expectedCurrentHp: String(monster.currentHp), damage: String(Math.min(10, Math.max(1, monster.currentHp))) })
}
function defeat(monster: Monster): void {
  run("combat.monster.kill", { monsterId: monster.id, expectedCurrentHp: String(monster.currentHp) })
}
function toggleSleep(monster: Monster): void {
  run("combat.monster.sleep.set", { monsterId: monster.id, asleep: String(!monster.asleep), expectedAsleep: String(monster.asleep) })
}
const seedValid = computed(() => Number.isInteger(rngSeed.value) && rngSeed.value >= 1 && rngSeed.value <= 999999)
const spawnValid = computed(() => (spawnType.value || props.combat?.monsterTypes[0]) && (spawnMode.value !== "exact" || (spawnCount.value >= 1 && spawnCount.value <= 16)))
function spawn(): void {
  if (!spawnValid.value) return
  const type = spawnType.value || String(props.combat?.monsterTypes[0]?.id ?? "")
  run("combat.monster.spawn", {
    monsterType: type, countMode: spawnMode.value, friends: String(spawnFriends.value),
    ...(spawnMode.value === "exact" ? { exactCount: String(spawnCount.value) } : {}),
    ...(spawnSeed.value.trim() ? { seed: spawnSeed.value.trim() } : {}),
  })
  spawnOpen.value = false
}
</script>

<template>
  <section class="pg fight-page" aria-label="Fight" data-fight-page>
    <p v-if="!combat" class="pg-note">Waiting for the combat status.</p>

    <template v-else-if="!active">
      <div class="pg-head"><div><h1>Fight</h1><p>{{ combat.unavailableReason || "No battle is under way. Compose an encounter and start it." }}</p></div></div>
      <CombatEncounterComposer :options="options" :pending="startPending" :preselect="preselect" @start="(argumentsByID) => emit('apply', [{ commandID: 'combat.encounter.start', argumentsByID }])" />
    </template>

    <template v-else>
      <div class="pg-head">
        <div>
          <h1 data-fight-title>{{ title }}</h1>
          <p>{{ combat.map?.victory ? "Victory declared." : turnName ? `Turn: ${turnName} · ` : "" }}{{ living.length }} monsters left{{ hiddenCount ? `, ${hiddenCount} hidden` : "" }}</p>
        </div>
        <div class="pg-actions">
          <button type="button" class="pg-btn" data-fight-win :disabled="disabled || combat.activeMonsterCount === 0" @click="run('combat.monster.kill-all')">Win now</button>
          <button v-if="has('combat.monster.sleep-all')" type="button" class="pg-btn" data-fight-sleep-all :disabled="disabled || living.length === 0" @click="run('combat.monster.sleep-all')">Put all to sleep</button>
          <button type="button" class="pg-btn warn" data-fight-leave :disabled="disabled" @click="run('combat.exit-debug')">Leave combat</button>
        </div>
      </div>
      <p v-if="blocked" class="pg-note warn" role="status" data-fight-blocked>{{ blocked }}</p>

      <div class="pg-split">
        <section class="pg-card" aria-label="Battlefield" style="flex: 0 1 420px">
          <h3>Battlefield</h3>
          <div class="pg-battle" role="img" aria-label="Eleven by eleven battlefield">
            <span v-for="cell in cells" :key="cell.key" :class="cell.classes" :title="cell.title">{{ cell.label }}</span>
          </div>
          <p class="pg-note">P1-P6 are the party, M a monster, yellow outline whoever acts now.</p>
        </section>

        <section class="pg-card" aria-label="Monsters" style="flex: 999 1 520px; padding: 0">
          <div class="pg-actions" style="justify-content: space-between; padding: 12px 14px 0">
            <h3>Monsters</h3>
            <button type="button" class="pg-btn small" data-fight-add-toggle :aria-expanded="spawnOpen" :disabled="disabled" @click="spawnOpen = !spawnOpen">+ Add monsters</button>
          </div>
          <form v-if="spawnOpen" class="pg-card" style="margin: 0 14px" data-fight-spawn @submit.prevent="spawn">
            <div class="pg-field-row">
              <label class="pg-field">Monster<select :value="spawnType || String(combat.monsterTypes[0]?.id ?? '')" @change="spawnType = ($event.target as HTMLSelectElement).value"><option v-for="type in combat.monsterTypes" :key="type.id" :value="String(type.id)">{{ type.name }}</option></select></label>
              <label class="pg-field">How many<select v-model="spawnMode"><option value="automatic">Automatic</option><option value="maximum">As many as fit</option><option value="exact">An exact number</option></select></label>
              <label v-if="spawnMode === 'exact'" class="pg-field">Number<input v-model.number="spawnCount" type="number" min="1" max="16" /></label>
            </div>
            <label class="pg-field" style="flex-direction: row; align-items: center; gap: 8px"><input v-model="spawnFriends" type="checkbox" /> Include the monster's usual friends</label>
            <label class="pg-field">Repeatable dice seed (optional)<input v-model="spawnSeed" type="number" min="0" max="2147483647" placeholder="game's own dice" data-fight-spawn-seed /></label>
            <div class="pg-actions"><button type="submit" class="pg-btn primary" data-fight-spawn-apply :disabled="disabled || !spawnValid">Add them</button></div>
          </form>
          <p v-if="combat.monsters.length === 0" class="pg-note" style="padding: 0 14px">No monsters in this fight.</p>
          <div v-for="monster in combat.monsters" :key="monster.id" class="pg-monster" :class="{ defeated: monster.dead }" :data-fight-monster="monster.id">
            <span><b>{{ monster.name }}</b><small>{{ monster.dead ? "Defeated" : `${monster.invisible ? "Hidden · " : ""}${monster.asleep ? "Asleep · " : ""}${monster.poisoned ? "Poisoned · " : ""}cell ${monster.x},${monster.y}` }}</small></span>
            <span><i class="pg-meter" :class="{ low: monster.currentHp * 2 < monster.maximumHp }" aria-hidden="true"><i :style="{ width: `${monster.maximumHp ? Math.round(100 * monster.currentHp / monster.maximumHp) : 0}%` }" /></i><small>{{ monster.currentHp }} / {{ monster.maximumHp }} HP</small></span>
            <span class="pg-actions">
              <button type="button" class="pg-btn small" data-monster-hurt :disabled="disabled || monster.dead" @click="hurt(monster)">−10 HP</button>
              <button type="button" class="pg-btn small" data-monster-sleep :disabled="disabled || monster.dead" @click="toggleSleep(monster)">{{ monster.asleep ? "Wake" : "Sleep" }}</button>
              <button type="button" class="pg-btn small warn" data-monster-defeat :disabled="disabled || monster.dead" @click="defeat(monster)">Defeat</button>
            </span>
          </div>
          <p class="pg-note" style="padding: 0 14px">Defeating a monster this way gives no experience and leaves no corpse or treasure.</p>
          <details style="padding: 0 14px 14px">
            <summary class="pg-muted">Advanced</summary>
            <div class="pg-card" style="margin-top: 10px">
              <div class="pg-with-button">
                <label class="pg-field">Repeatable dice seed (now {{ combat.rngSeed }}; changes the game's randomness afterwards too)<input v-model.number="rngSeed" type="number" min="1" max="999999" :aria-invalid="!seedValid" data-fight-seed /></label>
                <button type="button" class="pg-btn" data-fight-seed-set :disabled="disabled || !seedValid" @click="run('rng.seed.set', { seed: String(rngSeed) })">Set</button>
              </div>
              <PreferenceNumberControl command-id="combat.presentation.speed.set" :command="commands.find((command) => command.id === 'combat.presentation.speed.set')" :value="combat.presentationSpeedPercent" :disabled="busy || combat.configurationEditing" @apply="(argumentsByID) => emit('apply', [{ commandID: 'combat.presentation.speed.set', argumentsByID }])" />
              <div class="pg-actions">
                <button type="button" class="pg-btn" data-fight-clear-room :disabled="disabled || !combat.fixedDungeonRoom || combat.map?.victory || combat.activeHostileMonsterCount === 0" :title="combat.fixedDungeonRoom ? undefined : 'Only for a fixed dungeon room'" @click="run('combat.dungeon-room.clear')">Clear this room (victory, no rewards)</button>
                <button type="button" class="pg-btn" data-fight-clear :disabled="disabled || combat.totalMonsterCount === 0" @click="run('combat.monster.clear')">Remove every monster (no victory)</button>
              </div>
            </div>
          </details>
        </section>
      </div>
    </template>
  </section>
</template>
