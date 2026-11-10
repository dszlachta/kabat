<script setup lang="ts">
import {onMounted, ref} from 'vue'
import {Events} from '@wailsio/runtime'
import {SettingsIcon} from '@lucide/vue'
import Tab from './components/Tab.vue'
import type {TabId} from './components/Tab.vue'
import Wallets from './views/main/Wallets.vue'
import Txs from './views/main/Txs.vue'
import Contracts from './views/main/Contracts.vue'
import {loadABISelectors, loadTxs} from './txs'
import {applyRepeat} from './callForm'
import {formatEtherExact} from './formatEther'
import type {TxDetails} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'

const tab = ref<TabId>('wallets')
const focusContract = ref('')
const focusTx = ref('')

function openContract(address: string) {
  focusContract.value = address
  tab.value = 'contracts'
}

function repeatCall(details: TxDetails) {
  if (details.kind !== 'call' || !details.to) {
    return
  }
  const eth = formatEtherExact(details.value)
  applyRepeat({
    address: details.to,
    selector: details.selector ?? '',
    args: (details.args ?? []).map((arg) => arg.text),
    from: details.from,
    eth: eth === '0' ? '' : eth,
  })
  openContract(details.to)
}

function openTx(hash: string) {
  focusTx.value = hash
  tab.value = 'txs'
}

function openTxs() {
  focusTx.value = ''
  tab.value = 'txs'
}

function reportError(err: unknown) {
  console.error(err)
  if (typeof window !== 'undefined') {
    window.alert(String(err))
  }
}

async function openSettings() {
  try {
    await Events.Emit('open-settings')
  } catch (err) {
    reportError(err)
  }
}

onMounted(() => {
  void loadTxs()
  void loadABISelectors()
})
</script>

<template>
  <div class="app">
    <header class="chrome">
      <Tab v-model="tab" />
      <button class="icon-btn settings" type="button" aria-label="Settings" @click="openSettings">
        <SettingsIcon class="icon" />
      </button>
    </header>
    <Wallets
      v-if="tab === 'wallets'"
      @select-tx="openTx"
      @select-contract="openContract"
      @see-all="openTxs"
    />
    <Txs
      v-else-if="tab === 'txs'"
      :focus-hash="focusTx"
      @select-wallet="tab = 'wallets'"
      @select-contract="openContract"
      @repeat="repeatCall"
      @focused="focusTx = ''"
    />
    <Contracts
      v-else
      :focus-address="focusContract"
      @focused="focusContract = ''"
      @select-tx="openTx"
    />
  </div>
</template>

<style lang="css" scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
}

.chrome {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--s-2);
  height: 50px;
  padding: 0 var(--s-2) 0 78px;
  --wails-draggable: drag;
}

.settings {
  align-self: center;
}

.settings,
.tab-panel {
  --wails-draggable: no-drag;
}

.tab-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>
