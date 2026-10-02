import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { editionAllows, extensionMenuAllowed } from '@/composables/useEdition'
import { adminExtensionMenus } from '@/extensions/runtime'
import { normalizeWebUIMenuParent } from '@/extensions/menuRegistry'
import { activeMenuItem, buildAdminMenu, buildUserMenu } from './menu'

// routeExists answers for `optional` menu items. Without a router that can
// list its routes (unit tests mock it), optional items stay hidden.
function createRouteExists(router) {
  return path => (typeof router?.getRoutes === 'function'
    ? router.getRoutes().some(record => record.path === path)
    : false)
}

// useAdminMenu wires navigation/menu.js to the live edition, permissions
// and plugin menus. The sidebar, the top bar and the palette share it.
export function useAdminMenu() {
  const { t } = useAppI18n()
  const userStore = useUserStore()
  const route = useRoute()
  const router = useRouter()
  const routeExists = createRouteExists(router)

  const groups = computed(() => buildAdminMenu({
    t,
    editionAllows,
    hasPermission: permission => userStore.hasPermission(permission),
    routeExists,
    extensionMenus: adminExtensionMenus.value,
    extensionMenuAllowed,
    normalizeParent: normalizeWebUIMenuParent
  }))

  const active = computed(() => activeMenuItem(groups.value, route.path))

  return { groups, active }
}

export function useUserMenu() {
  const { t } = useAppI18n()
  const route = useRoute()
  const items = computed(() => buildUserMenu({ t, editionAllows }))
  const activeId = computed(() => {
    const path = route.path
    const match = items.value.find(item => path === item.to || path.startsWith(`${item.to}/`))
    return match?.id || ''
  })
  return { items, activeId }
}
