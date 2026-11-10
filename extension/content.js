(() => {
  const api = globalThis.browser ?? globalThis.chrome
  const disconnected = { code: 4900, message: "provider is disconnected" }
  const instanceId = crypto.randomUUID()

  let port = null
  let reqSeq = 0
  const pending = new Map()

  function reportError(err) {
    console.error(err)
    if (typeof window !== "undefined") {
      window.alert(String(err))
    }
  }

  function failPending() {
    for (const [, w] of pending) {
      w.resolve({ error: disconnected })
    }
    pending.clear()
  }

  function hello(p) {
    try {
      p.postMessage({ type: "kabat/hello", instanceId })
    } catch {
      // Port already closed.
    }
  }

  function connect() {
    const p = api.runtime.connect({ name: "page" })
    port = p
    p.onMessage.addListener((msg) => {
      if (!msg || typeof msg !== "object") return
      if (msg.type === "kabat/event") {
        window.postMessage(msg, "*")
        return
      }
      if (msg.type === "kabat/response") {
        const w = pending.get(msg.id)
        if (!w) return
        pending.delete(msg.id)
        w.resolve({ result: msg.result, error: msg.error })
      }
    })
    p.onDisconnect.addListener(() => {
      void api.runtime.lastError
      if (port === p) port = null
      failPending()
      setTimeout(connect, 50)
    })
    hello(p)
  }
  connect()

  function tellUnload() {
    try {
      port?.postMessage({ type: "kabat/unload", instanceId })
    } catch {
      // Already gone.
    }
    try {
      api.runtime.sendMessage({ type: "kabat/unload", instanceId })
    } catch {
      // Service worker is gone.
    }
  }

  window.addEventListener("pagehide", tellUnload)
  window.addEventListener("beforeunload", tellUnload)

  function send(payload) {
    return new Promise((resolve) => {
      const start = Date.now()
      const trySend = () => {
        if (!port) {
          if (Date.now() - start > 1000) {
            resolve({ error: disconnected })
            return
          }
          setTimeout(trySend, 20)
          return
        }
        const id = ++reqSeq
        pending.set(id, { resolve })
        try {
          port.postMessage({
            type: "kabat/request",
            id,
            instanceId,
            args: payload.args,
            url: payload.url,
          })
        } catch {
          pending.delete(id)
          resolve({ error: disconnected })
        }
      }
      trySend()
    })
  }

  window.addEventListener("message", (event) => {
    if (event.source !== window || event.data?.type !== "kabat/request") return
    const { id, args } = event.data
    void send({ args, url: window.location.href }).then((out) => {
      window.postMessage({
        type: "kabat/response",
        id,
        result: out.result,
        error: out.error,
      }, "*")
    }).catch(reportError)
  })
})()
