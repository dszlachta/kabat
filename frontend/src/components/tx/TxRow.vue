<script setup lang="ts">
import {computed} from 'vue'
import {SquareArrowRightIcon} from '@lucide/vue'
import type {StoredTx} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {formatEtherSigned} from '../../formatEther'
import {abiSelectors, methodNameFor} from '../../txs'

const props = defineProps<{
  tx: StoredTx
  wallets: string[]
  address?: string
  selectedWallet?: string
  selected?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-wallet', address: string): void
  (e: 'select', hash: string): void
}>()

const methodName = computed(() => {
  abiSelectors.value
  return methodNameFor(props.tx)
})

const kindLabel = computed(() => {
  switch (props.tx.kind) {
    case 'transfer':
      return 'Transfer'
    case 'call':
      return 'Call'
    case 'create':
      return 'Create'
    default:
      return props.tx.kind
  }
})

function sameAddress(a: string, b: string) {
  return a.toLowerCase() === b.toLowerCase()
}

function walletFor(addr: string | undefined): string {
  if (!addr) {
    return ''
  }
  const lower = addr.toLowerCase()
  return props.wallets.find((w) => w.toLowerCase() === lower) ?? ''
}

const viewer = computed(() => props.selectedWallet || props.address || '')
const incoming = computed(() =>
  props.tx.kind === 'transfer' &&
  Boolean(props.tx.to) &&
  Boolean(viewer.value) &&
  sameAddress(props.tx.to ?? '', viewer.value)
)
const counterparty = computed(() => incoming.value ? props.tx.from : props.tx.to)
const counterpartyWallet = computed(() => walletFor(counterparty.value))
const counterpartyLabel = computed(() => incoming.value ? 'From' : 'To')

function changeFor(addr: string): string {
  const changes = props.tx.balanceChanges
  if (!changes || !addr) {
    return ''
  }
  const lower = addr.toLowerCase()
  for (const [key, value] of Object.entries(changes)) {
    if (key.toLowerCase() === lower && value) {
      return value
    }
  }
  return ''
}

const ethChange = computed(() => {
  let raw = ''
  if (props.address) {
    raw = changeFor(props.address)
  } else {
    raw = changeFor(props.tx.from)
    if (!raw && props.tx.to) {
      raw = changeFor(props.tx.to)
    }
  }
  if (!raw || formatEtherSigned(raw) === '+0') {
    return ''
  }
  return raw
})

const ethChangeOut = computed(() => ethChange.value.startsWith('-'))
</script>

<template>
  <li
    class="tx-row"
    :class="{ 'is-selected': selected }"
    @click="emit('select', tx.hash)"
  >
    <span
      class="tx-status"
      :class="{ 'is-ok': tx.ok }"
      :title="tx.ok ? 'Success' : 'Failed'"
      :aria-label="tx.ok ? 'Success' : 'Failed'"
    />
    <div class="tx-meta">
      <span class="tx-block">{{ tx.blockNumber }}</span>
      <code class="address tx-hash">{{ tx.hash }}</code>
    </div>
    <span class="tx-kind">
      {{ kindLabel }}
      <span v-if="methodName" class="tx-method">{{ methodName }}</span>
      <code v-if="tx.kind === 'call' && tx.selector" class="address tx-selector">{{ tx.selector }}</code>
    </span>
    <div class="tx-details">
      <span
        v-if="ethChange"
        class="tx-eth"
        :class="ethChangeOut ? 'is-out' : 'is-in'"
      >{{ formatEtherSigned(ethChange) }} ETH</span>
      <span>gas {{ tx.gas }}</span>
      <span v-if="tx.kind === 'transfer'" class="tx-to">
        {{ counterpartyLabel }}
        <button
          v-if="counterpartyWallet"
          class="hex-link tx-to-btn"
          type="button"
          :aria-label="'Select wallet ' + counterpartyWallet"
          @click.stop="emit('select-wallet', counterpartyWallet)"
        >
          <SquareArrowRightIcon class="icon" />
          <span class="address">{{ counterparty }}</span>
        </button>
        <code v-else class="address">{{ counterparty }}</code>
      </span>
    </div>
  </li>
</template>

<style lang="css" scoped>
.tx-row {
  display: grid;
  grid-template-columns: auto 1fr;
  column-gap: var(--s-2);
  row-gap: 2px;
  align-items: start;
  padding: var(--s-2);
  border-bottom: 1px solid var(--border);
  min-width: 0;
  cursor: pointer;
  --wails-draggable: no-drag;
}

.tx-row.is-selected {
  background: var(--selected-fill);
}

.tx-status {
  grid-column: 1;
  grid-row: 1;
  width: 8px;
  height: 8px;
  margin-top: 4px;
  border-radius: var(--radius);
  background: crimson;
}

.tx-status.is-ok {
  background: limegreen;
}

.tx-meta,
.tx-kind,
.tx-details {
  grid-column: 2;
  min-width: 0;
}

.tx-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.tx-block {
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}

.tx-hash {
  flex: 1;
}

.tx-kind {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
  font-weight: 600;
}

.tx-method,
.tx-selector {
  font-weight: 400;
}

.tx-details {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  font-size: 0.85em;
  opacity: 0.9;
}

.tx-eth.is-out {
  color: crimson;
}

.tx-eth.is-in {
  color: limegreen;
}

.tx-to {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  min-width: 0;
}

.tx-to-btn {
  max-width: 12rem;
}
</style>
