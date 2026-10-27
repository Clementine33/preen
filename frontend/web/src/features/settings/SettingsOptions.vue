<script setup lang="ts">
/**
 * A one-of-few choice where each option needs its consequence spelled out:
 * native radios under square option cards (`.opt`). A `danger` option marks
 * its checked state in the danger ink.
 */
defineProps<{
  options: { value: string, label: string, hint?: string, danger?: boolean }[]
  value: string
  label: string
  /** The radios' group name; unique on the page. */
  name: string
  disabled?: boolean
}>()
defineEmits<{ change: [value: string] }>()
</script>

<template>
  <div class="opts" role="radiogroup" :aria-label="label">
    <label v-for="option in options" :key="option.value" class="opt" :class="{ danger: option.danger }">
      <input
        type="radio"
        :name="name"
        :value="option.value"
        :checked="option.value === value"
        :disabled="disabled"
        :data-testid="`${name}-${option.value}`"
        @change="$emit('change', option.value)"
      >
      <span class="box" aria-hidden="true" />
      <span>
        <span class="opt-t">{{ option.label }}</span>
        <span v-if="option.hint" class="opt-d">{{ option.hint }}</span>
      </span>
    </label>
  </div>
</template>
