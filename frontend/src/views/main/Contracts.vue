<script setup lang="ts">
import ContractList from '../../components/contract/ContractList.vue'

defineProps<{
  focusAddress?: string
}>()

const emit = defineEmits<{
  (e: 'focused'): void
  (e: 'select-tx', hash: string): void
}>()
</script>

<template>
  <section class="tab-panel" role="tabpanel">
    <ContractList
      :focus-address="focusAddress"
      @focused="emit('focused')"
      @select-tx="emit('select-tx', $event)"
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
