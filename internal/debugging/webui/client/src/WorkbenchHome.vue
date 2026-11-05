<script setup lang="ts">
import type { NavigationSurface } from "./command_navigation"
import type { HomeCard, HomeQuickAction, TaskID } from "./workbench_tasks"

defineProps<{
  cards: HomeCard[]
  recents: NavigationSurface[]
  /** Outcome of the most recent Home quick action. */
  feedback?: { success: boolean; text: string }
}>()

const emit = defineEmits<{
  openTask: [task: TaskID]
  openSurface: [surface: NavigationSurface]
  quick: [action: HomeQuickAction]
}>()
</script>

<template>
  <section class="workbench-home" aria-labelledby="workbench-home-title" data-home-workspace>
    <header class="home-intro">
      <h1 id="workbench-home-title">What do you want to set up?</h1>
      <p>Pick a job below, or press <kbd>Ctrl/⌘K</kbd> and type what you want. Opening a tool never changes the game; changes need that tool's own buttons.</p>
    </header>

    <p v-if="feedback" class="home-feedback" :class="feedback.success ? 'success' : 'failure'" role="status" data-home-feedback>{{ feedback.text }}</p>

    <div class="home-cards">
      <section v-for="card in cards" :key="card.task.id" class="home-card" :data-home-task="card.task.id" :aria-labelledby="`home-card-${card.task.id}`">
        <div class="home-card-head">
          <svg class="home-card-icon" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path v-for="path in card.task.icon" :key="path" :d="path" />
          </svg>
          <div>
            <h2 :id="`home-card-${card.task.id}`">{{ card.task.label }}</h2>
            <p>{{ card.task.blurb }}</p>
          </div>
        </div>
        <div v-if="card.quick.length" class="home-chips" role="group" :aria-label="`${card.task.label} quick actions`">
          <button
            v-for="action in card.quick"
            :key="action.id"
            type="button"
            :data-home-quick="action.id"
            :disabled="action.disabled"
            :title="action.description"
            @click="emit('quick', action)"
          >{{ action.label }}</button>
        </div>
        <button type="button" class="home-open" :data-home-open="card.task.id" @click="emit('openTask', card.task.id)">Open {{ card.task.label }}</button>
      </section>
    </div>

    <section class="home-recents" aria-labelledby="home-recents-title">
      <header><h2 id="home-recents-title">Recent</h2><small>Tools you opened lately</small></header>
      <ul v-if="recents.length" class="home-recent-list">
        <li v-for="surface in recents" :key="surface.id">
          <button type="button" :data-home-recent="surface.id" @click="emit('openSurface', surface)">
            <strong>{{ surface.label }}</strong><span>{{ surface.workspaceLabel }}</span><small>{{ surface.description }}</small>
          </button>
        </li>
      </ul>
      <p v-else class="home-empty">Nothing yet. Tools you open will be listed here so you can jump back.</p>
    </section>
  </section>
</template>
