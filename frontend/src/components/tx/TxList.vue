<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {TxDetails} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {loadABISelectors, loadTxs, selectedTxHash, txs} from '../../txs'
import {ListIcon} from '@lucide/vue'
import TxFilter from './TxFilter.vue'
import type {TxFilterValue} from './TxFilter.vue'
import TxRow from './TxRow.vue'
import TxDetail from './TxDetail.vue'

const props = withDefaults(defineProps<{
  address?: string
  selectedWallet?: string
  wallets: string[]
  focusHash?: string
  showDetail?: boolean
}>(), {
  showDetail: true,
})

const emit = defineEmits<{
  (e: 'select-wallet', address: string): void
  (e: 'select-contract', address: string): void
  (e: 'repeat', details: TxDetails): void
  (e: 'select-tx', hash: string): void
  (e: 'see-all'): void
  (e: 'focused'): void
}>()

const filter = ref<TxFilterValue>('all')
const selectedHash = selectedTxHash
const details = ref<TxDetails | null>(null)
const detailsError = ref('')

function sameAddress(a: string, b: string) {
  return a.toLowerCase() === b.toLowerCase()
}

const walletRows = computed(() => {
  const addr = props.address
  if (!addr) {
    return txs.value
  }
  return txs.value.filter((tx) => sameAddress(tx.from, addr) || (!!tx.to && sameAddress(tx.to, addr)))
})

const emptyWallet = computed(() => !!props.address && walletRows.value.length === 0)

const visible = computed(() => {
  if (filter.value === 'pending') {
    return []
  }
  let rows = walletRows.value
  if (filter.value === 'final') {
    rows = rows.filter((tx) => tx.state === 'final')
  }
  return rows
})

async function loadDetails(hash: string) {
  if (!hash) {
    details.value = null
    detailsError.value = ''
    return
  }
  try {
    details.value = await WalletService.TxDetails(hash)
    detailsError.value = ''
  } catch (err) {
    details.value = null
    detailsError.value = String(err)
    console.error(err)
  }
}

function select(hash: string) {
  if (!props.showDetail) {
    emit('select-tx', hash)
    return
  }
  selectedHash.value = hash
}

function applyFocusHash(hash: string | undefined) {
  if (!hash) {
    return false
  }
  const row = txs.value.find((tx) => tx.hash.toLowerCase() === hash.toLowerCase())
  if (!row) {
    return false
  }
  selectedHash.value = row.hash
  emit('focused')
  return true
}

watch(selectedHash, (hash) => {
  if (props.showDetail) {
    void loadDetails(hash)
  }
}, {immediate: true})

watch(() => props.focusHash, (hash) => {
  applyFocusHash(hash)
}, {immediate: true})

watch(visible, (rows) => {
  if (applyFocusHash(props.focusHash)) {
    return
  }
  if (!props.showDetail) {
    return
  }
  if (selectedHash.value && rows.some((tx) => tx.hash === selectedHash.value)) {
    return
  }
  selectedHash.value = rows[0]?.hash ?? ''
}, {immediate: true})

let offABI: (() => void) | undefined

onMounted(() => {
  offABI = Events.On('abi', () => {
    void loadABISelectors()
    if (props.showDetail && selectedHash.value) {
      void loadDetails(selectedHash.value)
    }
  })
  void loadTxs()
  void loadABISelectors()
})

onUnmounted(() => {
  offABI?.()
})
</script>

<template>
  <div class="tx-list-wrap">
    <div v-if="emptyWallet" class="tx-empty">
      <p class="tx-empty-message muted">This wallet has no transactions</p>
      <button class="btn" type="button" @click="emit('see-all')">
        <ListIcon class="icon" />
        See all
      </button>
    </div>
    <template v-else>
      <TxFilter v-model="filter" />
      <div :class="showDetail ? 'split' : 'tx-list-only'">
        <aside class="tx-list-pane" :class="{ 'is-full': !showDetail }">
          <ul class="tx-list" aria-label="Transactions">
            <TxRow
              v-for="tx in visible"
              :key="tx.hash"
              :tx="tx"
              :wallets="wallets"
              :address="address"
              :selected-wallet="selectedWallet"
              :selected="showDetail && selectedHash === tx.hash"
              @select="select"
              @select-wallet="emit('select-wallet', $event)"
            />
          </ul>
        </aside>
        <TxDetail
          v-if="showDetail"
          :details="details"
          :error="detailsError"
          @select-contract="emit('select-contract', $event)"
          @repeat="details && emit('repeat', details)"
        />
      </div>
    </template>
  </div>
</template>

<style lang="css" scoped>
.tx-list-wrap,
.split,
.tx-list-only,
.tx-list-pane,
.tx-list,
.tx-empty,
.tx-empty-message {
  --wails-draggable: no-drag;
}

.tx-list-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.split,
.tx-list-only {
  display: flex;
  flex: 1;
  min-height: 0;
}

.tx-list-pane {
  display: flex;
  flex-direction: column;
  width: 36%;
  min-width: 16rem;
  max-width: 24rem;
  border-right: 1px solid var(--border);
}

.tx-list-pane.is-full {
  width: auto;
  max-width: none;
  flex: 1;
  border-right: none;
}

.tx-list {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.tx-empty {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--s-2);
  padding: var(--s-3);
}

.tx-empty-message {
  margin: 0;
}
</style>
