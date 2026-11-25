import request from '@/utils/request'

// The intranet mesh's panel side (/api/v5/mesh, docs/design/mesh-regions):
// regions and border nodes, members and their addresses, host groups,
// device names, the intranet names and the region changes. Refusals:
// invalid_request (400), not_found (404), conflict and address_exhausted
// (409), each with a message.

function v5(config) {
  return request({ ...config, baseURL: '/api/v5' })
}

function unwrap(response) {
  return response?.data ?? response
}

const id = value => encodeURIComponent(value)

// { settings, regions: [{ id, code, name, subnet, server_range,
// device_pool, network_name, security, border_nodes, members, pool_used,
// pool_size, warnings }], host_groups, pending_changes }. The first call
// creates the default regions.
export async function getMesh() {
  return unwrap(await v5({ url: '/mesh', method: 'get' }))
}

export async function updateMeshSettings(settings) {
  return unwrap(await v5({ url: '/mesh/settings', method: 'put', data: settings }))
}

export async function createMeshRegion(region) {
  return unwrap(await v5({ url: '/mesh/regions', method: 'post', data: region }))
}

export async function updateMeshRegion(regionId, region) {
  return unwrap(await v5({ url: `/mesh/regions/${id(regionId)}`, method: 'put', data: region }))
}

export async function deleteMeshRegion(regionId) {
  return unwrap(await v5({ url: `/mesh/regions/${id(regionId)}`, method: 'delete' }))
}

// [{ id, region_id, region_code, node_ref, hostname, fqdn, role,
// address_mode, address, address_used_at, probe_address, agentless,
// warnings }]
export async function listMeshMembers(regionId) {
  const params = regionId ? { region_id: regionId } : undefined
  return unwrap(await v5({ url: '/mesh/members', method: 'get', params }))
}

export async function createMeshMember(member) {
  return unwrap(await v5({ url: '/mesh/members', method: 'post', data: member }))
}

export async function updateMeshMember(memberId, member) {
  return unwrap(await v5({ url: `/mesh/members/${id(memberId)}`, method: 'put', data: member }))
}

export async function deleteMeshMember(memberId) {
  return unwrap(await v5({ url: `/mesh/members/${id(memberId)}`, method: 'delete' }))
}

// { region_id, address, note }: the member gets a new address.
export async function moveMeshMember(memberId, move) {
  return unwrap(await v5({ url: `/mesh/members/${id(memberId)}/move`, method: 'post', data: move }))
}

export async function listMeshHostGroups() {
  return unwrap(await v5({ url: '/mesh/host-groups', method: 'get' }))
}

export async function createMeshHostGroup(group) {
  return unwrap(await v5({ url: '/mesh/host-groups', method: 'post', data: group }))
}

export async function updateMeshHostGroup(groupId, group) {
  return unwrap(await v5({ url: `/mesh/host-groups/${id(groupId)}`, method: 'put', data: group }))
}

export async function deleteMeshHostGroup(groupId) {
  return unwrap(await v5({ url: `/mesh/host-groups/${id(groupId)}`, method: 'delete' }))
}

export async function listMeshDeviceNames() {
  return unwrap(await v5({ url: '/mesh/device-names', method: 'get' }))
}

export async function createMeshDeviceName(name) {
  return unwrap(await v5({ url: '/mesh/device-names', method: 'post', data: name }))
}

export async function updateMeshDeviceName(nameId, name) {
  return unwrap(await v5({ url: `/mesh/device-names/${id(nameId)}`, method: 'put', data: name }))
}

export async function deleteMeshDeviceName(nameId) {
  return unwrap(await v5({ url: `/mesh/device-names/${id(nameId)}`, method: 'delete' }))
}

// { suffix, entries: [{ fqdn, name, region_code, address, source,
// member_id, duplicate }] }
export async function getMeshNames() {
  return unwrap(await v5({ url: '/mesh/names', method: 'get' }))
}

export async function listMeshRegionChanges(status) {
  const params = status ? { status } : undefined
  return unwrap(await v5({ url: '/mesh/region-changes', method: 'get', params }))
}

export async function approveMeshRegionChange(changeId, decision = {}) {
  return unwrap(await v5({ url: `/mesh/region-changes/${id(changeId)}/approve`, method: 'post', data: decision }))
}

export async function rejectMeshRegionChange(changeId, decision = {}) {
  return unwrap(await v5({ url: `/mesh/region-changes/${id(changeId)}/reject`, method: 'post', data: decision }))
}

// The message of a refusal, or fallback.
export function meshErrorMessage(error, fallback) {
  return error?.response?.data?.error?.message || error?.message || fallback
}
