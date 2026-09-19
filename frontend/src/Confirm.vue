<script setup lang="ts">
import {computed, onUnmounted} from 'vue'
import {Events} from '@wailsio/runtime'
import {pendingTx} from './pendingTx'

const pendingJson = computed(() => JSON.stringify(pendingTx.value ?? {}, null, 2))

const offPending = Events.On('pendingTransaction', (event) => {
  const data = event.data
  pendingTx.value = data && typeof data === 'object' && !Array.isArray(data)
    ? data as Record<string, unknown>
    : {}
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

async function confirm() {
  try {
    await Events.Emit('confirm-yes')
  } catch (err) {
    reportError(err)
  }
}

async function cancel() {
  try {
    await Events.Emit('confirm-no')
  } catch (err) {
    reportError(err)
  }
}
</script>

<template>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
    <header class="dialog-header">
      <h2 id="confirm-title" class="confirm-title">Confirm transaction</h2>
    </header>
    <pre class="tx-json">{{ pendingJson }}</pre>
    <div class="confirm-actions">
      <button class="copy" type="button" @click="cancel">Cancel</button>
      <button class="btn" type="button" @click="confirm">Confirm</button>
    </div>
  </div>
</template>
