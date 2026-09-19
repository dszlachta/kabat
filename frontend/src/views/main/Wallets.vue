<script setup lang="ts">
import {computed, nextTick, onMounted, onUnmounted, ref, watch} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {Account, DappMessage, ERC20Token} from '../../../bindings/github.com/dszlachta/kabat/internal/walletservice'
import {formatEtherExact, formatEtherList, parseEtherExact} from '../../formatEther'
import {AppWindowMacIcon, ArrowLeftIcon, ArrowRightIcon, CheckIcon, ChevronsLeftRightEllipsisIcon, CopyIcon, MailIcon, MinusIcon, PencilIcon, PlusIcon, SendIcon, SquareArrowRightIcon, UnplugIcon} from '@lucide/vue'
import TxList from '../../components/tx/TxList.vue'
import JsonBlock from '../../components/JsonBlock.vue'

const emit = defineEmits<{
  (e: 'select-tx', hash: string): void
  (e: 'select-contract', address: string): void
  (e: 'see-all'): void
}>()

const ANVIL_ERROR = 'cannot connect to local Anvil chain'

const loading = ref(true)
const error = ref('')
const accounts = ref<Account[]>([])
const copied = ref('')
const detailTab = ref<'txs' | 'tokens' | 'dapps'>('txs')
const tokens = ref<ERC20Token[]>([])
const nameDraft = ref('')
const editingName = ref(false)
const nameInput = ref<HTMLInputElement | null>(null)
const sendDialog = ref<HTMLDialogElement | null>(null)
const sendTo = ref('')
const sendEth = ref('')
const sendToken = ref('')
const sendToTouched = ref(false)
const sendEthTouched = ref(false)
const sendError = ref<string | null>(null)
const importDialog = ref<HTMLDialogElement | null>(null)
const importKeyInput = ref<HTMLInputElement | null>(null)
const importKey = ref('')
const importKeyTouched = ref(false)
const importKeyError = ref('')

const selected = computed(() => accounts.value.find((account) => account.selected))
const walletAddresses = computed(() => accounts.value.map((account) => account.address))
const otherWallets = computed(() => accounts.value.filter((account) => !account.selected))
const dappCount = computed(() => selected.value?.connections?.length ?? 0)
const dappMessages = ref<DappMessage[]>([])
const selectedDappMessages = computed(() => {
  const sessions = new Set((selected.value?.connections ?? []).map((dapp) => dapp.sessionUlid))
  return dappMessages.value.filter((msg) => sessions.has(msg.sessionUlid)).slice().reverse()
})

watch(dappCount, (count) => {
  if (count === 0 && detailTab.value === 'dapps') {
    detailTab.value = 'txs'
  }
})

watch(() => selected.value?.address, (address) => {
  editingName.value = false
  nameDraft.value = selected.value?.name ?? ''
  if (address) {
    void loadTokens()
  } else {
    tokens.value = []
  }
}, {immediate: true})

function isHexAddress(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim())
}

function shortAddr(address: string): string {
  return address.slice(0, 5) + '…'
}

function isDecimalAmount(value: string): boolean {
  const s = value.trim()
  if (!s || s.startsWith('-') || s === '.' || (s.match(/\./g) ?? []).length > 1) {
    return false
  }
  const [wholeRaw, fracRaw = ''] = s.split('.')
  if (wholeRaw !== '' && !/^[0-9]+$/.test(wholeRaw)) {
    return false
  }
  if (fracRaw !== '' && !/^[0-9]+$/.test(fracRaw)) {
    return false
  }
  return wholeRaw !== '' || fracRaw !== ''
}

const sendTokenMeta = computed(() =>
  tokens.value.find((token) => token.address === sendToken.value)
)
const sendAmountLabel = computed(() => {
  if (!sendToken.value) {
    return 'Native (ETH)'
  }
  return sendTokenMeta.value?.symbol || 'Token'
})

function sendErrorText(err: unknown): string {
  if (err && typeof err === 'object' && 'cause' in err && typeof err.cause === 'string' && err.cause) {
    return err.cause
  }
  const text = err instanceof Error ? err.message : String(err)
  const marker = 'Bound method returned an error: '
  const i = text.indexOf(marker)
  if (i >= 0) {
    return text.slice(i + marker.length)
  }
  return text.replace(/^ERR\s+/, '')
}

