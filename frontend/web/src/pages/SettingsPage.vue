<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import SettingsField from '@/features/settings/SettingsField.vue'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import UserTagSettings from '@/features/settings/UserTagSettings.vue'
import { MOTION_OPTIONS, useMotion, type MotionPreference } from '@/composables/use-motion'
import { THEME_OPTIONS, useTheme, type Theme } from '@/composables/use-theme'

/**
 * 设置 (`/settings/:section?`): what has no other home, one category per
 * address — this browser's appearance (theme, motion) and the user-tag
 * vocabulary. Each saves where its data lives: tags on the backend, appearance
 * in this browser. Task settings stay beside their tasks, a library's storage
 * class in its edit dialog.
 */
const SECTIONS = [
  { id: 'appearance', label: '外观', code: 'APPEARANCE' },
  { id: 'tags', label: '用户标签', code: 'USER TAGS' },
] as const

const route = useRoute()
const section = computed(() => SECTIONS.find((item) => item.id === route.params.section) ?? SECTIONS[0])

const { theme } = useTheme()
const { preference, systemReduces } = useMotion()
</script>

<template>
  <section class="flex h-full min-w-0 flex-col substrate" data-testid="settings-page">
    <header class="flex min-h-14 shrink-0 items-center gap-3 edge-bottom px-4 py-1.5 rail:px-5">
      <GlobalMenu />
      <h1 class="font-heading text-sm font-semibold">设置</h1>
    </header>

    <div class="flex min-h-0 flex-1 flex-col rail:flex-row">
      <nav
        aria-label="设置分类"
        class="flex shrink-0 gap-1 border-b border-[var(--wire-fine)] substrate-panel px-4 py-1.5 rail:w-[200px] rail:flex-col rail:border-r rail:border-b-0 rail:border-border rail:px-2 rail:py-3"
      >
        <RouterLink
          v-for="item in SECTIONS"
          :key="item.id"
          :to="{ name: 'settings', params: { section: item.id } }"
          :aria-current="section.id === item.id ? 'page' : undefined"
          class="flex min-h-11 items-center px-3 text-xs font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none rail:min-h-8"
          :class="section.id === item.id ? 'bg-primary text-primary-foreground' : 'text-[var(--text-secondary)] hover:bg-accent hover:text-foreground'"
          :data-testid="`settings-nav-${item.id}`"
        >
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <section
          class="grid max-w-3xl grid-cols-[minmax(0,1fr)] px-4 pt-6 pb-16 rail:px-8"
          aria-labelledby="settings-section-title"
        >
          <div class="strip"><span class="strip-name lbl">{{ section.code }}</span></div>
          <h2 id="settings-section-title" class="mt-3 font-heading text-base font-semibold">{{ section.label }}</h2>

          <template v-if="section.id === 'appearance'">
            <p class="mt-1 mb-1 max-w-[38em] text-[length:var(--fs-m)] leading-relaxed text-[var(--text-secondary)]">
              只影响这个浏览器，选择后立即生效。
            </p>
            <SettingsField id="theme" label="主题">
              <SettingsSegmented
                label="主题"
                :options="[...THEME_OPTIONS]"
                :value="theme"
                data-testid="theme-preference"
                @change="theme = $event as Theme"
              />
            </SettingsField>
            <SettingsField id="motion" label="界面动效" :hint="systemReduces ? '系统已要求减少动效，界面始终减少。' : undefined">
              <SettingsSegmented
                label="界面动效"
                :options="[...MOTION_OPTIONS]"
                :value="preference"
                data-testid="motion-preference"
                @change="preference = $event as MotionPreference"
              />
            </SettingsField>
          </template>

          <template v-else>
            <p class="mt-1 mb-3 max-w-[38em] text-[length:var(--fs-m)] leading-relaxed text-[var(--text-secondary)]">
              你给作品加的个人分类，与作品自带的来源标签分开。作品详情页和作品墙的筛选使用同一份标签。
            </p>
            <UserTagSettings />
          </template>
        </section>
      </div>
    </div>
  </section>
</template>
