<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {WalletService} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'

const anvilURL = ref('')
const defaultAnvilURL = ref('http://127.0.0.1:8545')
const mcp = ref(false)
const mcpURL = ref('')

const isDefaultURL = computed(() => anvilURL.value.trim() === defaultAnvilURL.value)

async function load() {
  const settings = await WalletService.Settings()
  anvilURL.value = settings.anvilUrl
  defaultAnvilURL.value = settings.defaultAnvilUrl
  mcp.value = settings.mcp
  mcpURL.value = settings.mcpUrl
}

onMounted(() => {
  void load()
})

function reportError(err: unknown) {
  console.error(err)
  if (typeof window !== 'undefined') {
    window.alert(String(err))
  }
}

async function saveURL() {
  try {
    await WalletService.SetAnvilURL(anvilURL.value)
    await load()
  } catch (err) {
    reportError(err)
    await load()
  }
}

async function resetURL() {
  try {
    await WalletService.SetAnvilURL('')
    await load()
  } catch (err) {
    reportError(err)
  }
}

async function saveMCP() {
  try {
    await WalletService.SetMCP(mcp.value)
  } catch (err) {
    reportError(err)
    mcp.value = !mcp.value
  }
}
</script>

<template>
  <div class="app">
    <header class="chrome">
      <h1 class="title">Settings</h1>
    </header>
    <form class="fields" @submit.prevent="saveURL">
      <div class="field">
        <span class="muted" id="anvil-url-label">Anvil URL</span>
        <span class="url-row">
          <input
            v-model="anvilURL"
            class="address"
            type="text"
            spellcheck="false"
            autocomplete="off"
            :placeholder="defaultAnvilURL"
            aria-labelledby="anvil-url-label"
            @keydown.enter.prevent="saveURL"
            @blur="saveURL"
          >
          <button class="copy" type="button" :disabled="isDefaultURL" @click="resetURL">
            Reset
          </button>
        </span>
      </div>
      <div class="field">
        <label class="toggle">
          <span class="muted">MCP server</span>
          <input
            v-model="mcp"
            type="checkbox"
            @change="saveMCP"
          >
        </label>
        <code v-if="mcp" class="address mcp-url">{{ mcpURL }}</code>
      </div>
    </form>
  </div>
</template>

<style lang="css" scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
}

.chrome {
  display: flex;
  align-items: center;
  height: 50px;
  padding: 0 var(--s-2) 0 78px;
  --wails-draggable: drag;
}

.title {
  margin: 0;
  font-weight: 600;
}

.fields,
.field,
.url-row,
input,
button {
  --wails-draggable: no-drag;
}

.fields {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
  padding: var(--s-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  font-weight: 600;
}

.field span.muted {
  font-size: 0.85em;
}

.url-row {
  display: flex;
  gap: var(--s-1);
  min-width: 0;
}

.url-row input {
  flex: 1;
  min-width: 0;
  font-weight: 400;
}

.toggle {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.toggle input {
  margin: 0;
}

.mcp-url {
  font-weight: 400;
}
</style>
