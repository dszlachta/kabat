(() => {
  const listeners = new Map()
  const pending = new Map()
  let nextId = 1

  function reportError(err) {
    console.error(err)
    if (typeof window !== "undefined") {
      window.alert(String(err))
    }
  }

  const provider = {
    selectedAddress: null,
    request(args) {
      const id = nextId++
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject, method: args?.method })
        window.postMessage({ type: "kabat/request", id, args }, "*")
      })
    },
    on(event, handler) {
      if (typeof handler !== "function") return
      if (!listeners.has(event)) listeners.set(event, new Set())
      listeners.get(event).add(handler)
    },
    removeListener(event, handler) {
      listeners.get(event)?.delete(handler)
    },
  }

  function setSelectedAddress(accounts) {
    if (Array.isArray(accounts) && accounts.length > 0) {
      provider.selectedAddress = accounts[0]
    } else {
      provider.selectedAddress = null
    }
  }

  window.addEventListener("message", (event) => {
    if (event.source !== window || !event.data || typeof event.data !== "object") return
    const msg = event.data
    if (msg.type === "kabat/response") {
      const waiter = pending.get(msg.id)
      if (!waiter) return
      pending.delete(msg.id)
      if (msg.error) {
        const err = new Error(msg.error.message || "provider error")
        err.code = msg.error.code
        err.data = msg.error.data
        waiter.reject(err)
      } else {
        if (waiter.method === "eth_accounts" || waiter.method === "eth_requestAccounts") {
          setSelectedAddress(msg.result)
        }
        waiter.resolve(msg.result)
      }
      return
    }
    if (msg.type === "kabat/event") {
      if (msg.event === "accountsChanged") {
        setSelectedAddress(msg.params)
      }
      const set = listeners.get(msg.event)
      if (!set) return
      for (const fn of set) fn(msg.params)
    }
  })

  window.ethereum = provider
  window.dispatchEvent(new Event("ethereum#initialized"))

  const info = Object.freeze({
    uuid: "7b3c1e90-4d2a-4f11-9c6b-0a1b2c3d4e5f",
    name: "Kabat",
    icon: "data:image/svg+xml,<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 32 32\"><rect width=\"32\" height=\"32\" rx=\"6\" fill=\"%23ff4d4d\"/></svg>",
    rdns: "com.dszlachta.kabat",
  })
  const announce = () => {
    window.dispatchEvent(new CustomEvent("eip6963:announceProvider", {
      detail: Object.freeze({ info, provider }),
    }))
  }
  window.addEventListener("eip6963:requestProvider", announce)
  announce()
})()
