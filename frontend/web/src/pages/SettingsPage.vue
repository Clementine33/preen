<script setup lang="ts">
import type { KeyGroup } from '@/composables/use-keymap'
import type { MotionPreference } from '@/composables/use-motion'
import type { Theme } from '@/composables/use-theme'
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import { Button } from '@/components/ui/button'
import { GROUP_LABELS, KEYS } from '@/composables/use-keymap'
import { MOTION_OPTIONS, useMotion } from '@/composables/use-motion'
import { THEME_OPTIONS, useTheme } from '@/composables/use-theme'
import AboutSettings from '@/features/settings/AboutSettings.vue'
import DataMaintenanceSettings from '@/features/settings/DataMaintenanceSettings.vue'
import DiagnosticLogExport from '@/features/settings/DiagnosticLogExport.vue'
import FormatPreferenceSettings from '@/features/settings/FormatPreferenceSettings.vue'
import OutboundProxySettings from '@/features/settings/OutboundProxySettings.vue'
import SettingsField from '@/features/settings/SettingsField.vue'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import UserTagSettings from '@/features/settings/UserTagSettings.vue'
import { useApiClient } from '@/lib/api/client'

/**
 * 设置 (`/settings/:section?`): what has no other home, one category per
 * address — appearance (theme and motion stay in this browser; the Format
 * Preference syncs through the server), network (the Outbound Proxy) and the
 * user-tag vocabulary and data maintenance (record cleanup), plus the read-only shortcut list and 关于 (version and licence). Each saves
 * where its data lives. Task settings stay beside their tasks, a library's storage
 * class in its edit dialog.
 */
const SECTIONS = [
  { id: 'appearance', label: '外观', code: 'APPEARANCE' },
  { id: 'network', label: '网络', code: 'NETWORK' },
  { id: 'tags', label: '用户标签', code: 'USER TAGS' },
  { id: 'maintenance', label: '数据维护', code: 'DATA MAINTENANCE' },
  { id: 'keys', label: '键盘快捷键', code: 'KEYBOARD' },
  { id: 'about', label: '关于', code: 'ABOUT' },
] as const

/** The shortcut list, generated from the keymap's descriptors (read-only, not a setting). */
const KEY_ROWS = (Object.keys(GROUP_LABELS) as KeyGroup[]).map(group => ({
  group,
  rows: Object.values(KEYS).filter(descriptor => descriptor.group === group),
}))
function keyCap(combo: string): string {
  return combo.replace('Arrow', '').replace('Left', '←').replace('Right', '→').replace('Up', '↑').replace('Down', '↓')
}

const route = useRoute()
const section = computed(() => SECTIONS.find(item => item.id === route.params.section) ?? SECTIONS[0])

const { theme } = useTheme()
const { preference, systemReduces } = useMotion()
const api = useApiClient()
const loggingOut = ref(false)
const logoutError = ref('')

async function logout(): Promise<void> {
  if (loggingOut.value)
    return
  loggingOut.value = true
  logoutError.value = ''
  try {
    await api.deleteSession()
    location.replace('/login')
  }
  catch {
    logoutError.value = '退出失败，请重试。'
  }
  finally {
    loggingOut.value = false
  }
}
</script>

<template>
  <section class="flex h-full min-w-0 flex-col substrate" data-testid="settings-page">
    <header class="flex min-h-14 shrink-0 items-center gap-3 edge-bottom px-4 py-1.5 rail:px-5">
      <GlobalMenu />
      <h1 class="font-heading text-sm font-semibold">设置</h1>
      <Button variant="outline" class="ml-auto" :disabled="loggingOut" data-testid="logout" @click="logout">
        {{ loggingOut ? '退出中…' : '退出登录' }}
      </Button>
    </header>
    <p v-if="logoutError" role="alert" class="px-4 py-2 text-sm text-destructive">{{ logoutError }}</p>

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
            <SettingsField id="format-preference" label="格式偏好" hint="只作用于 TRACKLIST；跨设备同步">
              <FormatPreferenceSettings />
            </SettingsField>
          </template>

          <template v-else-if="section.id === 'network'">
            <SettingsField id="outbound-proxy" label="代理服务器" hint="元数据与封面下载所用；跟随系统时读取后端所在电脑的代理设置">
              <OutboundProxySettings />
            </SettingsField>
          </template>

          <template v-else-if="section.id === 'maintenance'">
            <DataMaintenanceSettings />
            <SettingsField id="diagnostic-log" label="诊断日志" hint="最近的后端运行日志，用于反馈问题；不含访问密钥，含媒体路径" class="mt-6">
              <DiagnosticLogExport />
            </SettingsField>
          </template>

          <template v-else-if="section.id === 'keys'">
            <div data-testid="settings-keys">
              <dl v-for="{ group, rows } in KEY_ROWS" :key="group" class="mt-4">
                <dt class="lbl">{{ GROUP_LABELS[group] }}</dt>
                <dd v-for="row in rows" :key="row.text" class="flex items-baseline gap-3 border-b border-[var(--wire-fine)] py-1.5 text-sm">
                  <span class="flex min-w-32 shrink-0 gap-1"><kbd v-for="combo in row.keys" :key="combo" class="border border-[var(--wire-fine)] px-1 font-mono text-xs">{{ keyCap(combo) }}</kbd></span>
                  <span>{{ row.text }}</span>
                </dd>
              </dl>
            </div>
          </template>

          <AboutSettings v-else-if="section.id === 'about'" />

          <template v-else>
            <UserTagSettings />
          </template>
        </section>
      </div>
    </div>
  </section>
</template>
