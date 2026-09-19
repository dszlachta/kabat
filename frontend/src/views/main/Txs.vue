<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {Account, TxDetails} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import TxList from '../../components/tx/TxList.vue'

const props = defineProps<{
  focusHash?: string
}>()

const emit = defineEmits<{
  (e: 'select-wallet', address: string): void
  (e: 'select-contract', address: string): void
  (e: 'repeat', details: TxDetails): void
  (e: 'focused'): void
}>()

const accounts = ref<Account[]>([])
const wallets = computed(() => accounts.value.map((account) => account.address))
const selectedWallet = computed(() =>
  accounts.value.find((account) => account.selected)?.address ?? ''
)

let offAccounts: (() => void) | undefined

async function load() {
  try {
    accounts.value = await WalletService.Accounts() ?? []
  } catch (err) {
    accounts.value = []
    console.error(err)
  }
}

async function selectWallet(address: string) {
  await WalletService.Select(address)
  emit('select-wallet', address)
}

onMounted(() => {
  load()
  offAccounts = Events.On('accounts', (event) => {
    if (Array.isArray(event.data)) {
      accounts.value = event.data
    }
  })
})

onUnmounted(() => {
  offAccounts?.()
})
</script>

<template>
  <section class="tab-panel" role="tabpanel">
    <TxList
      :wallets="wallets"
      :selected-wallet="selectedWallet"
      :focus-hash="focusHash"
      @select-wallet="selectWallet"
      @select-contract="emit('select-contract', $event)"
      @repeat="emit('repeat', $event)"
      @focused="emit('focused')"
    />
  </section>
</template>

<style lang="css" scoped>
.tab-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  --wails-draggable: no-drag;
}
</style>
