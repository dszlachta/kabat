(() => {
  const api = globalThis.browser ?? globalThis.chrome

  function reportError(err) {
    console.error(err)
    if (typeof window !== "undefined") {
      window.alert(String(err))
    }
  }

  function connect() {
    const port = api.runtime.connect({ name: "page" })
    port.onMessage.addListener((msg) => {
      if (msg && msg.type === "kabat/event") {
        window.postMessage(msg, "*")
      }
    })
    port.onDisconnect.addListener(() => {
      setTimeout(connect, 50)
    })
  }
  connect()

  window.addEventListener("message", (event) => {
    if (event.source !== window || event.data?.type !== "kabat/request") return
    const { id, args } = event.data
    api.runtime.sendMessage({ args }, (res) => {
      const last = api.runtime.lastError
      if (last) {
        window.postMessage({
          type: "kabat/response",
          id,
          error: { code: 4900, message: last.message || "provider is disconnected" },
        }, "*")
        return
      }
      window.postMessage({ type: "kabat/response", id, result: res?.result, error: res?.error }, "*")
    })
  })
})()
