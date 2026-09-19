<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {Account, ContractArg, ContractCallResult, ContractFunction} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {EraserIcon, PlayIcon, SendIcon, SquareArrowRightIcon} from '@lucide/vue'
import {parseEtherExact} from '../../formatEther'
import {callFormFor, clearCallInputs} from '../../callForm'
import ContractArgs from './ContractArgs.vue'

const props = defineProps<{
  address: string
  functions: ContractFunction[]
}>()

const emit = defineEmits<{
  (e: 'select-tx', hash: string): void
}>()

const callAccounts = ref<Account[]>([])
const callResult = ref<ContractCallResult | null>(null)
const callError = ref<string | null>(null)
const callTouched = ref<Record<string, boolean[]>>({})
const ethTouched = ref(false)

const currentForm = computed(() => callFormFor(props.address))
const callSelector = computed({
  get: () => currentForm.value.selector,
  set: (value) => {
    currentForm.value.selector = value
  },
})
const callFrom = computed({
  get: () => currentForm.value.from,
  set: (value) => {
    currentForm.value.from = value
  },
})
const callEth = computed({
  get: () => currentForm.value.eth,
  set: (value) => {
    currentForm.value.eth = value
  },
})
const callMethod = computed(() =>
  props.functions.find((fn) => fn.selector === callSelector.value) ?? null
)
const callValues = computed(() => currentForm.value.args[callSelector.value] ?? [])
const isWriteCall = computed(() => {
  const mut = callMethod.value?.stateMutability
  return mut === 'nonpayable' || mut === 'payable'
})
const isPayableCall = computed(() => callMethod.value?.stateMutability === 'payable')

function isRequiredArg(arg: ContractArg): boolean {
  return arg.type !== 'string'
}

function argTouched(index: number): boolean {
  return callTouched.value[callSelector.value]?.[index] ?? false
}

function markArgTouched(index: number) {
  const sel = callSelector.value
  const n = callMethod.value?.inputs?.length ?? 0
  const current = (callTouched.value[sel] ?? []).slice()
  while (current.length < n) {
    current.push(false)
  }
  current[index] = true
  callTouched.value = {...callTouched.value, [sel]: current}
}

function resetCallTouched() {
  const sel = callSelector.value
  const n = callMethod.value?.inputs?.length ?? 0
  if (sel) {
    callTouched.value = {...callTouched.value, [sel]: Array.from({length: n}, () => false)}
  }
  ethTouched.value = false
}

function argLabel(arg: ContractArg): string {
  return [arg.type, arg.name].filter(Boolean).join(' ')
}

function isIntType(type: string): boolean {
  return /^(u)?int\d*$/.test(type)
}

function argError(arg: ContractArg, value: string): string | null {
  const label = argLabel(arg)
  if (arg.type === 'string') {
    return null
  }
  if (value === '') {
    return `${label} is required`
  }
  if (isIntType(arg.type)) {
    if (/^[0-9]+$/.test(value) || /^0x[0-9a-fA-F]+$/.test(value)) {
      return null
    }
    return `${label} must be a number or 0x…`
  }
  if (value.startsWith('0x') && !/^0x([0-9a-fA-F]{2})*$/.test(value)) {
    return `${label} must be even-length hex`
  }
  return null
}

function callArgError(fn: ContractFunction, showUntouched: boolean): string {
  const inputs = fn.inputs ?? []
  const values = callValues.value
  for (let i = 0; i < inputs.length; i++) {
    const value = values[i] ?? ''
    if (!showUntouched && !argTouched(i) && value === '') {
      continue
    }
    const err = argError(inputs[i], value)
    if (err) {
      return err
    }
  }
  if (fn.stateMutability === 'payable' && (showUntouched || ethTouched.value)) {
    try {
      parseEtherExact(callEth.value)
    } catch {
      return 'ETH must be a decimal amount'
    }
  }
  return ''
}

const callValidationError = computed(() => {
  const fn = callMethod.value
  return fn ? callArgError(fn, true) : ''
})
const displayedCallError = computed(() => {
  const fn = callMethod.value
  return fn ? callArgError(fn, false) : ''
})
const canCall = computed(() => Boolean(callMethod.value) && !callValidationError.value)

function callErrorText(err: unknown): string {
  if (err && typeof err === 'object' && 'cause' in err && typeof err.cause === 'string' && err.cause) {
    return err.cause
  }
  const text = err instanceof Error ? err.message : String(err)
  const marker = 'Bound method returned an error: '
  const i = text.indexOf(marker)
  if (i >= 0) {
    return text.slice(i + marker.length)
  }
  return text.replace(/^ERR\s+/, '')
}

function ensureCallArgs(fn: ContractFunction | null) {
  const form = currentForm.value
  if (!fn) {
    return
  }
  const n = fn.inputs?.length ?? 0
  const existing = form.args[fn.selector]
  if (existing && existing.length === n) {
    return
  }
  form.args = {
    ...form.args,
    [fn.selector]: Array.from({length: n}, (_, i) => existing?.[i] ?? ''),
  }
}

function setCallArg(index: number, event: Event) {
  const form = currentForm.value
  const target = event.target
  if (!(target instanceof HTMLInputElement)) {
    return
  }
  const selector = callSelector.value
  const current = form.args[selector] ?? []
  const next = current.slice()
  next[index] = target.value
  form.args = {...form.args, [selector]: next}
  markArgTouched(index)
}

