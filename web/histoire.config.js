// Histoire: the component library's documentation and visual review
// surface (decision D6). npm run story:dev / story:build.
// Stories live next to the components: src/ui/stories/*.story.vue.
// The preview loads the vendored tokens, base.css and both locales
// (src/ui/histoire.setup.js); the toolbar's dark-mode switch maps to
// <html data-theme="dark">, and ?lang=en on the preview URL switches locale.
import { defineConfig } from 'histoire'
import { HstVue } from '@histoire/plugin-vue'

export default defineConfig({
  plugins: [HstVue()],
  setupFile: './src/ui/histoire.setup.js',
  storyMatch: ['src/ui/**/*.story.vue'],
  outDir: '.histoire/dist',
  // The app build writes favicons and the manifest; stories do not need them.
  viteIgnorePlugins: ['anixops-brand-icons'],
  theme: {
    title: 'AnixOps Control UI',
    defaultColorScheme: 'light',
    storeColorScheme: true,
    darkClass: 'dark'
  },
  // The preview paints its own token background (stories/story.css).
  backgroundPresets: [{ label: 'Page (--bg)', color: 'transparent' }],
  autoApplyContrastColor: false,
  responsivePresets: [
    { label: 'Phone 390', width: 390, height: 844 },
    { label: 'Tablet 834', width: 834, height: 1112 },
    { label: 'Desktop 1440', width: 1440, height: 900 }
  ],
  tree: {
    groups: [
      { id: 'top', title: '' },
      { id: 'actions', title: 'Actions' },
      { id: 'forms', title: 'Forms' },
      { id: 'overlays', title: 'Overlays and feedback' },
      { id: 'display', title: 'Display' },
      { id: 'layout', title: 'Layout' }
    ]
  }
})
