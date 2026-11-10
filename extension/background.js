import { ulid } from "./lib/ulid.js"

const api = globalThis.browser ?? globalThis.chrome
const HOST = "com.dszlachta.kabat"
const CLOSE_DELAY_MS = 1000
const INSTANCE_GRACE_MS = 400
const RECONNECT_MIN_MS = 250
const RECONNECT_MAX_MS = 4000
const disconnected = { code: 4900, message: "provider is disconnected" }

const pages = new Set()
const waiters = new Map()
const sessionUlids = new Map()
const portInstance = new Map()
const instanceTimers = new Map()
const outbound = []
let port
let portReady = false
let closeTimer = null
let reconnectTimer = null
let reconnectDelay = RECONNECT_MIN_MS

function reportError(err) {
  console.error(err)
  if (typeof window !== "undefined") {
    window.alert(String(err))
  }
  // Service worker: alert is unavailable.
}

function sessionUlidForInstance(instanceId) {
  if (!instanceId) return ulid()
  let id = sessionUlids.get(instanceId)
  if (id) return id
  id = ulid()
  sessionUlids.set(instanceId, id)
  return id
}

function failWaiters() {
  outbound.length = 0
  for (const [, w] of waiters) {
    w.reject(disconnected)
  }
  waiters.clear()
}

function emitToPages(event, params) {
  const msg = { type: "kabat/event", event, params }
  for (const p of pages) {
    try {
      p.postMessage(msg)
    } catch {
      // The tab is gone.
    }
  }
}

function walletGone() {
  sessionUlids.clear()
  for (const timer of instanceTimers.values()) clearTimeout(timer)
  instanceTimers.clear()
  emitToPages("accountsChanged", [])
  emitToPages("disconnect", disconnected)
}

function clearReconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnectDelay = RECONNECT_MIN_MS
}

function scheduleReconnect() {
  if (port || pages.size === 0 || reconnectTimer) return
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    if (port || pages.size === 0) return
    ensurePort()
  }, reconnectDelay)
  reconnectDelay = Math.min(RECONNECT_MAX_MS, reconnectDelay * 2)
}

function markReady() {
  portReady = true
  reconnectDelay = RECONNECT_MIN_MS
  if (!port) return
  const q = outbound.splice(0)
  for (const env of q) {
    port.postMessage(env)
  }
}

function nativeDied() {
  const wasReady = portReady
  port = null
  portReady = false
  if (pages.size === 0) {
    failWaiters()
    clearReconnect()
    return
  }
  if (wasReady) {
    failWaiters()
    walletGone()
  }
  scheduleReconnect()
}

function tellKabatDisconnect(sessionUlid) {
  if (!sessionUlid) return
  const envelope = {
    ulid: ulid(),
    ts: Date.now(),
    payload: { method: "kabat_disconnect", sessionUlid },
  }
  const done = new Promise((resolve, reject) => {
    waiters.set(envelope.ulid, { resolve, reject })
  })
  try {
    dispatch(envelope)
  } catch {
    waiters.delete(envelope.ulid)
    return
  }
  done.catch(() => {})
}

function dropInstance(instanceId) {
  if (!instanceId) return
  const timer = instanceTimers.get(instanceId)
  if (timer) {
    clearTimeout(timer)
    instanceTimers.delete(instanceId)
  }
  const sessionUlid = sessionUlids.get(instanceId)
  sessionUlids.delete(instanceId)
  tellKabatDisconnect(sessionUlid)
}

function instanceHasPort(instanceId) {
  for (const id of portInstance.values()) {
    if (id === instanceId) return true
  }
  return false
}

function scheduleDropInstance(instanceId) {
  if (!instanceId || instanceTimers.has(instanceId)) return
  instanceTimers.set(instanceId, setTimeout(() => {
    instanceTimers.delete(instanceId)
    if (!instanceHasPort(instanceId)) dropInstance(instanceId)
  }, INSTANCE_GRACE_MS))
}

function bindPort(page, instanceId) {
  if (!instanceId) return
  const prev = instanceTimers.get(instanceId)
  if (prev) {
    clearTimeout(prev)
    instanceTimers.delete(instanceId)
  }
  portInstance.set(page, instanceId)
}

