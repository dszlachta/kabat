import { ulid } from "./lib/ulid.js"

const api = globalThis.browser ?? globalThis.chrome
const HOST = "com.dszlachta.kabat"

const pages = new Set()
const waiters = new Map()
let port

function reportError(err) {
  console.error(err)
  if (typeof window !== "undefined") {
    window.alert(String(err))
  }
  // Service worker: alert is unavailable.
}

function failWaiters() {
  for (const [, w] of waiters) {
    w.reject({ code: 4900, message: "provider is disconnected" })
  }
  waiters.clear()
}

function ensurePort() {
  if (port) return port
  try {
    port = api.runtime.connectNative(HOST)
  } catch (err) {
    reportError(err)
    return null
  }
  const native = port
  native.onMessage.addListener((env) => {
    const payload = env && env.payload ? env.payload : {}
    if (payload.event) {
      for (const p of pages) {
        try {
          p.postMessage({ type: "kabat/event", event: payload.event, params: payload.params })
        } catch {
          // The tab is gone.
        }
      }
      return
    }
    const w = waiters.get(env.ulid)
    if (!w) return
    waiters.delete(env.ulid)
    if (payload.error) w.reject(payload.error)
    else w.resolve(payload.result)
  })
  native.onDisconnect.addListener(() => {
    if (port === native) {
      port = null
      failWaiters()
    }
  })
  return port
}

api.runtime.onConnect.addListener((p) => {
  if (p.name !== "page") return
  pages.add(p)
  ensurePort()
  p.onDisconnect.addListener(() => {
    pages.delete(p)
    if (pages.size === 0 && port) {
      const native = port
      port = null
      native.disconnect()
    }
  })
})

api.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  const native = ensurePort()
  if (!native) {
    sendResponse({ error: { code: 4900, message: "provider is disconnected" } })
    return
  }
  let id
  try {
    id = ulid()
  } catch (err) {
    reportError(err)
    sendResponse({ error: { code: 4900, message: "provider is disconnected" } })
    return
  }
  const done = new Promise((resolve, reject) => waiters.set(id, { resolve, reject }))
  try {
    native.postMessage({
      ulid: id,
      ts: Date.now(),
      payload: { method: msg.args?.method, params: msg.args?.params },
    })
  } catch (err) {
    waiters.delete(id)
    reportError(err)
    sendResponse({ error: { code: 4900, message: "provider is disconnected" } })
    return
  }
  done.then(
    (result) => sendResponse({ result }),
    (error) => sendResponse({ error }),
  )
  return true
})
