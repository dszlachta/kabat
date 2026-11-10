<script setup lang="ts">
import type {EIP712Info} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'

defineProps<{
  eip712: EIP712Info | null | undefined
}>()

function typeNames(eip712: EIP712Info | null | undefined): string[] {
  const types = eip712?.types
  if (!types) {
    return []
  }
  const names = Object.keys(types)
  const primary = eip712?.primaryType
  names.sort((a, b) => {
    if (a === 'EIP712Domain') {
      return -1
    }
    if (b === 'EIP712Domain') {
      return 1
    }
    if (a === primary) {
      return -1
    }
    if (b === primary) {
      return 1
    }
    return a.localeCompare(b)
  })
  return names
}
</script>

<template>
  <div class="eip712-pane" role="tabpanel" aria-label="EIP-712">
    <p v-if="!eip712?.types || !Object.keys(eip712.types).length" class="eip712-empty muted">
      No EIP-712 types.
    </p>
    <template v-else>
      <p v-if="eip712.primaryType" class="eip712-primary">
        <span class="muted">Primary type</span>
        <code class="type">{{ eip712.primaryType }}</code>
      </p>
      <section
        v-for="name in typeNames(eip712)"
        :key="name"
        class="eip712-type"
      >
        <h3 class="eip712-type-name">{{ name }}</h3>
        <ul class="eip712-fields">
          <li
            v-for="(field, i) in eip712.types?.[name] ?? []"
            :key="name + '-' + i"
            class="eip712-field"
          >
            <code class="type">{{ field.type }}</code>
            <span v-if="field.name">{{ field.name }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style lang="css" scoped>
.eip712-pane,
.eip712-empty,
.eip712-primary,
.eip712-type,
.eip712-type-name,
.eip712-fields,
.eip712-field {
  --wails-draggable: no-drag;
}

.eip712-pane {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.eip712-empty {
  margin: 0;
}

.eip712-primary {
  display: flex;
  align-items: baseline;
  gap: var(--s-1);
  margin: 0;
  font-size: 0.85em;
}

.eip712-type {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: var(--s-2) 0;
  border-bottom: 1px solid var(--border);
}

.eip712-type-name {
  margin: 0;
  font-size: 0.85em;
  font-weight: 700;
}

.eip712-fields {
  margin: 0;
  padding: 0;
  list-style: none;
}

.eip712-field {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-size: 0.85em;
  padding: 0.15rem 0;
}
</style>
