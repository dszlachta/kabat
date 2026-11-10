import {ref} from 'vue'
import {Events} from '@wailsio/runtime'

export const pendingTx = ref<Record<string, unknown> | null>(null)

Events.On('pendingTransaction', (event) => {
  let data: unknown = event.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch {
      data = {}
    }
  }
  pendingTx.value = data && typeof data === 'object' && !Array.isArray(data)
    ? data as Record<string, unknown>
    : {}
})
