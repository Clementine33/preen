<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { Button } from '@/components/ui/button'
import { useApiClient } from '@/lib/api/client'
import { loginReturnAddress } from '@/app/auth-navigation'
import { useTheme } from '@/composables/use-theme'

const api = useApiClient()
const route = useRoute()
const token = ref('')
const pending = ref(false)
const error = ref('')
useTheme()

async function login(): Promise<void> {
  if (pending.value) return
  pending.value = true
  error.value = ''
  try {
    await api.createSession(token.value)
    token.value = ''
    location.replace(loginReturnAddress(route.query.redirect))
  } catch {
    token.value = ''
    error.value = '登录失败，请检查访问密钥后重试。'
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <main class="grid min-h-dvh place-items-center substrate px-6 text-foreground">
    <form class="w-full max-w-sm space-y-5" @submit.prevent="login">
      <h1 class="font-heading text-xl font-semibold">登录 Preen</h1>
      <div class="space-y-2">
        <label for="access-key" class="block text-sm">访问密钥</label>
        <input id="access-key" v-model="token" type="password" autocomplete="current-password" required autofocus
          class="h-11 w-full border border-input bg-background px-3 text-base focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none" />
      </div>
      <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
      <Button type="submit" class="w-full" :disabled="pending">{{ pending ? '登录中…' : '登录' }}</Button>
    </form>
  </main>
</template>
