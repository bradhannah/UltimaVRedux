<script setup lang="ts">
import MoonPhaseGlyph from "./MoonPhaseGlyph.vue"
import type { TaskDefinition, TaskID } from "./workbench_tasks"

export interface TopBarClock {
  date: string
  time: string
  moonPhase: string
  moonPhaseIndex: number
}

export interface TopBarParty {
  count: number
  text: string
  needsAttention: boolean
}

defineProps<{
  connectionState: string
  connectionLabel: string
  sceneGeneration: number
  closeDetail: string
  where?: string
  clock?: TopBarClock
  party?: TopBarParty
  /** Undefined until combat status has been received. */
  mode?: "exploring" | "combat"
  tasks: TaskDefinition[]
  activeView: TaskID | "home"
  exitCombatDisabled: boolean
}>()

const emit = defineEmits<{
  openPalette: []
  selectView: [view: TaskID | "home"]
  exitCombat: []
  retry: []
}>()
</script>

<template>
  <header class="topbar">
    <div class="topbar-status">
      <strong class="topbar-brand">U5 Redux · Workbench</strong>
      <span class="connection" :class="connectionState" role="status"><i />{{ connectionLabel }}</span>
      <span v-if="where" class="topbar-fact" data-topbar-where><span>Where</span><b>{{ where }}</b></span>
      <span v-if="clock" class="topbar-fact topbar-when" data-topbar-when>
        <span>When</span><MoonPhaseGlyph :phase-index="clock.moonPhaseIndex" :label="clock.moonPhase" :size="16" /><b>{{ clock.date }} · {{ clock.time }}</b>
      </span>
      <span v-if="party" class="topbar-fact" data-topbar-party>
        <span>Party</span><b :class="{ attention: party.needsAttention }">{{ party.count }} · {{ party.text }}</b>
      </span>
      <span v-if="mode" class="mode-chip" :class="mode" data-topbar-mode>{{ mode === "combat" ? "In combat" : "Exploring" }}</span>
      <span class="topbar-scene">Scene {{ sceneGeneration || "--" }}</span>
      <small v-if="closeDetail" class="topbar-detail">{{ closeDetail }}</small>
      <span class="topbar-actions">
        <button v-if="connectionState === 'disconnected'" type="button" data-retry @click="emit('retry')">Retry now</button>
        <button
          v-if="mode === 'combat'"
          type="button"
          class="combat-exit-quick"
          data-quick-combat-exit
          :disabled="exitCombatDisabled"
          title="Discard the active debug encounter and return to its pre-combat location"
          @click="emit('exitCombat')"
        >Escape combat</button>
        <button type="button" class="command-palette-launcher" data-command-palette-launcher aria-haspopup="dialog" @click="emit('openPalette')">Search <kbd>Ctrl/⌘K</kbd></button>
      </span>
    </div>
    <nav class="task-tabs" aria-label="Workbench tasks">
      <button
        type="button"
        data-workspace="home"
        :class="{ selected: activeView === 'home' }"
        :aria-current="activeView === 'home' ? 'page' : undefined"
        @click="emit('selectView', 'home')"
      >Home</button>
      <button
        v-for="task in tasks"
        :key="task.id"
        type="button"
        :data-workspace="task.id"
        :class="{ selected: activeView === task.id, combat: task.id === 'fight' && mode === 'combat' }"
        :aria-current="activeView === task.id ? 'page' : undefined"
        @click="emit('selectView', task.id)"
      >{{ task.label }}<span v-if="task.id === 'fight' && mode === 'combat'" class="combat-marker" title="Combat in progress" aria-label="(combat in progress)">●</span></button>
    </nav>
  </header>
</template>
