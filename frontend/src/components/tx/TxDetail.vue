<script setup lang="ts">
import {computed, ref} from 'vue'
import {CheckIcon, CopyIcon, RotateCcwIcon, SquareArrowRightIcon} from '@lucide/vue'
import type {TxDetails} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {formatEtherExact, formatEtherSigned} from '../../formatEther'

const props = defineProps<{
  details: TxDetails | null
  error: string
}>()

const emit = defineEmits<{
  (e: 'select-contract', address: string): void
  (e: 'repeat'): void
}>()

const canRepeat = computed(() => props.details?.kind === 'call' && Boolean(props.details.to))

const copied = ref(false)

const kindLabel = computed(() => {
  switch (props.details?.kind) {
    case 'transfer':
      return 'Transfer'
    case 'call':
      return 'Call'
    case 'create':
      return 'Create'
    default:
      return props.details?.kind ?? ''
  }
})

async function copyHash() {
  if (!props.details?.hash) {
    return
  }
  await navigator.clipboard.writeText(props.details.hash)
  copied.value = true
  window.setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <section v-if="error" class="tx-detail" aria-label="Transaction details">
    <p class="tx-detail-empty muted">{{ error }}</p>
  </section>
  <section v-else-if="details" class="tx-detail" :aria-label="details.hash">
    <header class="tx-detail-header">
      <span
        class="tx-status"
        :class="{ 'is-ok': details.ok }"
        :title="details.ok ? 'Success' : 'Failed'"
        :aria-label="details.ok ? 'Success' : 'Failed'"
      />
      <div class="tx-detail-title">
        <span class="tx-kind">
          {{ details.ok ? 'Success' : 'Reverted' }}
          {{ kindLabel }}
          <span v-if="details.method" class="tx-method">{{ details.method }}</span>
          <code v-if="details.selector" class="address tx-selector">{{ details.selector }}</code>
        </span>
        <div class="tx-hash-row">
          <code class="address">{{ details.hash }}</code>
          <button
            class="icon-btn"
            type="button"
            :aria-label="copied ? 'Copied' : 'Copy hash'"
            @click="copyHash"
          >
            <CheckIcon v-if="copied" class="icon" />
            <CopyIcon v-else class="icon" />
          </button>
          <button
            v-if="canRepeat"
            class="icon-btn"
            type="button"
            aria-label="Repeat"
            @click="emit('repeat')"
          >
            <RotateCcwIcon class="icon" />
          </button>
        </div>
        <small class="tx-block muted">Block {{ details.blockNumber }}</small>
      </div>
    </header>

    <dl class="tx-fields">
      <div class="tx-field">
        <dt class="muted">From</dt>
        <dd><code class="address">{{ details.from }}</code></dd>
      </div>
      <div v-if="details.to" class="tx-field">
        <dt class="muted">To</dt>
        <dd><code class="address">{{ details.to }}</code></dd>
      </div>
      <div v-if="details.contract" class="tx-field">
        <dt class="muted">Contract</dt>
        <dd>
          <button
            class="hex-link tx-contract-btn"
            type="button"
            :aria-label="'Open contract ' + details.contract"
            @click="emit('select-contract', details.contract)"
          >
            <SquareArrowRightIcon class="icon" />
            <span class="address">{{ details.contract }}</span>
          </button>
        </dd>
      </div>
      <div class="tx-field">
        <dt class="muted">Value</dt>
        <dd>{{ formatEtherExact(details.value) }} ETH</dd>
      </div>
      <div class="tx-field">
        <dt class="muted">Nonce</dt>
        <dd>{{ details.nonce }}</dd>
      </div>
      <div class="tx-field">
        <dt class="muted">Gas</dt>
        <dd>{{ details.gasUsed }} / {{ details.gasLimit }}</dd>
      </div>
      <div class="tx-field">
        <dt class="muted">Fee</dt>
        <dd>{{ formatEtherExact(details.fee) }} ETH</dd>
      </div>
      <div class="tx-field">
        <dt class="muted">Gas price</dt>
        <dd><code class="address">{{ details.gasPrice }}</code></dd>
      </div>
    </dl>

    <ul v-if="details.balanceChanges?.length" class="tx-balances" aria-label="ETH balance changes">
      <li v-for="change in details.balanceChanges" :key="change.address" class="tx-balance">
        <code class="address">{{ change.address }}</code>
        <span
          class="tx-eth"
          :class="change.delta.startsWith('-') ? 'is-out' : 'is-in'"
        >{{ formatEtherSigned(change.delta) }} ETH</span>
      </li>
    </ul>

    <template v-if="details.args?.length">
      <h3 class="tx-section">Arguments</h3>
      <ul class="tx-args">
        <li v-for="(arg, i) in details.args" :key="i" class="tx-arg">
          <span class="tx-arg-label">
            <code v-if="arg.type" class="type">{{ arg.type }}</code>
            <span v-if="arg.name">{{ arg.name }}</span>
          </span>
          <code class="address">{{ arg.text }}</code>
        </li>
      </ul>
    </template>
    <h3 class="tx-section">Calldata</h3>
    <pre class="pre tx-pre">{{ details.calldata }}</pre>

    <template v-if="details.revert">
      <h3 class="tx-section">Revert</h3>
      <pre class="pre tx-pre">{{ details.revert }}</pre>
    </template>

    <template v-if="details.logs?.length">
      <h3 class="tx-section">Logs</h3>
      <ul class="tx-logs">
        <li v-for="(log, i) in details.logs" :key="i" class="tx-log">
          <div class="tx-log-head">
            <span v-if="log.name" class="tx-log-name">{{ log.name }}</span>
            <code class="address">{{ log.address }}</code>
          </div>
          <ul v-if="log.args?.length" class="tx-args">
            <li v-for="(arg, j) in log.args" :key="j" class="tx-arg">
              <span class="tx-arg-label">
                <code v-if="arg.type" class="type">{{ arg.type }}</code>
                <span v-if="arg.name">{{ arg.name }}</span>
              </span>
              <code class="address">{{ arg.text }}</code>
            </li>
          </ul>
          <dl class="tx-fields tx-log-raw">
            <div class="tx-field">
              <dt>Data</dt>
              <dd><pre class="pre tx-pre">{{ log.data }}</pre></dd>
            </div>
            <div
              v-for="(topic, ti) in log.topics ?? []"
              :key="ti + '-' + topic"
              class="tx-field"
            >
              <dt>Topic {{ ti }}</dt>
              <dd><code class="address">{{ topic }}</code></dd>
            </div>
          </dl>
        </li>
      </ul>
    </template>
  </section>
  <section v-else class="tx-detail" aria-label="Transaction details">
    <p class="tx-detail-empty muted">Select a transaction.</p>
  </section>
</template>

<style lang="css" scoped>
.tx-detail,
.tx-detail-header,
.tx-detail-title,
.tx-fields,
.tx-pre,
.tx-detail-empty {
  --wails-draggable: no-drag;
}

.tx-detail {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  padding: var(--s-3);
  overflow: auto;
}

.tx-detail-empty {
  margin: 0;
}

.tx-detail-header {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
}

.tx-status {
  width: 8px;
  height: 8px;
  margin-top: 6px;
  flex-shrink: 0;
  border-radius: var(--radius);
  background: crimson;
}

.tx-status.is-ok {
  background: limegreen;
}

.tx-detail-title {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 0;
  flex: 1;
}

.tx-kind {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem;
  font-weight: 600;
}

.tx-method,
.tx-selector {
  font-weight: 400;
}

.tx-hash-row {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  min-width: 0;
}

.tx-hash-row .address {
  flex: 1;
}

.tx-fields {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin: 0;
}

.tx-field {
  display: grid;
  grid-template-columns: 6rem 1fr;
  gap: var(--s-1);
  align-items: baseline;
}

.tx-field dt {
  margin: 0;
  font-size: 0.85em;
}

.tx-field dd {
  margin: 0;
  min-width: 0;
}

.tx-contract-btn {
  max-width: 100%;
}

.tx-balances,
.tx-args,
.tx-logs {
  list-style: none;
  margin: 0;
  padding: 0;
}

.tx-balance {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--s-1);
  padding: 0.25rem 0;
}

.tx-eth.is-out {
  color: crimson;
}

.tx-eth.is-in {
  color: limegreen;
}

.tx-section {
  margin: var(--s-1) 0 0;
  font-size: 0.85em;
  font-weight: 700;
}

.tx-arg {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.25rem 0;
  border-bottom: 1px solid var(--border);
}

.tx-arg-label {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-weight: 600;
  font-size: 0.85em;
}

.tx-pre {
  margin: 0;
}

.tx-log {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: var(--s-2) 0;
  border-bottom: 1px solid var(--border);
}

.tx-log-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem;
}

.tx-log-name {
  font-weight: 600;
}

.tx-log-raw {
  margin: 0;
}
</style>