const sendToError = computed(() => {
  const value = sendTo.value.trim()
  if (value === '') {
    return 'To is required'
  }
  if (!isHexAddress(value)) {
    return 'To must be a 0x address'
  }
  return ''
})
const sendEthError = computed(() => {
  if (sendEth.value.trim() === '') {
    return `${sendAmountLabel.value} is required`
  }
  if (!sendToken.value) {
    try {
      parseEtherExact(sendEth.value)
    } catch {
      return `${sendAmountLabel.value} must be a decimal amount`
    }
    return ''
  }
  if (!isDecimalAmount(sendEth.value)) {
    return `${sendAmountLabel.value} must be a decimal amount`
  }
  return ''
})
const sendValidationError = computed(() => sendToError.value || sendEthError.value)
const displayedSendToError = computed(() =>
  sendToTouched.value ? sendToError.value : ''
)
const displayedSendEthError = computed(() =>
  sendEthTouched.value ? sendEthError.value : ''
)
const canSend = computed(() => !sendValidationError.value)

const importKeyFieldError = computed(() => {
  if (importKey.value.trim() === '') {
    return 'Private key is required'
  }
  return ''
})
const displayedImportKeyError = computed(() =>
  importKeyTouched.value ? importKeyFieldError.value || importKeyError.value : importKeyError.value
)
const canImportKey = computed(() => importKey.value.trim() !== '')

let offAccounts: (() => void) | undefined
let offDappMessages: (() => void) | undefined
let offImportKey: (() => void) | undefined

