<script setup lang="ts">
import type {ContractEvent, ContractFunction} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {ArrowLeftIcon, ArrowRightIcon, PlayIcon} from '@lucide/vue'

defineProps<{
  events: ContractEvent[]
  functions: ContractFunction[]
}>()

const emit = defineEmits<{
  (e: 'call', selector: string): void
}>()
</script>

<template>
  <ul class="abi-members" role="tabpanel" aria-label="Events and functions">
    <li v-for="(ev, i) in events" :key="'event-' + ev.name + '-' + i" class="abi-event">
      <div class="abi-name">{{ ev.name }}</div>
      <div class="abi-inputs">
        <template v-if="ev.inputs?.length">
          <template v-for="(arg, j) in ev.inputs" :key="j">
            <span class="abi-arg">
              <span v-if="j" class="abi-arg-sep">,</span>
              <code class="type">{{ arg.type }}</code>
              <span v-if="arg.indexed" class="abi-arg-mod">indexed</span>
              <span v-if="arg.name" class="abi-arg-name">{{ arg.name }}</span>
            </span>
          </template>
        </template>
      </div>
    </li>
    <li v-for="(fn, i) in functions" :key="'fn-' + fn.selector + '-' + i" class="abi-fn">
      <div class="abi-fn-head">
        <div class="abi-name">
          <code v-if="fn.stateMutability" class="type">{{ fn.stateMutability }}</code>
          <span>{{ fn.name }}</span>
        </div>
        <button class="btn" type="button" @click="emit('call', fn.selector)">
          <PlayIcon class="icon" />
          Call
        </button>
      </div>
      <div class="abi-io">
        <ArrowRightIcon class="abi-io-icon" title="Inputs" aria-label="Inputs" />
        <div class="abi-io-line">
          <span v-if="!fn.inputs?.length">(takes no arguments)</span>
          <template v-else>
            <template v-for="(arg, j) in fn.inputs" :key="j">
              <span class="abi-arg">
                <span v-if="j" class="abi-arg-sep">,</span>
                <code class="type">{{ arg.type }}</code>
                <span v-if="arg.name" class="abi-arg-name">{{ arg.name }}</span>
              </span>
            </template>
          </template>
        </div>
      </div>
      <div class="abi-io">
        <ArrowLeftIcon class="abi-io-icon" title="Outputs" aria-label="Outputs" />
        <div class="abi-io-line">
          <span v-if="!fn.outputs?.length">(returns nothing)</span>
          <template v-else>
            <template v-for="(arg, j) in fn.outputs" :key="j">
              <span class="abi-arg">
                <span v-if="j" class="abi-arg-sep">, </span>
                <code class="type">{{ arg.type }}</code>
                <span v-if="arg.name" class="abi-arg-name">{{ arg.name }}</span>
              </span>
            </template>
          </template>
        </div>
      </div>
    </li>
  </ul>
</template>

<style lang="css" scoped>
.abi-members,
.abi-event,
.abi-fn {
  --wails-draggable: no-drag;
}

.abi-members {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
  list-style: none;
  padding: 0;
}

.abi-event,
.abi-fn {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: var(--s-2) 0;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  min-width: 0;
}

.abi-fn-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--s-2);
}

.abi-fn-head .btn {
  flex-shrink: 0;
}

.abi-name {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.abi-inputs,
.abi-io-line {
  font-size: 0.85em;
  overflow-wrap: anywhere;
  min-width: 0;
}

.abi-arg {
  display: inline-flex;
  align-items: baseline;
  gap: 0.35em;
}

.abi-arg-name,
.abi-arg-mod {
  font-family: inherit;
  font-weight: 400;
}

.abi-io {
  display: flex;
  align-items: flex-start;
  gap: 0.4rem;
  min-width: 0;
}

.abi-io-icon {
  width: 14px;
  height: 14px;
  margin-top: 0.15rem;
  flex-shrink: 0;
  color: currentColor;
}
</style>
