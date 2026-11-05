<script setup lang="ts">
import { plural } from "./page_steps"

defineProps<{
  count: number
  summary: string
  busy: boolean
  /** Why Apply is unavailable although changes are staged. */
  blocked?: string
}>()

const emit = defineEmits<{ discard: []; apply: [] }>()
</script>

<template>
  <div v-if="count > 0" class="pg-bar" role="region" aria-label="Pending changes" data-change-bar>
    <span class="pg-bar-text"><b>{{ plural(count, "change") }}:</b> {{ summary }}<small v-if="blocked" role="status">{{ blocked }}</small></span>
    <span class="pg-actions">
      <button type="button" class="pg-btn" data-change-discard :disabled="busy" @click="emit('discard')">Discard</button>
      <button type="button" class="pg-btn primary" data-change-apply :disabled="busy || Boolean(blocked)" @click="emit('apply')">{{ busy ? "Applying…" : `Apply ${plural(count, "change")}` }}</button>
    </span>
  </div>
</template>
