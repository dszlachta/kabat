<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {StoredContract} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {selectedHash} from '../../callForm'
import ContractRow from './ContractRow.vue'
import ContractDetail from './ContractDetail.vue'

const props = defineProps<{
  focusAddress?: string
}>()

const emit = defineEmits<{
  (e: 'focused'): void
  (e: 'select-tx', hash: string): void
}>()

const contracts = ref<StoredContract[]>([])

const selected = computed(() =>
  contracts.value.find((c) => c.hash === selectedHash.value) ?? null
)

async function load() {
  try {
    contracts.value = await WalletService.Contracts() ?? []
  } catch (err) {
    contracts.value = []
    console.error(err)
  }
}

function select(hash: string) {
  selectedHash.value = hash
}

function selectByAddress(address: string | undefined) {
  if (!address) {
    return false
  }
  const lower = address.toLowerCase()
  const item = contracts.value.find((c) => c.address.toLowerCase() === lower)
  if (!item) {
    return false
  }
  selectedHash.value = item.hash
  emit('focused')
  return true
}

watch(() => props.focusAddress, (address) => {
  selectByAddress(address)
})

watch(contracts, (list) => {
  if (list.length === 0) {
    selectedHash.value = ''
    return
  }
  if (selectByAddress(props.focusAddress)) {
    return
  }
  if (!selected.value) {
    selectedHash.value = list[0].hash
  }
}, {immediate: true})

let offTransactions: (() => void) | undefined

onMounted(() => {
  load()
  offTransactions = Events.On('transactions', () => {
    load()
  })
})

onUnmounted(() => {
  offTransactions?.()
})
</script>

<template>
  <div class="split">
    <aside class="contract-list-pane">
      <ul class="contracts" aria-label="Deployed contracts">
        <ContractRow
          v-for="item in contracts"
          :key="item.hash"
          :contract="item"
          :selected="selected?.hash === item.hash"
          @select="select"
        />
      </ul>
    </aside>
    <ContractDetail
      v-if="selected"
      :key="selected.hash"
      :contract="selected"
      @select-tx="emit('select-tx', $event)"
    />
  </div>
</template>

<style lang="css" scoped>
.split,
.contract-list-pane,
.contracts {
  --wails-draggable: no-drag;
}

.split {
  display: flex;
  flex: 1;
  min-height: 0;
}

.contract-list-pane {
  display: flex;
  flex-direction: column;
  width: 36%;
  min-width: 16rem;
  max-width: 24rem;
  border-right: 1px solid var(--border);
}

.contracts {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow: auto;
}
</style>
