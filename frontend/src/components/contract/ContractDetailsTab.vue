<script setup lang="ts">
import {ref} from 'vue'
import {CheckIcon, CopyIcon, SquareArrowRightIcon} from '@lucide/vue'
import type {ContractDetails} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {formatEtherExact} from '../../formatEther'
import ContractArgs from './ContractArgs.vue'

const props = defineProps<{
  details: ContractDetails | null
  error: string
}>()

const emit = defineEmits<{
  (e: 'select-tx', hash: string): void
}>()

const copied = ref(false)

async function copyAddress() {
  if (!props.details?.address) {
    return
  }
  await navigator.clipboard.writeText(props.details.address)
  copied.value = true
  window.setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <div class="contract-details-pane" role="tabpanel" aria-label="Details">
    <p v-if="error" class="contract-details-empty muted">{{ error }}</p>
    <template v-else-if="details">
      <template v-if="details.erc20">
        <h3 class="contract-section">ERC-20</h3>
        <dl class="contract-fields">
          <div v-if="details.erc20.name" class="contract-field">
            <dt class="muted">Name</dt>
            <dd>{{ details.erc20.name }}</dd>
          </div>
          <div v-if="details.erc20.symbol" class="contract-field">
            <dt class="muted">Symbol</dt>
            <dd>{{ details.erc20.symbol }}</dd>
          </div>
          <div class="contract-field">
            <dt class="muted">Decimals</dt>
            <dd>{{ details.erc20.decimals }}</dd>
          </div>
          <div v-if="details.erc20.totalSupply" class="contract-field">
            <dt class="muted">Total supply</dt>
            <dd>{{ details.erc20.totalSupply }}</dd>
          </div>
        </dl>
      </template>
      <dl class="contract-fields">
        <div class="contract-field">
          <dt class="muted">Address</dt>
          <dd>
            <div class="contract-copy-row">
              <code class="address">{{ details.address }}</code>
              <button
                class="icon-btn"
                type="button"
                :aria-label="copied ? 'Copied' : 'Copy address'"
                @click="copyAddress"
              >
                <CheckIcon v-if="copied" class="icon" />
                <CopyIcon v-else class="icon" />
              </button>
            </div>
          </dd>
        </div>
        <div v-if="details.name" class="contract-field">
          <dt class="muted">Name</dt>
          <dd>{{ details.name }}</dd>
        </div>
        <div class="contract-field">
          <dt class="muted">Deployer</dt>
          <dd><code class="address">{{ details.deployer }}</code></dd>
        </div>
        <div class="contract-field">
          <dt class="muted">Deploy tx</dt>
          <dd>
            <button
              class="hex-link contract-link-btn"
              type="button"
              :aria-label="'Open deploy transaction ' + details.hash"
              @click="emit('select-tx', details.hash)"
            >
              <SquareArrowRightIcon class="icon" />
              <span class="address">{{ details.hash }}</span>
            </button>
          </dd>
        </div>
        <div class="contract-field">
          <dt class="muted">Block</dt>
          <dd>{{ details.blockNumber }}</dd>
        </div>
        <div class="contract-field">
          <dt class="muted">Balance</dt>
          <dd>{{ formatEtherExact(details.balance) }} ETH</dd>
        </div>
        <div class="contract-field">
          <dt class="muted">Code size</dt>
          <dd>{{ details.codeSize }} bytes</dd>
        </div>
        <div class="contract-field">
          <dt class="muted">ABI</dt>
          <dd>{{ details.abiLoaded ? 'Loaded' : 'Not loaded' }}</dd>
        </div>
        <div v-if="details.abiLoaded" class="contract-field">
          <dt class="muted">Functions</dt>
          <dd>{{ details.functions }}</dd>
        </div>
        <div v-if="details.abiLoaded" class="contract-field">
          <dt class="muted">Events</dt>
          <dd>{{ details.events }}</dd>
        </div>
      </dl>
      <template v-if="details.constructorArgs?.length">
        <h3 class="contract-section">Constructor</h3>
        <ContractArgs :args="details.constructorArgs ?? []" />
      </template>
      <h3 class="contract-section">Bytecode</h3>
      <pre class="pre contract-pre">{{ details.bytecode }}</pre>
    </template>
  </div>
</template>

<style lang="css" scoped>
.contract-details-pane,
.contract-details-empty,
.contract-fields,
.contract-field,
.contract-copy-row,
.contract-link-btn,
.contract-section,
.contract-pre {
  --wails-draggable: no-drag;
}

.contract-details-pane {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.contract-details-empty {
  margin: 0;
}

.contract-fields {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin: 0;
}

.contract-field {
  display: grid;
  grid-template-columns: 6rem 1fr;
  gap: var(--s-1);
  align-items: baseline;
}

.contract-field dt {
  margin: 0;
  font-size: 0.85em;
}

.contract-field dd {
  margin: 0;
  min-width: 0;
}

.contract-copy-row {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  min-width: 0;
}

.contract-copy-row .address {
  flex: 1;
}

.contract-link-btn {
  max-width: 100%;
}

.contract-section {
  margin: var(--s-1) 0 0;
  font-size: 0.85em;
  font-weight: 700;
}

.contract-pre {
  margin: 0;
}
</style>
