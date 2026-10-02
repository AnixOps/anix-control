// Pages moved to the list page template in UI U6 (plan §7.1): their tables
// are UiDataTable, never a bare <table>. ESLint (vue/no-restricted-html-elements)
// and src/__tests__/dataTableGuard.test.js read this list; add a page when it
// migrates (U7 and U8 add theirs).
export const DATA_TABLE_PAGES = [
  'src/views/admin/Users.vue',
  'src/views/admin/Tickets.vue',
  'src/views/admin/Plugins.vue',
  'src/views/admin/Agent.vue',
  'src/views/admin/Orders.vue',
  'src/views/admin/Coupons.vue',
  'src/views/admin/Plans.vue',
  'src/views/admin/Payment.vue',
  'src/views/admin/Knowledge.vue',
  'src/views/admin/InviteCodes.vue',
  'src/views/admin/AccessGroups.vue',
  'src/views/admin/Invite.vue',
  'src/views/admin/AnsibleMachines.vue'
]
