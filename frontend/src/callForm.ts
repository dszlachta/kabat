import {ref} from 'vue'
import {Events} from '@wailsio/runtime'

export type ViewTab = 'details' | 'functions' | 'call' | 'abi' | 'eip712'

export type CallFormState = {
  selector: string
  args: Record<string, string[]>
  from: string
  eth: string
}

export const selectedHash = ref('')
export const viewTab = ref<ViewTab>('details')

const forms = ref<Record<string, CallFormState>>({})
let defaultFrom = ''
let lastSelected = ''

function emptyForm(): CallFormState {
  return {selector: '', args: {}, from: defaultFrom, eth: ''}
}

export function callFormFor(address: string): CallFormState {
  const key = address.toLowerCase()
  if (!forms.value[key]) {
    forms.value = {...forms.value, [key]: emptyForm()}
  }
  return forms.value[key]
}

export function applyRepeat(input: {
  address: string
  selector: string
  args: string[]
  from: string
  eth: string
}) {
  const form = callFormFor(input.address)
  form.selector = input.selector
  form.from = input.from
  form.eth = input.eth
  if (input.selector) {
    form.args = {...form.args, [input.selector]: input.args.slice()}
  }
  viewTab.value = 'call'
}

export function applySelectedFrom(address: string) {
  if (!address) {
    return
  }
  if (lastSelected.toLowerCase() === address.toLowerCase()) {
    return
  }
  lastSelected = address
  defaultFrom = address
  for (const form of Object.values(forms.value)) {
    form.from = address
  }
}

export function clearCallInputs(address: string) {
  const form = callFormFor(address)
  const sel = form.selector
  if (sel) {
    const n = form.args[sel]?.length ?? 0
    form.args = {...form.args, [sel]: Array.from({length: n}, () => '')}
  }
  form.eth = ''
}

Events.On('accounts', (event) => {
  if (!Array.isArray(event.data)) {
    return
  }
  const selected = event.data.find((item: {selected?: boolean, address?: string}) => item?.selected)
  if (selected && typeof selected.address === 'string') {
    applySelectedFrom(selected.address)
  }
})
