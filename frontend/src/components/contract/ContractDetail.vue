<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {ContractDetails, ContractInfo, StoredContract} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {FileJsonIcon} from '@lucide/vue'
import {callFormFor, viewTab} from '../../callForm'
import ContractDetailsTab from './ContractDetailsTab.vue'
import ContractFunctionsTab from './ContractFunctionsTab.vue'
import ContractCallTab from './ContractCallTab.vue'
import ContractEIP712Tab from './ContractEIP712Tab.vue'
import JsonBlock from '../JsonBlock.vue'

const props = defineProps<{
  contract: StoredContract
}>()

const emit = defineEmits<{
  (e: 'select-tx', hash: string): void
}>()

const info = ref<ContractInfo | null>(null)
const details = ref<ContractDetails | null>(null)
const detailsError = ref('')

const hasABI = computed(() => Boolean(info.value?.abi))
const events = computed(() => info.value?.events ?? [])
const functions = computed(() => info.value?.functions ?? [])
const eip712 = computed(() => info.value?.eip712 ?? null)
const hasEIP712 = computed(() => {
  const types = eip712.value?.types
  return Boolean(types && Object.keys(types).length)
})

function emptyInfo(address: string): ContractInfo {
  return {address, name: '', abi: '', events: [], functions: []}
}

function reportError(err: unknown) {
  console.error(err)
  if (typeof window !== 'undefined') {
    window.alert(String(err))
  }
}

async function loadInfo(address: string) {
  try {
    info.value = await WalletService.ContractInfo(address)
  } catch (err) {
    info.value = emptyInfo(address)
    console.error(err)
  }
}

async function loadDetails(address: string) {
  if (!address) {
    details.value = null
    detailsError.value = ''
    return
  }
  try {
    details.value = await WalletService.ContractDetails(address)
    detailsError.value = ''
  } catch (err) {
    details.value = null
    detailsError.value = String(err)
    console.error(err)
  }
}

async function loadABI() {
  try {
    info.value = await WalletService.LoadContractABI(props.contract.address)
    await loadDetails(props.contract.address)
  } catch (err) {
    reportError(err)
  }
}

function openCall(selector: string) {
  callFormFor(props.contract.address).selector = selector
  viewTab.value = 'call'
}

watch(() => props.contract.address, (address) => {
  loadInfo(address)
  loadDetails(address)
}, {immediate: true})

watch(hasEIP712, (ok) => {
  if (!ok && viewTab.value === 'eip712') {
    viewTab.value = 'details'
  }
})

let offABI: (() => void) | undefined

onMounted(() => {
  offABI = Events.On('abi', () => {
    void loadInfo(props.contract.address)
    void loadDetails(props.contract.address)
  })
})

onUnmounted(() => {
  offABI?.()
})
</script>

<template>
  <section class="contract-detail" :aria-label="contract.address">
    <header class="contract-detail-header">
      <h2 class="contract-name">{{ contract.address }}</h2>
      <button class="btn" type="button" @click="loadABI">
        <FileJsonIcon class="icon" />
        Load ABI
      </button>
    </header>
    <nav class="tabs contract-detail-tabs" role="tablist" aria-label="Contract">
      <button
        class="tab"
        type="button"
        role="tab"
        :class="{ 'is-active': viewTab === 'details' }"
        :aria-selected="viewTab === 'details'"
        @click="viewTab = 'details'"
      >
        Details
      </button>
      <button
        class="tab"
        type="button"
        role="tab"
        :class="{ 'is-active': viewTab === 'functions' }"
        :aria-selected="viewTab === 'functions'"
        :disabled="!hasABI"
        @click="viewTab = 'functions'"
      >
        Functions
      </button>
      <button
        class="tab"
        type="button"
        role="tab"
        :class="{ 'is-active': viewTab === 'call' }"
        :aria-selected="viewTab === 'call'"
        :disabled="!hasABI"
        @click="viewTab = 'call'"
      >
        Call
      </button>
      <button
        class="tab"
        type="button"
        role="tab"
        :class="{ 'is-active': viewTab === 'eip712' }"
        :aria-selected="viewTab === 'eip712'"
        :disabled="!hasEIP712"
        @click="viewTab = 'eip712'"
      >
        EIP-712
      </button>
      <button
        class="tab"
        type="button"
        role="tab"
        :class="{ 'is-active': viewTab === 'abi' }"
        :aria-selected="viewTab === 'abi'"
        :disabled="!hasABI"
        @click="viewTab = 'abi'"
      >
        ABI
      </button>
    </nav>
    <ContractDetailsTab
      v-show="viewTab === 'details' || !hasABI"
      :details="details"
      :error="detailsError"
      @select-tx="emit('select-tx', $event)"
    />
    <template v-if="hasABI">
      <ContractFunctionsTab
        v-show="viewTab === 'functions'"
        :events="events"
        :functions="functions"
        @call="openCall"
      />
      <ContractCallTab
        v-show="viewTab === 'call'"
        :address="contract.address"
        :functions="functions"
        @select-tx="emit('select-tx', $event)"
      />
      <ContractEIP712Tab
        v-show="viewTab === 'eip712'"
        :eip712="eip712"
      />
      <JsonBlock v-show="viewTab === 'abi'" class="abi-json" role="tabpanel" :json="info?.abi ?? ''" />
    </template>
  </section>
</template>

<style lang="css" scoped>
.contract-detail,
.contract-detail-header,
.contract-name,
.contract-detail-tabs,
.abi-json {
  --wails-draggable: no-drag;
}

.contract-detail {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: var(--s-3);
  gap: var(--s-2);
}

.contract-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-2);
}

.contract-name {
  margin: 0;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
}

.contract-detail-tabs {
  flex-shrink: 0;
}

.abi-json {
  flex: 1;
  min-height: 0;
}
</style>
