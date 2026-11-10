<script setup lang="ts">
import type {StoredContract} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'

defineProps<{
  contract: StoredContract
  selected?: boolean
}>()

const emit = defineEmits<{
  (e: 'select', hash: string): void
}>()
</script>

<template>
  <li
    class="contract"
    :class="{ 'is-selected': selected }"
    @click="emit('select', contract.hash)"
  >
    <code class="address">{{ contract.address }}</code>
    <small class="contract-meta muted">{{ contract.blockNumber }} {{ contract.hash }}</small>
  </li>
</template>

<style lang="css" scoped>
.contract,
.contract-meta {
  --wails-draggable: no-drag;
}

.contract {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: var(--s-2);
  border-bottom: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
}

.contract.is-selected {
  background: var(--selected-fill);
}

.contract .address {
  max-width: 100%;
}

.contract-meta {
  font-size: 0.85em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
