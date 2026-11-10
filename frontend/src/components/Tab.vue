<script setup lang="ts">
export type TabId = 'wallets' | 'txs' | 'contracts'

const tabs: { id: TabId; label: string }[] = [
  {id: 'wallets', label: 'Wallets'},
  {id: 'txs', label: 'Txs'},
  {id: 'contracts', label: 'Contracts'},
]

defineProps<{
  modelValue: TabId
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', id: TabId): void
}>()
</script>

<template>
  <nav class="tabs" role="tablist" aria-label="Main">
    <button
      v-for="item in tabs"
      :key="item.id"
      class="tab"
      type="button"
      role="tab"
      :class="{ 'is-active': modelValue === item.id }"
      :aria-selected="modelValue === item.id"
      @click="emit('update:modelValue', item.id)"
    >
      {{ item.label }}
    </button>
  </nav>
</template>
