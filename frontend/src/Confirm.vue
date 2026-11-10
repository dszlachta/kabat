<script setup lang="ts">
import {computed, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import ConfirmField from './components/ConfirmField.vue'
import JsonBlock from './components/JsonBlock.vue'
import type {ConfirmFieldNode} from './components/ConfirmField.vue'
import {pendingTx} from './pendingTx'

type ConfirmTab = 'eip712' | 'raw'

const tab = ref<ConfirmTab>('raw')

const title = computed(() => {
  const value = pendingTx.value?.title
  return typeof value === 'string' && value ? value : 'Confirm transaction'
})

const isTypedData = computed(() => pendingTx.value?.kind === 'typedData')

const fields = computed((): ConfirmFieldNode[] => readFields(pendingTx.value?.fields))

const hasTypedFields = computed(() => fields.value.length > 0)

const rawJson = computed(() => {
  const value = pendingTx.value?.rawJSON
  return typeof value === 'string' && value ? value : '{}'
})

watch([isTypedData, hasTypedFields], ([typed, hasFields]) => {
  tab.value = typed || hasFields ? 'eip712' : 'raw'
}, {immediate: true})

const offPending = Events.On('pendingTransaction', (event) => {
  pendingTx.value = asPending(event.data)
})

onUnmounted(() => {
  offPending()
})

function asPending(data: unknown): Record<string, unknown> {
  let value: unknown = data
  if (typeof value === 'string') {
    try {
      value = JSON.parse(value)
    } catch {
      return {}
    }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }
  return value as Record<string, unknown>
}

function readFields(value: unknown): ConfirmFieldNode[] {
  if (typeof value === 'string') {
    try {
      value = JSON.parse(value)
    } catch {
      return []
    }
  }
  if (Array.isArray(value)) {
    return value.map(normalizeField).filter((field): field is ConfirmFieldNode => field !== null)
  }
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    const keys = Object.keys(record)
    if (keys.length && keys.every((key) => /^\d+$/.test(key))) {
      return Object.values(record)
        .map(normalizeField)
        .filter((field): field is ConfirmFieldNode => field !== null)
    }
    const one = normalizeField(record)
    return one ? [one] : []
  }
  return []
}

function str(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function fieldText(value: unknown): string | undefined {
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' || typeof value === 'bigint' || typeof value === 'boolean') {
    return String(value)
  }
  return undefined
}

function normalizeField(raw: unknown): ConfirmFieldNode | null {
  if (!raw || typeof raw !== 'object') {
    return null
  }
  const node = raw as Record<string, unknown>
  const name = str(node.name) ?? str(node.Name) ?? ''
  const nested = readFields(node.fields ?? node.Fields)
  return {
    name,
    type: str(node.type) ?? str(node.Type),
    text: fieldText(node.text) ?? fieldText(node.Text),
    fields: nested.length ? nested : undefined,
  }
}

function reportError(err: unknown) {
  console.error(err)
  if (typeof window !== 'undefined') {
    window.alert(String(err))
  }
}

async function confirm() {
  try {
    await Events.Emit('confirm-yes')
  } catch (err) {
    reportError(err)
  }
}

async function cancel() {
  try {
    await Events.Emit('confirm-no')
  } catch (err) {
    reportError(err)
  }
}
</script>

<template>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
    <header class="dialog-header">
      <h2 id="confirm-title" class="confirm-title">{{ title }}</h2>
      <nav class="tabs" role="tablist" aria-label="Confirm view">
        <button
          v-if="isTypedData || hasTypedFields"
          class="tab"
          type="button"
          role="tab"
          :class="{ 'is-active': tab === 'eip712' }"
          :aria-selected="tab === 'eip712'"
          @click="tab = 'eip712'"
        >
          EIP-712
        </button>
        <button
          class="tab"
          type="button"
          role="tab"
          :class="{ 'is-active': tab === 'raw' }"
          :aria-selected="tab === 'raw'"
          @click="tab = 'raw'"
        >
          Raw
        </button>
      </nav>
    </header>
    <div v-show="tab === 'eip712'" class="typed-pane" role="tabpanel">
      <ul v-if="fields.length" class="field-list">
        <ConfirmField v-for="(field, i) in fields" :key="i" :field="field" />
      </ul>
      <p v-else class="typed-empty muted">No typed-data fields.</p>
    </div>
    <JsonBlock v-show="tab === 'raw'" class="tx-json" role="tabpanel" :json="rawJson" />
    <div class="confirm-actions">
      <button class="copy" type="button" @click="cancel">Cancel</button>
      <button class="btn-primary" type="button" @click="confirm">Confirm</button>
    </div>
  </div>
</template>

<style lang="css" scoped>
.dialog {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  background: var(--fill);
  color: var(--text);
  border: 1px solid var(--border);
}

.dialog-header {
  --wails-draggable: drag;
  padding: var(--s-2) var(--s-3) 0;
}

.confirm-title {
  margin: 0;
  font-weight: 600;
  color: var(--text);
}

.tabs {
  margin-top: var(--s-1);
}

.typed-pane,
.tx-json,
.confirm-actions,
.field-list,
.typed-empty {
  --wails-draggable: no-drag;
}

.typed-pane,
.tx-json {
  flex: 1;
  min-height: 0;
  max-height: none;
  margin: var(--s-3);
}

.field-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.typed-empty {
  margin: 0;
}

.typed-pane {
  overflow: auto;
  padding: var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--fill);
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-1);
  padding: 0 var(--s-3) var(--s-3);
}
</style>
