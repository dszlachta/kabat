const WEI_DECIMALS = 18n
const BASE = 10n ** WEI_DECIMALS

function parseWei(wei: string): bigint {
  if (!wei) {
    return 0n
  }
  try {
    const s = wei.trim()
    if (s.startsWith('-0x') || s.startsWith('-0X')) {
      return -BigInt(s.slice(1))
    }
    return BigInt(s)
  } catch {
    return 0n
  }
}

function toFrac(wei: string): {whole: bigint, frac: bigint} {
  const value = parseWei(wei)
  if (value === 0n) {
    return {whole: 0n, frac: 0n}
  }
  
  const whole = value / BASE
  const frac = value % BASE
  return {whole, frac}
}

/** Wei hex from an ETH decimal string. Empty is 0x0. */
export function parseEtherExact(eth: string): string {
  const s = eth.trim()
  if (!s) {
    return '0x0'
  }
  if (s.startsWith('-') || s === '.' || (s.match(/\./g) ?? []).length > 1) {
    throw new Error('invalid ETH amount')
  }
  const [wholeRaw, fracRaw = ''] = s.split('.')
  if (wholeRaw !== '' && !/^[0-9]+$/.test(wholeRaw)) {
    throw new Error('invalid ETH amount')
  }
  if (fracRaw !== '' && !/^[0-9]+$/.test(fracRaw)) {
    throw new Error('invalid ETH amount')
  }
  if (fracRaw.length > Number(WEI_DECIMALS)) {
    throw new Error('invalid ETH amount')
  }
  const whole = wholeRaw === '' ? 0n : BigInt(wholeRaw)
  const frac = fracRaw === '' ? 0n : BigInt(fracRaw.padEnd(Number(WEI_DECIMALS), '0'))
  return '0x' + (whole * BASE + frac).toString(16)
}

/** Exact ETH decimal from chain wei (hex or decimal), no rounding. */
export function formatEtherExact(wei: string): string {
  const {whole, frac} = toFrac(wei)
  if (frac === 0n) {
    return whole.toString()
  }
  return whole.toString() + '.' + frac.toString().padStart(18, '0').replace(/0+$/, '')
}

/** ETH for the wallet list: at most 4 digits after the decimal, truncated. */
export function formatEtherList(wei: string): string {
  const {whole, frac} = toFrac(wei)

  // Keep 4 decimal places.
  const scale = 10n ** (WEI_DECIMALS - 4n)

  // Round based on the 5th decimal place.
  const roundedFrac = (frac + scale / 2n) / scale

  // Rounding can turn .99995... into 1.0000
  if (roundedFrac === 10000n) {
    return `${whole + 1n}`
  }

  if (roundedFrac === 0n) {
    return whole.toString()
  }

  return `${whole}.${roundedFrac.toString().padStart(4, '0').replace(/0+$/, '')}`
}

/** Signed ETH for tx rows: +N or -N using the list formatter. */
export function formatEtherSigned(wei: string): string {
  const value = parseWei(wei)
  if (value === 0n) {
    return '+0'
  }
  const sign = value < 0n ? '-' : '+'
  const abs = value < 0n ? -value : value
  return sign + formatEtherList(abs.toString())
}
