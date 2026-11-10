import {ref} from 'vue'
import {Events} from '@wailsio/runtime'
import {WalletService} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'
import type {StoredTx} from '../bindings/github.com/dszlachta/kabat/internal/walletservice'

export const txs = ref<StoredTx[]>([])
export const selectedTxHash = ref('')
export const abiSelectors = ref<Record<string, string>>({})

export async function loadTxs() {
  try {
    txs.value = await WalletService.Transactions() ?? []
  } catch (err) {
    txs.value = []
    console.error(err)
  }
}

export async function loadABISelectors() {
  try {
    const got = await WalletService.ABISelectors() ?? {}
    const out: Record<string, string> = {}
    for (const [selector, name] of Object.entries(got)) {
      if (selector && name) {
        out[selector.toLowerCase()] = name
      }
    }

    console.log('out', out)

    abiSelectors.value = out
  } catch (err) {
    abiSelectors.value = {}
    console.error(err)
  }
}

export function methodNameFor(tx: StoredTx): string {
  if (tx.kind !== 'call' || !tx.selector) {
    return ''
  }
  return abiSelectors.value[tx.selector.toLowerCase()] ?? ''
}

Events.On('transactions', () => {
  void loadTxs()
})

Events.On('abi', () => {
  void loadABISelectors()
})
