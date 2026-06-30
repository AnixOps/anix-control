# Arco Design Vue Setup

Official docs: `https://arco.design/vue/docs/start`

This project's frontend is **Vue 3**, so it uses **Arco Design Vue** (`@arco-design/web-vue`).
Semi Design was dropped because it is a React-only library and incompatible with Vue.

## Why no MCP

Arco Design Vue has **no official MCP server** (only a discussion in GitHub issue #3468).
Do not install unverified third-party npx packages to impersonate one.
Use WebFetch against the official docs instead:

- Start guide: `https://arco.design/vue/docs/start`
- Dark mode: `https://arco.design/vue/docs/dark`
- Design Token: `https://arco.design/vue/docs/token`
- Component docs: `https://arco.design/vue/component/{name}`

## Install

```bash
npm install @arco-design/web-vue
```

## Bootstrap (main.js)

```js
import ArcoVue from '@arco-design/web-vue'
import ArcoVueIcon from '@arco-design/web-vue/es/icon'
import '@arco-design/web-vue/dist/arco.css'  // before custom styles

app.use(ArcoVue)
app.use(ArcoVueIcon)
```

## Dark mode

Toggle `arco-theme="dark"` on `body`; Arco's CSS variables auto-invert.
See `web/src/composables/useTheme.js` for this project's implementation.
