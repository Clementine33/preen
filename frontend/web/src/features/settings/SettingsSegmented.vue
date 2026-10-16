<script setup lang="ts">
/**
 * A one-of-few choice as a segmented row (`.seg`): the chosen value is a
 * hard-cut invert. For short labels only; choices that need a sentence each
 * use SettingsOptions.
 */
defineProps<{
  options: { value: string; label: string }[]
  value: string
  label: string
  disabled?: boolean
  mono?: boolean
}>()
defineEmits<{ change: [value: string] }>()
</script>

<template>
  <div class="seg" :class="{ mono }" role="group" :aria-label="label">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      :aria-pressed="option.value === value"
      :disabled="disabled"
      :data-value="option.value"
      @click="option.value !== value && $emit('change', option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>
