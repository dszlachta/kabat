<script setup lang="ts">
import {ref} from 'vue'
import {CheckIcon, CopyIcon} from '@lucide/vue'

const props = defineProps<{
  json: string
}>()

const copied = ref(false)

async function copyJSON() {
  if (!props.json) {
    return
  }
  await navigator.clipboard.writeText(props.json)
  copied.value = true
  window.setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <div class="json-block">
    <button
      v-if="json"
      class="icon-btn json-copy"
      type="button"
      :aria-label="copied ? 'Copied' : 'Copy JSON'"
      @click="copyJSON"
    >
      <CheckIcon v-if="copied" class="icon" />
      <CopyIcon v-else class="icon" />
    </button>
    <pre class="pre json-pre">{{ json }}</pre>
  </div>
</template>

<style lang="css" scoped>
.json-block {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.json-copy {
  position: absolute;
  top: var(--s-1);
  right: var(--s-1);
  z-index: 1;
  opacity: 0;
  pointer-events: none;
}

.json-block:hover > .json-copy {
  opacity: 1;
  pointer-events: auto;
}

.json-pre {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
}
</style>
