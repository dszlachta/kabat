<script setup lang="ts">
export type TxFilterValue = 'all' | 'final' | 'pending'

defineProps<{
  modelValue: TxFilterValue
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TxFilterValue): void
}>()

// We do not fetch pending (mempool) txs yet.
</script>

<template>
  <nav class="tabs tx-filter" role="tablist" aria-label="Transaction filter">
    <button
      class="tab"
      type="button"
      role="tab"
      :class="{ 'is-active': modelValue === 'all' }"
      :aria-selected="modelValue === 'all'"
      @click="emit('update:modelValue', 'all')"
    >
      All
    </button>
    <button
      class="tab"
      type="button"
      role="tab"
      :class="{ 'is-active': modelValue === 'final' }"
      :aria-selected="modelValue === 'final'"
      @click="emit('update:modelValue', 'final')"
    >
      Final
    </button>
    <!-- Pending txs are not fetched yet. -->
    <button
      class="tab"
      type="button"
      role="tab"
      :class="{ 'is-active': modelValue === 'pending' }"
      :aria-selected="modelValue === 'pending'"
      disabled
      @click="emit('update:modelValue', 'pending')"
    >
      Pending
    </button>
  </nav>
</template>

<style lang="css" scoped>
.tx-filter {
  display: none;
  --wails-draggable: no-drag;
  flex-shrink: 0;
}
</style>
