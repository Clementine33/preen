/** One release version contract for desktop, containers and tag publication. */
export function releaseVersion(raw) {
  const version = raw?.replace(/^v/, '') ?? ''
  const number = '(0|[1-9][0-9]*)'
  if (!new RegExp(`^${number}\\.${number}\\.${number}(?:-rc\\.${number})?$`).test(version) || version.length > 128) {
    throw new Error('RAW_TAG must be vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N (no dev or build metadata)')
  }
  return version
}
