export function humanizeForwardRuntimeBackend(t, value) {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'gost') {
    return t('runtime.nodeX.backends.gost')
  }
  if (normalized === 'nftables_ansible' || normalized === 'iptables_ansible') {
    // iptables 已下线, 归一化为 nftables 显示
    return t('runtime.localRuntime.backends.nftables.label')
  }
  return value || '-'
}
