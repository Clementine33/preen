<script setup lang="ts">
/** An on/off setting as a square switch (`.switch`) over a native checkbox. */
defineProps<{ checked: boolean; disabled?: boolean; testid?: string }>()
defineEmits<{ change: [checked: boolean] }>()
</script>

<template>
  <label class="switch">
    <input
      type="checkbox"
      :checked="checked"
      :disabled="disabled"
      :data-testid="testid"
      @change="$emit('change', ($event.target as HTMLInputElement).checked)"
    />
    <span class="track" aria-hidden="true" />
    <span><slot /></span>
  </label>
</template>
