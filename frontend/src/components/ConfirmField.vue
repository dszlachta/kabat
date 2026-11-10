<script lang="ts">
export default {name: 'ConfirmField'}
</script>

<script setup lang="ts">
export type ConfirmFieldNode = {
  name: string
  type?: string
  text?: string
  fields?: ConfirmFieldNode[]
}

defineProps<{
  field: ConfirmFieldNode
}>()
</script>

<template>
  <li class="field">
    <div class="field-head">
      <span class="field-name">{{ field.name }}</span>
      <code v-if="field.type" class="type field-type muted">{{ field.type }}</code>
    </div>
    <p v-if="field.text != null && field.text !== ''" class="field-text">{{ field.text }}</p>
    <ul v-if="field.fields?.length" class="field-list">
      <ConfirmField v-for="(child, i) in field.fields" :key="i" :field="child" />
    </ul>
  </li>
</template>

<style lang="css" scoped>
.field-list,
.field,
.field-head,
.field-name,
.field-type,
.field-text {
  --wails-draggable: no-drag;
}

.field-list {
  list-style: none;
  margin: var(--s-1) 0 0;
  padding: 0 0 0 var(--s-2);
  border-left: 1px solid var(--border);
}

.field {
  padding: var(--s-1) 0;
  border-bottom: 1px solid var(--border);
}

.field:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.field-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--s-1);
}

.field-name {
  font-weight: 600;
}

.field-text {
  margin: 0.25rem 0 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: inherit;
  line-height: 1.45;
  word-break: break-all;
  -webkit-user-select: text;
  user-select: text;
}
</style>