async function load() {
  loading.value = true
  error.value = ''
  try {
    accounts.value = await WalletService.Accounts() ?? []
    dappMessages.value = await WalletService.DappMessages() ?? []
    await loadTokens()
  } catch (err) {
    accounts.value = []
    dappMessages.value = []
    tokens.value = []
    error.value = ANVIL_ERROR
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function loadTokens() {
  try {
    tokens.value = await WalletService.Tokens() ?? []
  } catch {
    tokens.value = []
  }
}

async function copyAddress(address: string) {
  await navigator.clipboard.writeText(address)
  copied.value = address
  window.setTimeout(() => {
    if (copied.value === address) {
      copied.value = ''
    }
  }, 1500)
}

async function selectAccount(address: string) {
  await WalletService.Select(address)
  accounts.value = await WalletService.Accounts() ?? []
}

async function disconnect(sessionUlid: string) {
  await WalletService.Disconnect(sessionUlid)
  accounts.value = await WalletService.Accounts() ?? []
}

async function startEditName() {
  nameDraft.value = selected.value?.name ?? ''
  editingName.value = true
  await nextTick()
  nameInput.value?.focus()
  nameInput.value?.select()
}

function cancelName() {
  nameDraft.value = selected.value?.name ?? ''
  editingName.value = false
}

async function saveName() {
  if (!editingName.value || !selected.value) {
    return
  }
  const next = nameDraft.value.trim()
  editingName.value = false
  if (next === (selected.value.name ?? '')) {
    nameDraft.value = next
    return
  }
  try {
    await WalletService.SetAccountName(selected.value.address, next)
    nameDraft.value = next
  } catch (err) {
    console.error(err)
    editingName.value = true
  }
}

function resetSendForm() {
  sendTo.value = ''
  sendEth.value = ''
  sendToken.value = ''
  sendToTouched.value = false
  sendEthTouched.value = false
  sendError.value = null
}

async function openSend() {
  resetSendForm()
  sendDialog.value?.showModal()
  await loadTokens()
}

function closeSend() {
  sendDialog.value?.close()
}

function openAddMenu(event: MouseEvent) {
  const el = event.currentTarget as HTMLElement
  el.dispatchEvent(new MouseEvent('contextmenu', {
    bubbles: true,
    cancelable: true,
    clientX: event.clientX,
    clientY: event.clientY,
    view: window,
  }))
}

function resetImportForm() {
  importKey.value = ''
  importKeyTouched.value = false
  importKeyError.value = ''
}

async function openImportKey() {
  resetImportForm()
  importDialog.value?.showModal()
  await nextTick()
  importKeyInput.value?.focus()
}

function closeImportKey() {
  importDialog.value?.close()
}

async function addWalletFromKey() {
  importKeyTouched.value = true
  importKeyError.value = ''
  if (!canImportKey.value) {
    return
  }
  try {
    await WalletService.AddWalletFromKey(importKey.value)
    resetImportForm()
    closeImportKey()
  } catch (err) {
    importKeyError.value = sendErrorText(err)
  }
}

async function sendETH() {
  if (!canSend.value) {
    return
  }
  sendError.value = null
  try {
    const to = sendTo.value.trim()
    const hash = sendToken.value
      ? await WalletService.SendERC20(sendToken.value, to, sendEth.value.trim())
      : await WalletService.SendETH(to, parseEtherExact(sendEth.value))
    resetSendForm()
    closeSend()
    if (hash) {
      emit('select-tx', hash)
    }
  } catch (err) {
    sendError.value = sendErrorText(err)
  }
}

onMounted(() => {
  load()
  offAccounts = Events.On('accounts', (event) => {
    if (Array.isArray(event.data)) {
      accounts.value = event.data
      error.value = ''
      void loadTokens()
    }
  })
  offDappMessages = Events.On('dappMessages', (event) => {
    if (Array.isArray(event.data)) {
      dappMessages.value = event.data
    }
  })
  offImportKey = Events.On('import-key', () => {
    void openImportKey()
  })
})

onUnmounted(() => {
  offAccounts?.()
  offDappMessages?.()
  offImportKey?.()
})
</script>

<template>
  <section class="tab-panel" role="tabpanel">
    <p v-if="loading" class="status">Connecting…</p>

    <div v-else-if="error" class="error" role="alert">
      <p class="error-message">{{ error }}</p>
      <button class="btn" type="button" @click="load">Retry</button>
    </div>

    <div v-else class="split">
      <aside class="wallet-list-pane">
        <ul class="wallets" aria-label="Anvil wallets">
          <li
            v-for="account in accounts"
            :key="account.address"
            class="wallet"
            :class="{ 'is-selected': account.selected }"
            @click="selectAccount(account.address)"
          >
            <div class="wallet-identity">
              <div class="wallet-id">
                <span v-if="account.name" class="wallet-list-name">{{ account.name }}</span>
                <code class="address muted">{{ account.address }}</code>
              </div>
              <button
                class="icon-btn"
                type="button"
                :aria-label="copied === account.address ? 'Copied' : 'Copy ' + account.address"
                @click.stop="copyAddress(account.address)"
              >
                <CheckIcon v-if="copied === account.address" class="icon" />
                <CopyIcon v-else class="icon" />
              </button>
            </div>
            <div class="wallet-balance-row">
              <ChevronsLeftRightEllipsisIcon
                v-if="account.connected > 0"
                class="icon"
              />
              <small class="balance">{{ formatEtherList(account.balance) }} ETH</small>
            </div>
          </li>
        </ul>
        <div class="wallet-list-actions">
          <button
            class="icon-btn"
            type="button"
            aria-label="Add"
            aria-haspopup="menu"
            style="--custom-contextmenu: add-wallet"
            @click="openAddMenu"
          >
            <PlusIcon class="icon" />
          </button>
          <button class="icon-btn" type="button" aria-label="Remove">
            <MinusIcon class="icon" />
          </button>
        </div>
        <dialog
          ref="importDialog"
          class="send-dialog"
          aria-labelledby="import-key-title"
          @close="resetImportForm"
        >
          <form class="send-eth" @submit.prevent="addWalletFromKey">
            <header class="dialog-header">
              <h2 id="import-key-title" class="send-title">From private key</h2>
            </header>
            <label class="send-field">
              <span class="send-label muted">
                Private key
                <span class="send-required" title="Required" aria-hidden="true">*</span>
              </span>
              <input
                ref="importKeyInput"
                v-model="importKey"
                class="address"
                spellcheck="false"
                autocomplete="off"
                :aria-required="true"
                @input="importKeyTouched = true; importKeyError = ''"
                @blur="importKeyTouched = true"
              >
              <p class="send-error">{{ displayedImportKeyError }}</p>
            </label>
            <div class="send-actions">
              <button class="copy" type="button" @click="closeImportKey">Cancel</button>
              <button class="btn-primary" type="submit" :disabled="!canImportKey">Add</button>
            </div>
          </form>
        </dialog>
      </aside>

      <section v-if="selected" class="wallet-detail" :aria-label="selected.address">
        <header class="wallet-detail-header">
          <input
            v-if="editingName"
            ref="nameInput"
            class="wallet-name"
            v-model="nameDraft"
            aria-label="Wallet name"
            @keydown.enter.prevent="saveName"
            @keydown.escape.prevent="cancelName"
            @blur="saveName"
          >
          <template v-else>
            <h2
              class="wallet-name-label"
              :class="{ address: !selected.name }"
            >{{ selected.name || selected.address }}</h2>
            <button
              class="icon-btn"
              type="button"
              aria-label="Edit name"
              @click="startEditName"
            >
              <PencilIcon class="icon" />
            </button>
          </template>
        </header>
        <dl class="wallet-fields">
          <div class="wallet-field">
            <dt class="muted">Address</dt>
            <dd>
              <div class="wallet-detail-address">
                <code class="address">{{ selected.address }}</code>
                <button
                  class="icon-btn"
                  type="button"
                  :aria-label="copied === selected.address ? 'Copied' : 'Copy ' + selected.address"
                  @click="copyAddress(selected.address)"
                >
                  <CheckIcon v-if="copied === selected.address" class="icon" />
                  <CopyIcon v-else class="icon" />
                </button>
              </div>
            </dd>
          </div>
          <div class="wallet-field">
            <dt class="muted">Balance</dt>
            <dd class="wallet-detail-actions">
              <p class="wallet-detail-balance">{{ formatEtherExact(selected.balance) }} ETH</p>
              <button class="btn" type="button" @click="openSend">
                <SendIcon class="icon" />
                Send
              </button>
            </dd>
          </div>
        </dl>
        <dialog
          ref="sendDialog"
          class="send-dialog"
          aria-labelledby="send-title"
          @close="resetSendForm"
        >
          <form class="send-eth" @submit.prevent="sendETH">
            <header class="dialog-header">
              <h2 id="send-title" class="send-title">Send</h2>
            </header>
            <label class="send-field">
              <span class="send-label muted">Token</span>
              <select v-model="sendToken">
                <option value="">Native (ETH)</option>
                <option
                  v-for="token in tokens"
                  :key="token.address"
                  :value="token.address"
                >
                  {{ token.symbol || 'Token' }} ({{ shortAddr(token.address) }})
                </option>
              </select>
            </label>
            <label class="send-field">
              <span class="send-label muted">
                To
                <span class="send-required" title="Required" aria-hidden="true">*</span>
              </span>
              <input
                v-model="sendTo"
                list="send-to-wallets"
                :aria-required="true"
                @input="sendToTouched = true"
                @blur="sendToTouched = true"
              >
              <datalist id="send-to-wallets">
                <option
                  v-for="account in otherWallets"
                  :key="account.address"
                  :value="account.address"
                  :label="account.name || account.address"
                />
              </datalist>
              <p class="send-error">{{ displayedSendToError }}</p>
            </label>
            <label class="send-field">
              <span class="send-label muted">
                {{ sendAmountLabel }}
                <span class="send-required" title="Required" aria-hidden="true">*</span>
              </span>
              <input
                v-model="sendEth"
                :aria-required="true"
                @input="sendEthTouched = true"
                @blur="sendEthTouched = true"
              >
              <p v-if="sendTokenMeta" class="muted send-balance">
                Balance {{ sendTokenMeta.balance || '0' }} {{ sendAmountLabel }}
              </p>
              <p class="send-error">{{ displayedSendEthError }}</p>
            </label>
            <pre class="send-result" :class="{ 'is-empty': sendError === null }">{{ sendError }}</pre>
            <div class="send-actions">
              <button class="copy" type="button" @click="closeSend">Cancel</button>
              <button class="btn-primary" type="submit" :disabled="!canSend">
                <SendIcon class="icon" />
                Send
              </button>
            </div>
          </form>
        </dialog>
        <nav class="tabs wallet-detail-tabs" role="tablist" aria-label="Wallet details">
          <button
            class="tab"
            type="button"
            role="tab"
            :class="{ 'is-active': detailTab === 'txs' }"
            :aria-selected="detailTab === 'txs'"
            @click="detailTab = 'txs'"
          >
            Txs
          </button>
          <button
            class="tab"
            type="button"
            role="tab"
            :class="{ 'is-active': detailTab === 'tokens' }"
            :aria-selected="detailTab === 'tokens'"
            @click="detailTab = 'tokens'"
          >
            Tokens
          </button>
          <button
            class="tab"
            type="button"
            role="tab"
            :class="{ 'is-active': detailTab === 'dapps' }"
            :aria-selected="detailTab === 'dapps'"
            :disabled="dappCount === 0"
            @click="detailTab = 'dapps'"
          >
            {{ dappCount > 0 ? `Dapps (${dappCount})` : 'Dapps' }}
          </button>
        </nav>
        <div v-if="detailTab === 'txs'" class="wallet-detail-tab-panel" role="tabpanel">
          <TxList
            :address="selected.address"
            :wallets="walletAddresses"
            :show-detail="false"
            @select-wallet="selectAccount"
            @select-tx="emit('select-tx', $event)"
            @see-all="emit('see-all')"
          />
        </div>
        <div v-else-if="detailTab === 'tokens'" class="wallet-detail-tab-panel" role="tabpanel">
          <p v-if="tokens.length === 0" class="muted token-empty">No tokens</p>
          <div v-else class="token-list-wrap">
            <table class="token-list" aria-label="Tokens">
              <colgroup>
                <col class="token-col-symbol">
                <col class="token-col-address">
                <col class="token-col-balance">
              </colgroup>
              <thead>
                <tr>
                  <th scope="col">Symbol</th>
                  <th scope="col">Address</th>
                  <th scope="col">Balance</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="token in tokens" :key="token.address">
                  <td>{{ token.symbol || 'Token' }}</td>
                  <td>
                    <button
                      class="hex-link token-addr-btn"
                      type="button"
                      :title="token.address"
                      :aria-label="'Open contract ' + token.address"
                      @click="emit('select-contract', token.address)"
                    >
                      <SquareArrowRightIcon class="icon" />
                      <span class="address">{{ token.address }}</span>
                    </button>
                  </td>
                  <td class="token-balance">{{ token.balance || '0' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div v-else class="wallet-detail-tab-panel" role="tabpanel">
          <div class="dapp-list-wrap">
            <table class="dapp-list" aria-label="Connected dapps">
              <colgroup>
                <col class="dapp-col-url">
                <col class="dapp-col-session">
                <col class="dapp-col-action">
              </colgroup>
              <thead>
                <tr>
                  <th scope="col">URL</th>
                  <th scope="col">Session</th>
                  <th scope="col" aria-label="Disconnect"></th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="dapp in selected.connections ?? []"
                  :key="dapp.sessionUlid"
                >
                  <td><code class="address">{{ dapp.origin }}</code></td>
                  <td><code class="address">{{ dapp.sessionUlid }}</code></td>
                  <td>
                    <button
                      class="icon-btn"
                      type="button"
                      :aria-label="'Disconnect ' + dapp.origin"
                      @click="disconnect(dapp.sessionUlid)"
                    >
                      <UnplugIcon class="icon" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <ul class="dapp-msg-list" aria-label="Dapp messages">
            <li
              v-for="(msg, index) in selectedDappMessages"
              :key="msg.ulid + '-' + msg.direction + '-' + index"
              class="dapp-msg"
              :class="msg.direction === 'in' ? 'is-in' : 'is-out'"
            >
              <ArrowRightIcon
                v-if="msg.direction === 'in'"
                class="dapp-msg-icon"
                title="Incoming"
                aria-label="Incoming"
              />
              <ArrowLeftIcon
                v-else
                class="dapp-msg-icon"
                title="Outgoing"
                aria-label="Outgoing"
              />
              <code class="dapp-msg-method">{{ msg.method }}</code>
              <code v-if="msg.error" class="dapp-msg-params">{{ msg.error }}</code>
              <JsonBlock v-else-if="msg.payloadJson" class="dapp-msg-params" :json="msg.payloadJson" />
              <small class="dapp-msg-ids muted">                
                <span class="dapp-msg-id">
                  <MailIcon
                    class="dapp-msg-id-icon"
                    title="Message ID"
                    aria-label="Message ID"
                  />
                  <code>{{ msg.ulid }}</code>
                </span>
                <span class="dapp-msg-id">
                  <AppWindowMacIcon
                    class="dapp-msg-id-icon"
                    title="Session"
                    aria-label="Session"
                  />
                  <code>{{ msg.sessionUlid }}</code>
                </span>
              </small>
            </li>
          </ul>
        </div>
      </section>
    </div>
  </section>
</template>

<style lang="css" scoped>
.tab-panel,
.wallet-list-pane,
.wallet-detail,
.wallets,
.wallet,
.wallet-list-actions,
.icon-btn,
.error,
.wallet-detail-balance,
.wallet-balance-row,
.dapp-list-wrap,
.dapp-list,
.dapp-msg-list,
.dapp-msg,
.wallet-detail-tabs,
.wallet-detail-tab-panel,
.wallet-name,
.wallet-name-label,
.wallet-fields,
.wallet-field,
.wallet-detail-actions,
.send-dialog,
.dialog-header,
.send-eth,
.send-field,
.send-error,
.send-result,
.icon {
  --wails-draggable: no-drag;
}

.tab-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.status {
  margin: var(--s-3);
  color: var(--text);
}

.error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--s-2);
  margin: var(--s-3);
  padding: var(--s-3);
  background: var(--fill);
  border: 1px solid var(--border);
  color: var(--text);
}

.error-message {
  margin: 0;
  color: var(--text);
}

.split {
  display: flex;
  flex: 1;
  min-height: 0;
}

.wallet-list-pane {
  display: flex;
  flex-direction: column;
  width: 36%;
  min-width: 16rem;
  max-width: 24rem;
  border-right: 1px solid var(--border);
}

.wallets {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.wallet {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: var(--s-2);
  border-bottom: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
}

.wallet.is-selected {
  background: var(--selected-fill);
}

.wallet-identity {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
  width: 100%;
}

.wallet-id {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  min-width: 0;
  flex: 1;
}

.wallet-list-name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.balance {
  color: var(--text);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.wallet-balance-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--s-1);
  width: 100%;
}

.wallet-list-actions {
  display: flex;
  gap: var(--s-1);
  padding: var(--s-1) var(--s-2);
  border-top: 1px solid var(--border);
}

.wallet-detail {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: var(--s-3);
  gap: var(--s-2);
}

.wallet-detail-header {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  min-width: 0;
}

.wallet-name-label {
  margin: 0;
  min-width: 0;
  flex: 1;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
}

.wallet-name {
  margin: 0;
  flex: 1;
  min-width: 0;
  font-size: var(--font-title);
  font-weight: 600;
}

.wallet-fields {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  margin: 0;
}

.wallet-field {
  display: grid;
  grid-template-columns: 6rem 1fr;
  gap: var(--s-1);
  align-items: center;
}

.wallet-field dt {
  margin: 0;
  font-size: 0.85em;
}

.wallet-field dd {
  margin: 0;
  min-width: 0;
}

.wallet-detail-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-2);
}

