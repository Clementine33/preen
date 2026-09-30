<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import WorkInspector from '@/features/works/WorkInspector.vue'

// One work's detail page (ADR 0012 §4): the mobile tap target and OPEN DETAIL.
// The prototype covers no detail screen, so this is deliberately minimal: the
// inspector itself, centered as a specimen on the substrate, with the way back
// to the wall carrying the `?rj=` identity so the selection survives the jump.
const route = useRoute()
const router = useRouter()

const rj = computed(() => (typeof route.params.rj === 'string' && route.params.rj !== '' ? route.params.rj : null))

function backToWall(): void {
  router.push({ name: 'works', query: rj.value !== null ? { rj: rj.value } : {} })
}
</script>

<template>
  <section class="detail substrate" aria-label="作品详情">
    <div class="frame">
      <div class="head">
        <GlobalMenu />
        <button class="btn back" type="button" @click="backToWall">BACK TO WALL</button>
      </div>
      <WorkInspector :rj="rj" standalone />
    </div>
  </section>
</template>

<style scoped>
.detail {
  height: 100%; overflow-y: auto; min-height: 0;
  display: flex; justify-content: center;
  padding: var(--sp-6) var(--sp-4) var(--sp-6);
}
.frame {
  width: min(100%, 608px);
  display: flex; flex-direction: column; gap: var(--sp-4);
}
.head { display: flex; align-items: center; gap: var(--sp-2); }
.frame > :deep(.insp) {
  border-left: none; padding: 0;
  background: var(--grain), var(--bg-panel);
  border: 1px solid var(--wire-strong);
  box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid);
}
</style>