function dropPort() {
  clearReconnect()
  if (!port) {
    failWaiters()
    return
  }
  const native = port
  port = null
  portReady = false
  try {
    native.disconnect()
  } catch {
    // Already gone.
  }
  failWaiters()
}

function ensurePort() {
  if (port) return port
  let native
  try {
    native = api.runtime.connectNative(HOST)
  } catch (err) {
    reportError(err)
    nativeDied()
    return null
  }
  native.onDisconnect.addListener(() => {
    void api.runtime.lastError
    // dropPort already nulled `port`; do not treat that as Kabat exiting.
    if (port !== native) return
    nativeDied()
  })
  native.onMessage.addListener((env) => {
    const payload = env && env.payload ? env.payload : {}
    if (payload.event) {
      if (payload.event === "connect") {
        markReady()
        emitToPages(payload.event, payload.params)
        return
      }
      if (payload.event === "disconnect") {
        if (!portReady) return
        portReady = false
        failWaiters()
        sessionUlids.clear()
        emitToPages("accountsChanged", [])
        emitToPages("disconnect", payload.params || disconnected)
        return
      }
      if (portReady) emitToPages(payload.event, payload.params)
      return
    }
    const w = waiters.get(env.ulid)
    if (!w) return
    waiters.delete(env.ulid)
    if (payload.error) w.reject(payload.error)
    else w.resolve(payload.result)
  })
  port = native
  portReady = false
  if (api.runtime.lastError) {
    port = null
    nativeDied()
    return null
  }
  return port
}

function dispatch(envelope) {
  const native = ensurePort()
  if (!native || !portReady) {
    outbound.push(envelope)
    if (!native) scheduleReconnect()
    return
  }
  native.postMessage(envelope)
}

function handleDappRequest(page, msg) {
  const instanceId = msg.instanceId || portInstance.get(page) || ""
  if (instanceId) bindPort(page, instanceId)
  const url = page?.sender?.tab?.url || msg.url || ""
  const pageId = msg.id
  let id
  let sessionUlid
  try {
    id = ulid()
    sessionUlid = sessionUlidForInstance(instanceId)
  } catch (err) {
    reportError(err)
    replyPage(page, pageId, { error: disconnected })
    return
  }
  const envelope = {
    ulid: id,
    ts: Date.now(),
    payload: {
      method: msg.args?.method,
      params: msg.args?.params,
      origin: url,
      sessionUlid,
    },
  }
  const done = new Promise((resolve, reject) => {
    waiters.set(id, { resolve, reject })
  })
  try {
    dispatch(envelope)
  } catch (err) {
    waiters.delete(id)
    replyPage(page, pageId, { error: err && err.code ? err : disconnected })
    return
  }
  done.then(
    (result) => replyPage(page, pageId, { result }),
    (error) => replyPage(page, pageId, { error }),
  )
}

function replyPage(page, id, out) {
  if (!page) return
  try {
    page.postMessage({ type: "kabat/response", id, result: out.result, error: out.error })
  } catch {
    // The tab is gone.
  }
}

api.runtime.onConnect.addListener((p) => {
  if (p.name !== "page") return
  pages.add(p)
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
  ensurePort()
  p.onMessage.addListener((msg) => {
    if (msg?.type === "kabat/hello") {
      bindPort(p, msg.instanceId)
      return
    }
    if (msg?.type === "kabat/unload") {
      dropInstance(msg.instanceId || portInstance.get(p))
      return
    }
    if (msg?.type !== "kabat/request") return
    handleDappRequest(p, msg)
  })
  p.onDisconnect.addListener(() => {
    const instanceId = portInstance.get(p)
    portInstance.delete(p)
    pages.delete(p)
    scheduleDropInstance(instanceId)
    if (pages.size !== 0) return
    closeTimer = setTimeout(() => {
      closeTimer = null
      if (pages.size === 0) dropPort()
    }, CLOSE_DELAY_MS)
  })
})

api.runtime.onMessage.addListener((msg) => {
  if (msg?.type === "kabat/unload") {
    dropInstance(msg.instanceId)
  }
})
