<script setup lang="ts">
import {onMounted, onUnmounted, ref} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {Account} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'

const ANVIL_ERROR = 'cannot connect to local Anvil chain'

const loading = ref(true)
const error = ref('')
const accounts = ref<Account[]>([])
const copied = ref('')

let offAccounts: (() => void) | undefined

async function load() {
  loading.value = true
  error.value = ''
  try {
    accounts.value = await WalletService.Accounts() ?? []
  } catch {
    accounts.value = []
    error.value = ANVIL_ERROR
  } finally {
    loading.value = false
  }
}

async function copyAddress(address: string) {
  await navigator.clipboard.writeText(address)
  copied.value = address
  window.setTimeout(() => {
    if (copied.value === address) {
      copied.value = ''
    }
  }, 1500)
}

async function selectAccount(address: string) {
  await WalletService.Select(address)
  accounts.value = await WalletService.Accounts() ?? []
}

onMounted(() => {
  load()
  offAccounts = Events.On('accounts', (event) => {
    if (Array.isArray(event.data)) {
      accounts.value = event.data
      error.value = ''
    }
  })
})

onUnmounted(() => {
  offAccounts?.()
})
</script>

<template>
  <main class="container">
    <h1 class="title"><span class="title-accent">Kabat</span></h1>
    <p class="subtitle">Local Anvil development wallets</p>

    <p v-if="loading" class="status">Connecting…</p>

    <div v-else-if="error" class="error" role="alert">
      <p class="error-message">{{ error }}</p>
      <button class="btn" type="button" @click="load">Retry</button>
    </div>

    <ul v-else class="wallets" aria-label="Anvil wallets">
      <li
        v-for="account in accounts"
        :key="account.address"
        class="wallet"
        :class="{ 'is-selected': account.selected }"
        @click="selectAccount(account.address)"
      >
        <div class="identity">
          <code class="address">{{ account.address }}</code>
          <span v-if="account.selected" class="selected-label">Selected</span>
          <button
            class="copy"
            type="button"
            :aria-label="'Copy ' + account.address"
            @click.stop="copyAddress(account.address)"
          >
            {{ copied === account.address ? 'Copied' : 'Copy' }}
          </button>
        </div>
        <span class="balance">{{ account.balance }} ETH</span>
      </li>
    </ul>
  </main>
</template>
