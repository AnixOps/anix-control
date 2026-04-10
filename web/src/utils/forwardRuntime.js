export function humanizeForwardRuntimeBackend(t, value) {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'gost') {
    return t('runtime.nodeX.backends.gost')
  }
  if (normalized === 'nftables_ansible') {
    return t('runtime.localRuntime.backends.nftables.label')
  }
  if (normalized === 'iptables_ansible') {
    return t('runtime.localRuntime.backends.iptables.label')
  }
  return value || '-'
}
