<script setup lang="ts">
import type { NavigationSurface } from "./command_navigation"

defineProps<{
  recents: NavigationSurface[]
}>()

const emit = defineEmits<{
  openPalette: []
  openSurface: [surface: NavigationSurface]
}>()
</script>

<template>
  <section class="workbench-home" aria-labelledby="workbench-home-title" data-home-workspace>
    <header class="home-intro">
      <div><span>BRITANNIAN SWITCHBOARD</span><h2 id="workbench-home-title">Workbench Home</h2><p>Choose a recent surface or search the control plane. Navigation never runs a mutation.</p></div>
      <button type="button" data-home-go-to @click="emit('openPalette')">GO TO A SURFACE… <kbd>CTRL/⌘K</kbd></button>
    </header>

    <div class="home-grid">
      <section class="home-panel home-recents" aria-labelledby="home-recents-title">
        <header><h3 id="home-recents-title">Recent Surfaces</h3><small>{{ recents.length }} / 8</small></header>
        <div v-if="recents.length" class="home-recent-list">
          <button v-for="surface in recents" :key="surface.id" type="button" :data-home-recent="surface.id" @click="emit('openSurface', surface)">
            <span>{{ surface.workspaceLabel }}</span><strong>{{ surface.label }}</strong><small>{{ surface.description }}</small><i aria-hidden="true">→</i>
          </button>
        </div>
        <div v-else class="home-empty"><strong>NO RECENT SURFACES</strong><p>Use Go To or choose a workspace above to begin.</p></div>
      </section>

      <div class="home-side-column">
        <section class="home-panel home-getting-started" aria-labelledby="home-start-title">
          <header><h3 id="home-start-title">Getting Started</h3></header>
          <p><kbd>CTRL/⌘K</kbd> searches every Workbench surface.</p>
          <p>Opening a surface may load read-only telemetry. Mutations still require their normal controls and review.</p>
        </section>
      </div>
    </div>
  </section>
</template>