.send-dialog {
  width: calc(100% - 2 * var(--s-3));
  max-width: 28rem;
  padding: var(--s-3);
  overflow: visible;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--fill);
  color: var(--text);
}

.send-dialog::backdrop {
  background: color-mix(in srgb, CanvasText 35%, transparent);
}

.send-title {
  margin: 0;
  font-weight: 600;
}

.send-eth {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.send-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-1);
}

.send-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  color: var(--text);
  font-weight: 600;
}

.send-label {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
}

.send-required {
  color: crimson;
}

.send-field input,
.send-field select {
  width: 100%;
  font-weight: 400;
}

.send-balance {
  margin: 0;
  font-weight: 400;
  font-size: 0.85em;
}

.send-error {
  margin: 0;
  min-height: 1.25em;
  color: #c62828;
  font-weight: 400;
  font-size: 0.85em;
}

.send-result {
  margin: 0;
  min-height: 1.25em;
  padding: 0;
  border: none;
  background: none;
  color: var(--text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: inherit;
  line-height: 1.25;
  white-space: pre-wrap;
  word-break: break-all;
  -webkit-user-select: text;
  user-select: text;
}

.send-result.is-empty {
  visibility: hidden;
}

.wallet-detail-address {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.wallet-detail-balance {
  margin: 0;
  color: var(--text);
  font-variant-numeric: tabular-nums;
  word-break: break-all;
  -webkit-user-select: text;
  user-select: text;
}

.wallet-detail-tabs {
  flex-shrink: 0;
}

.wallet-detail-tab-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  overflow: hidden;
}

.token-empty {
  margin: 0;
}

.token-list-wrap {
  flex: 1;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
  background: Field;
  border: 1px solid var(--border);
}

.token-list {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.token-col-symbol {
  width: 5.5rem;
}

.token-col-address {
  width: auto;
}

.token-col-balance {
  width: 7rem;
}

.token-list th,
.token-list td {
  padding: var(--s-1);
  text-align: left;
  vertical-align: middle;
}

.token-list th {
  position: sticky;
  top: 0;
  font-weight: 600;
  background: Field;
}

.token-list th:not(:last-child),
.token-list td:not(:last-child) {
  min-width: 0;
}

.token-list th:last-child,
.token-list td:last-child {
  white-space: nowrap;
  text-align: right;
}

.token-addr-btn {
  width: 100%;
  justify-content: flex-start;
}

.token-balance {
  font-variant-numeric: tabular-nums;
}

.token-empty,
.token-list-wrap,
.token-list,
.token-addr-btn {
  --wails-draggable: no-drag;
}

.dapp-list-wrap {
  max-height: 50%;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
  background: Field;
  border: 1px solid var(--border);
}

.dapp-list {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.dapp-col-url {
  width: 45%;
}

.dapp-col-session {
  width: auto;
}

.dapp-col-action {
  width: calc(var(--control-height) + 2 * var(--s-1));
}

.dapp-list th,
.dapp-list td {
  padding: var(--s-1);
  text-align: left;
  vertical-align: middle;
}

.dapp-list th {
  position: sticky;
  top: 0;
  font-weight: 600;
  background: Field;
}

.dapp-list th:not(:last-child),
.dapp-list td:not(:last-child) {
  min-width: 0;
}

.dapp-list th:last-child,
.dapp-list td:last-child {
  white-space: nowrap;
  text-align: right;
}

.dapp-list .address {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dapp-msg-list {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow: auto;
  background: Field;
  border: 1px solid var(--border);
}

.dapp-msg {
  display: grid;
  grid-template-columns: auto 1fr;
  column-gap: 0.5rem;
  row-gap: 0.15rem;
  align-items: start;
  padding: var(--s-1);
  border-bottom: 1px solid var(--border);
  min-width: 0;
}

.dapp-msg-icon {
  grid-column: 1;
  grid-row: 1;
  width: 14px;
  height: 14px;
  margin-top: 0.15rem;
  color: currentColor;
}

.dapp-msg.is-in .dapp-msg-icon {
  color: var(--msg-in);
}

.dapp-msg.is-out .dapp-msg-icon {
  color: var(--msg-out);
}

.dapp-msg-method,
.dapp-msg-params,
.dapp-msg-ids {
  grid-column: 2;
  min-width: 0;
  overflow-wrap: anywhere;
}

.dapp-msg-method {
  font-weight: 600;
}

.dapp-msg-params {
  font-size: 0.85em;
  opacity: 0.9;
}

.dapp-msg-ids {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  font-size: 0.85em;
}

.dapp-msg-id {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  min-width: 0;
}

.dapp-msg-id-icon {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
  color: currentColor;
}

.dapp-msg-id code {
  font-size: inherit;
}

@media (max-width: 640px) {
  .split {
    flex-direction: column;
  }
  .wallet-list-pane {
    width: 100%;
    max-width: none;
    max-height: 45%;
    border-right: none;
    border-bottom: 1px solid var(--border);
  }
}
</style>