function clearCall() {
  clearCallInputs(props.address)
  resetCallTouched()
  callResult.value = null
  callError.value = null
}

async function loadCallAccounts() {
  try {
    callAccounts.value = await WalletService.Accounts() ?? []
    if (!callFrom.value) {
      callFrom.value = await WalletService.Selected()
    }
  } catch (err) {
    callAccounts.value = []
    console.error(err)
  }
}

async function runCall() {
  if (!callMethod.value || !canCall.value) {
    return
  }
  callResult.value = null
  callError.value = null
  try {
    const value = isPayableCall.value ? parseEtherExact(callEth.value) : ''
    callResult.value = await WalletService.CallContract(
      props.address,
      callSelector.value,
      callValues.value,
      callFrom.value,
      value,
    )
    if (callResult.value?.kind === 'send') {
      clearCallInputs(props.address)
      resetCallTouched()
      if (callResult.value.hash) {
        emit('select-tx', callResult.value.hash)
      }
    }
  } catch (err) {
    callError.value = callErrorText(err)
  }
}

watch(() => props.address, (address) => {
  callResult.value = null
  callError.value = null
  const form = callFormFor(address)
  if (!form.from) {
    form.from = callAccounts.value.find((account) => account.selected)?.address
      ?? callAccounts.value[0]?.address
      ?? ''
  }
})

watch(callSelector, (selector) => {
  callResult.value = null
  callError.value = null
  ensureCallArgs(props.functions.find((fn) => fn.selector === selector) ?? null)
})

watch(() => props.functions, () => {
  ensureCallArgs(callMethod.value)
})

let offAccounts: (() => void) | undefined

onMounted(() => {
  void loadCallAccounts()
  ensureCallArgs(callMethod.value)
  offAccounts = Events.On('accounts', (event) => {
    if (Array.isArray(event.data)) {
      callAccounts.value = event.data
    }
  })
})

onUnmounted(() => {
  offAccounts?.()
})
</script>

<template>
  <div class="call-pane" role="tabpanel" aria-label="Call">
    <label class="call-field">
      <span class="muted">Function</span>
      <select v-model="callSelector">
        <option disabled value="">Select a function</option>
        <option v-for="fn in functions" :key="fn.selector" :value="fn.selector">
          {{ fn.stateMutability }} {{ fn.name }}
        </option>
      </select>
    </label>
    <label class="call-field">
      <span class="muted">From</span>
      <select v-model="callFrom">
        <option v-for="account in callAccounts" :key="account.address" :value="account.address">
          {{ account.name ? account.name + ' ' + account.address : account.address }}
        </option>
      </select>
    </label>
    <label
      v-for="(arg, i) in callMethod?.inputs ?? []"
      :key="callSelector + '-' + i"
      class="call-field"
    >
      <span class="call-arg-label">
        <code class="type">{{ arg.type }}</code>
        <span v-if="arg.name">{{ arg.name }}</span>
        <span v-if="isRequiredArg(arg)" class="call-required" title="Required" aria-hidden="true">*</span>
      </span>
      <input
        :value="callValues[i] ?? ''"
        :aria-required="isRequiredArg(arg)"
        @input="setCallArg(i, $event)"
        @blur="markArgTouched(i)"
      >
    </label>
    <label v-if="isPayableCall" class="call-field">
      <span class="muted">ETH</span>
      <input
        v-model="callEth"
        @input="ethTouched = true"
        @blur="ethTouched = true"
      >
    </label>
    <p v-if="displayedCallError" class="call-error">{{ displayedCallError }}</p>
            <div class="call-actions">
              <button class="btn" type="button" :disabled="!canCall" @click="runCall">
                <SendIcon v-if="isWriteCall" class="icon" />
                <PlayIcon v-else class="icon" />
                {{ isWriteCall ? 'Send' : 'Call' }}
              </button>
              <button class="btn" type="button" @click="clearCall">
                <EraserIcon class="icon" />
                Clear
              </button>
            </div>
            <pre v-if="callError !== null" class="pre call-result">{{ callError }}</pre>
    <template v-else-if="callResult">
      <p v-if="callResult.kind === 'view' && !callResult.outputs?.length" class="call-empty muted">
        (returns nothing)
      </p>
      <ContractArgs
        v-else-if="callResult.kind === 'view'"
        :args="callResult.outputs ?? []"
        label="Return values"
      />
      <button
        v-else-if="callResult.hash"
        class="hex-link contract-link-btn"
        type="button"
        :aria-label="'Open transaction ' + callResult.hash"
        @click="emit('select-tx', callResult.hash)"
      >
        <SquareArrowRightIcon class="icon" />
        <span class="address">{{ callResult.hash }}</span>
      </button>
    </template>
  </div>
</template>

<style lang="css" scoped>
.call-pane,
.call-field,
.call-error,
.call-result,
.call-empty,
.contract-link-btn {
  --wails-draggable: no-drag;
}

.call-pane {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.call-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  color: var(--text);
  font-weight: 600;
}

.call-arg-label {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
}

.call-required {
  color: crimson;
}

.call-field select,
.call-field input {
  font-weight: 400;
}

.call-error {
  margin: 0;
  color: #c62828;
}

.call-actions {
  display: flex;
  align-items: center;
  gap: var(--s-1);
}

.call-empty {
  margin: 0;
}

.call-result {
  margin: 0;
}

.contract-link-btn {
  max-width: 100%;
}
</style>
