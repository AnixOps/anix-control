import request from '@/utils/request'

async function maintenance(url, method = 'get', data) {
  const response = await request({ baseURL: '/api/v3/maintenance', url, method, ...(data === undefined ? {} : { data }) })
  return response?.data ?? response
}

export const getMaintenanceRole = () => maintenance('/me')
export const getMaintenanceSettings = () => maintenance('/settings')
export const saveMaintenanceSettings = settings => maintenance('/settings', 'put', settings)
export const verifyMaintenanceChannel = channel => maintenance('/settings/verify', 'post', { channel })
export const confirmMaintenanceChannel = (channel, code) => maintenance('/settings/confirm', 'post', { channel, code })
export const getMaintenanceTickets = () => maintenance('/tickets')
export const getMaintenanceTicket = id => maintenance(`/tickets/${id}`)
export const claimMaintenanceTicket = id => maintenance(`/tickets/${id}/claim`, 'post', {})
export const addMaintenanceNote = (id, note) => maintenance(`/tickets/${id}/notes`, 'post', { note })
export const closeMaintenanceTicket = (id, note) => maintenance(`/tickets/${id}/close`, 'post', { note })
export const getMaintenanceChanges = () => maintenance('/changes')
export const createMaintenanceChange = change => maintenance('/changes', 'post', change)
export const approveMaintenanceChange = (id, bindingHash) => maintenance(`/changes/${id}/approve`, 'post', { binding_hash: bindingHash })
export const executeMaintenanceChange = id => maintenance(`/changes/${id}/execute`, 'post', {})

export const getMaintenanceCatalog = (afterID = 0) => maintenance(`/catalog?after_id=${afterID}`)
