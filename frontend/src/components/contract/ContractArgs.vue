<script setup lang="ts">
import type {TxArg} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'

defineProps<{
  args: TxArg[]
  label?: string
}>()
</script>

<template>
  <ul class="contract-args" :aria-label="label">
    <li v-for="(arg, i) in args" :key="i" class="contract-arg">
      <span class="contract-arg-label">
        <code v-if="arg.type" class="type">{{ arg.type }}</code>
        <span v-if="arg.name">{{ arg.name }}</span>
      </span>
      <code class="address">{{ arg.text }}</code>
    </li>
  </ul>
</template>

<style lang="css" scoped>
.contract-args,
.contract-arg,
.contract-arg-label {
  --wails-draggable: no-drag;
}

.contract-args {
  list-style: none;
  margin: 0;
  padding: 0;
}

.contract-arg {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.25rem 0;
  border-bottom: 1px solid var(--border);
}

.contract-arg-label {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-weight: 600;
  font-size: 0.85em;
}
</style>
