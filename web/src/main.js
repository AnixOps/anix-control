import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { mountLegacyI18n } from './utils/legacyI18n'

async function bootstrap() {
  await initI18n()

  const app = createApp(App)

  app.use(createPinia())
  app.use(router)
  app.use(i18n)

  app.mount('#app')
  mountLegacyI18n(i18n, document.querySelector('#app'))
}

bootstrap()
