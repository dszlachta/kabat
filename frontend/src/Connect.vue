<script setup lang="ts">
import {onUnmounted, ref} from 'vue'
import {Events} from '@wailsio/runtime'

const origin = ref('')
const sessionUlid = ref('')
const accounts = ref<string[]>([])
const selected = ref('')

const offPending = Events.On('pendingConnection', (event) => {
  const data = event.data && typeof event.data === 'object' && !Array.isArray(event.data)
    ? event.data as {origin?: string; sessionUlid?: string; accounts?: unknown}
    : {}
  origin.value = typeof data.origin === 'string' ? data.origin : ''
  sessionUlid.value = typeof data.sessionUlid === 'string' ? data.sessionUlid : ''
  accounts.value = Array.isArray(data.accounts)
    ? data.accounts.filter((addr): addr is string => typeof addr === 'string')
    : []
  selected.value = accounts.value[0] ?? ''
})

onUnmounted(() => {
  offPending()
})

function reportError(err: unknown) {
  console.error(err)
  if (typeof window !== 'undefined') {
    window.alert(String(err))
  }
}

async function allow() {
  try {
    await Events.Emit('connect-allow', selected.value)
  } catch (err) {
    reportError(err)
  }
}

async function disallow() {
  try {
    await Events.Emit('connect-deny')
  } catch (err) {
    reportError(err)
  }
}
</script>

<template>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="connect-title">
    <header class="dialog-header">
      <h2 id="connect-title" class="confirm-title">Connection request</h2>
    </header>
    <p class="connect-copy">A dapp wants to connect. Please choose a wallet.</p>
    <dl class="connect-meta">
      <div>
        <dt class="muted">URL</dt>
        <dd class="address">{{ origin }}</dd>
      </div>
      <div>
        <dt class="muted">Session ID</dt>
        <dd class="address">{{ sessionUlid }}</dd>
      </div>
    </dl>
    <ul class="wallets connect-wallets" aria-label="Wallets">
      <li
        v-for="address in accounts"
        :key="address"
        class="wallet"
        :class="{ 'is-selected': address === selected }"
        @click="selected = address"
      >
        <span class="address">{{ address }}</span>
      </li>
    </ul>
    <div class="confirm-actions">
      <button class="copy" type="button" @click="disallow">Disallow</button>
      <button class="btn-primary" type="button" :disabled="!selected" @click="allow">Allow</button>
    </div>
  </div>
</template>

<style lang="css" scoped>
.dialog {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  background: var(--fill);
  color: var(--text);
  border: 1px solid var(--border);
}

.dialog-header {
  --wails-draggable: drag;
  padding: var(--s-2) var(--s-3);
  border-bottom: 1px solid var(--border);
}

.confirm-title {
  margin: 0;
  font-weight: 600;
  color: var(--text);
}

.connect-copy,
.connect-meta,
.connect-wallets,
.wallets,
.wallet,
.confirm-actions {
  --wails-draggable: no-drag;
}

.connect-copy {
  margin: 0 var(--s-3) var(--s-2);
}

.connect-meta {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  margin: 0 var(--s-3) var(--s-2);
}

.connect-meta dt {
  font-size: 0.85em;
  font-weight: 600;
}

.connect-meta dd {
  margin: 0.15rem 0 0;
  word-break: break-all;
}

.wallets {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.connect-wallets {
  margin: 0 0 var(--s-2);
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}

.wallet {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: var(--s-2);
  border-bottom: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
}

.wallet.is-selected {
  background: var(--selected-fill);
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-1);
  padding: 0 var(--s-3) var(--s-3);
}
</style>
