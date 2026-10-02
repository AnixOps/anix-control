<template>
  <div class="help">
    <!-- One article: reading width, table of contents, previous / next. -->
    <template v-if="articleId">
      <div v-if="state === 'loading'" class="help-article">
        <UiSkeleton v-if="showSkeleton" variant="text" :lines="8" />
      </div>
      <LoadError v-else-if="state === 'failed'" :title="t('portal.help.loadFailed')" :error="loadError" @retry="load" />
      <div v-else-if="!article" class="help-article">
        <RouterLink :to="listLocation" class="help-article__back"><UiIcon :icon="ChevronLeft" :size="16" />{{ t('portal.help.back') }}</RouterLink>
        <UiEmptyState :icon="FileQuestion" :title="t('portal.help.notFound')" :description="t('portal.help.notFoundHint')" heading-tag="h1" data-help-not-found />
      </div>
      <article v-else class="help-article" :aria-labelledby="`help-article-${article.id}`" data-help-article>
        <RouterLink :to="listLocation" class="help-article__back" data-help-back><UiIcon :icon="ChevronLeft" :size="16" />{{ t('portal.help.back') }}</RouterLink>
        <header class="help-article__header">
          <h1 :id="`help-article-${article.id}`" ref="articleHeadingRef" class="help-article__title" tabindex="-1">{{ article.title }}</h1>
          <p class="help-article__meta">
            <span v-if="article.category">{{ article.category }} · </span>
            <span :title="format.dateTime(article.updated_at)">{{ t('portal.help.updated', { date: format.date(article.updated_at) }) }}</span>
          </p>
        </header>

        <nav v-if="toc.length >= 3" class="help-toc" :aria-label="t('portal.help.toc')" data-help-toc>
          <p class="help-toc__title">{{ t('portal.help.toc') }}</p>
          <ol class="help-toc__list">
            <li v-for="heading in toc" :key="heading.id" :class="`help-toc__item--${heading.level}`">
              <a :href="`#${heading.id}`" class="help-toc__link" @click.prevent="jumpTo(heading.id)">{{ heading.text }}</a>
            </li>
          </ol>
        </nav>

        <ArticleBody :blocks="blocks" />

        <nav class="help-pager" :aria-label="t('portal.help.prev') + ' / ' + t('portal.help.next')" data-help-pager>
          <RouterLink v-if="neighbours.prev" :to="articleLocation(neighbours.prev)" class="help-pager__link" rel="prev" data-help-prev>
            <span class="help-pager__label"><UiIcon :icon="ChevronLeft" :size="16" />{{ t('portal.help.prev') }}</span>
            <span class="help-pager__title">{{ neighbours.prev.title }}</span>
          </RouterLink>
          <span v-else />
          <RouterLink v-if="neighbours.next" :to="articleLocation(neighbours.next)" class="help-pager__link help-pager__link--next" rel="next" data-help-next>
            <span class="help-pager__label">{{ t('portal.help.next') }}<UiIcon :icon="ChevronRight" :size="16" /></span>
            <span class="help-pager__title">{{ neighbours.next.title }}</span>
          </RouterLink>
        </nav>

        <p class="help-contact">
          {{ t('portal.help.contact') }}
          <RouterLink :to="{ path: '/user/tickets', query: { new: '1' } }">{{ t('portal.help.contactAction') }}</RouterLink>
        </p>
      </article>
    </template>

    <!-- The list: search, categories, articles. -->
    <template v-else>
      <header class="help-hero">
        <h1 class="help-hero__title">{{ t('portal.help.title') }}</h1>
        <p class="help-hero__description">{{ t('portal.help.description') }}</p>
        <form class="help-search" role="search" @submit.prevent>
          <label for="help-search" class="visually-hidden">{{ t('portal.help.search') }}</label>
          <UiIcon :icon="Search" :size="20" class="help-search__icon" />
          <input
            id="help-search"
            ref="searchRef"
            v-model="query"
            class="help-search__input"
            type="search"
            :placeholder="t('portal.help.search')"
            autocomplete="off"
            enterkeyhint="search"
            data-help-search
            @keydown.esc="clearSearch"
          >
          <kbd class="help-search__kbd" aria-hidden="true">/</kbd>
        </form>
      </header>

      <div v-if="state === 'loading'">
        <UiSkeleton v-if="showSkeleton" variant="table-row" :rows="4" :columns="1" />
      </div>
      <LoadError v-else-if="state === 'failed'" :title="t('portal.help.loadFailed')" :error="loadError" @retry="load" />
      <UiEmptyState v-else-if="!articles.length" :icon="BookOpen" :title="t('portal.help.empty')" heading-tag="h2" data-help-empty />

      <template v-else>
        <section v-if="!trimmedQuery && categories.length > 1" class="help-categories" aria-labelledby="help-categories-title">
          <h2 id="help-categories-title" class="visually-hidden">{{ t('portal.help.categories') }}</h2>
          <ul class="help-categories__grid">
            <li v-for="category in categoryCards" :key="category.key">
              <button
                type="button"
                class="help-category"
                :class="{ 'is-selected': selectedCategory === category.key }"
                :aria-pressed="selectedCategory === category.key ? 'true' : 'false'"
                :data-help-category="category.key || 'all'"
                @click="selectCategory(category.key)"
              >
                <UiIcon :icon="category.key ? Folder : LayoutGrid" :size="20" class="help-category__icon" />
                <span class="help-category__name">{{ category.label }}</span>
                <span class="help-category__count">{{ t('portal.help.articles', { n: category.count }, category.count) }}</span>
              </button>
            </li>
          </ul>
        </section>

        <section class="help-results" aria-labelledby="help-results-title" data-help-results>
          <h2 id="help-results-title" class="help-results__title" aria-live="polite">
            {{ trimmedQuery ? t('portal.help.results', { query: trimmedQuery }) : (selectedCategory ? categoryLabel(selectedCategory) : t('portal.help.all')) }}
          </h2>
          <UiEmptyState
            v-if="!filtered.length"
            :icon="SearchX"
            :title="t('portal.help.noResults', { query: trimmedQuery })"
            compact
            data-help-no-results
          >
            <template #actions>
              <UiButton @click="clearSearch">{{ t('portal.help.clearSearch') }}</UiButton>
            </template>
          </UiEmptyState>
          <ul v-else class="help-list">
            <li v-for="item in filtered" :key="item.id" class="help-list__item">
              <RouterLink :to="articleLocation(item)" class="help-list__link" :data-help-article-link="item.id">
                <span class="help-list__title">{{ item.title }}</span>
                <span class="help-list__excerpt">{{ excerpt(item.body) }}</span>
                <span class="help-list__meta">
                  <span v-if="item.category">{{ item.category }} · </span>{{ format.date(item.updated_at) }}
                </span>
              </RouterLink>
              <UiIcon :icon="ChevronRight" :size="20" class="help-list__chevron" />
            </li>
          </ul>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup>
// 帮助中心 (plan §8.1): a centred search (Apple support style; "/" focuses
// it), category cards, and the article list; an article opens in place at
// the reading width (692 px) with a table of contents, previous / next and a
// way to open a ticket. State lives in the query (?q=, ?category=,
// ?article=) so a search or an article can be shared and Back works; no new
// route. Data: GET /user/knowledge (the list carries the bodies), in the
// order the administrator sorted the articles.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { BookOpen, ChevronLeft, ChevronRight, FileQuestion, Folder, LayoutGrid, Search, SearchX } from '@lucide/vue'
import { getKnowledgeList } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { listOf, unwrapPanel } from '@/utils/panelResponse'
import { articleExcerpt, parseArticle } from '@/utils/articleMarkup'
import LoadError from '@/components/common/LoadError.vue'
import ArticleBody from './ArticleBody.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'

const { t } = useAppI18n()
const format = useFormat()
const route = useRoute()
const router = useRouter()

const articles = ref([])
const state = ref('loading')
const loadError = ref(null)
const showSkeleton = useDelayedLoading(computed(() => state.value === 'loading'))
const searchRef = ref(null)
const articleHeadingRef = ref(null)

async function load() {
  state.value = 'loading'
  loadError.value = null
  try {
    articles.value = listOf(unwrapPanel(await getKnowledgeList()))
    state.value = 'ready'
  } catch (error) {
    loadError.value = error
    state.value = 'failed'
  }
}

// --- query state -------------------------------------------------------------
const articleId = computed(() => String(route.query.article || ''))
const selectedCategory = computed(() => String(route.query.category || ''))
const query = ref(String(route.query.q || ''))
const trimmedQuery = computed(() => query.value.trim())

let queryTimer = null
watch(query, (value) => {
  if (queryTimer) clearTimeout(queryTimer)
  queryTimer = setTimeout(() => {
    const q = value.trim()
    if (q === String(route.query.q || '')) return
    router.replace({ query: { ...route.query, q: q || undefined } })
  }, 250)
})
watch(() => route.query.q, (value) => {
  if (String(value || '') !== trimmedQuery.value) query.value = String(value || '')
})

function selectCategory(key) {
  router.replace({ query: { ...route.query, category: key || undefined } })
}

function clearSearch() {
  query.value = ''
  router.replace({ query: { ...route.query, q: undefined } })
  searchRef.value?.focus()
}

const listLocation = computed(() => ({ path: '/user/knowledge', query: { category: selectedCategory.value || undefined, q: trimmedQuery.value || undefined } }))

function articleLocation(item) {
  return { path: '/user/knowledge', query: { ...listLocation.value.query, article: String(item.id) } }
}

// --- list --------------------------------------------------------------------
function categoryLabel(key) {
  return key === '__other' ? t('portal.help.uncategorized') : key
}

function categoryKey(item) {
  return String(item.category || '').trim() || '__other'
}

const categories = computed(() => {
  const counts = new Map()
  for (const item of articles.value) counts.set(categoryKey(item), (counts.get(categoryKey(item)) || 0) + 1)
  return [...counts.entries()].map(([key, count]) => ({ key, label: categoryLabel(key), count }))
})

const categoryCards = computed(() => [{ key: '', label: t('portal.help.all'), count: articles.value.length }, ...categories.value])

const filtered = computed(() => {
  const terms = trimmedQuery.value.toLowerCase().split(/\s+/).filter(Boolean)
  return articles.value.filter((item) => {
    if (!terms.length && selectedCategory.value && categoryKey(item) !== selectedCategory.value) return false
    if (!terms.length) return true
    const haystack = `${item.title}\n${item.category}\n${item.body}`.toLowerCase()
    return terms.every(term => haystack.includes(term))
  })
})

function excerpt(body) {
  return articleExcerpt(body, 110)
}

// --- article -------------------------------------------------------------------
const article = computed(() => articles.value.find(item => String(item.id) === articleId.value) || null)
const blocks = computed(() => (article.value ? parseArticle(article.value.body) : []))
const toc = computed(() => blocks.value.filter(block => block.type === 'heading' && block.level <= 3))

// Previous / next follow the list the reader came from (search or category).
const neighbours = computed(() => {
  const list = filtered.value.some(item => String(item.id) === articleId.value) ? filtered.value : articles.value
  const index = list.findIndex(item => String(item.id) === articleId.value)
  return { prev: index > 0 ? list[index - 1] : null, next: index >= 0 && index < list.length - 1 ? list[index + 1] : null }
})

function jumpTo(id) {
  const target = document.getElementById(id)
  if (!target) return
  target.scrollIntoView({ behavior: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' })
  target.focus({ preventScroll: true })
}

// Opening another article: start at its title.
watch(articleId, (id, previous) => {
  if (id && previous !== undefined) nextTick(() => articleHeadingRef.value?.focus({ preventScroll: true }))
})

// "/" focuses the search (plan §9), unless typing somewhere already.
function onKeydown(event) {
  if (event.key !== '/' || articleId.value || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target
  if (target?.closest?.('input, textarea, select, [contenteditable="true"], [role="dialog"]')) return
  event.preventDefault()
  searchRef.value?.focus()
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  if (queryTimer) clearTimeout(queryTimer)
})
</script>

<style scoped>
.help {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

/* ---- hero and search ---------------------------------------------------------- */
.help-hero {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  align-items: center;
  text-align: center;
}

.help-hero__title {
  font-size: var(--type-display-size);
  font-weight: var(--type-display-weight);
  line-height: var(--type-display-line);
  letter-spacing: var(--type-display-tracking);
}

.help-hero__description {
  color: var(--label-2);
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-regular);
}

.help-search {
  position: relative;
  display: flex;
  align-items: center;
  width: min(560px, 100%);
  margin-top: var(--space-4);
}

.help-search__icon {
  position: absolute;
  left: var(--space-4);
  color: var(--label-2);
  pointer-events: none;
}

.help-search__input {
  width: 100%;
  height: 48px;
  padding: 0 var(--space-12) 0 calc(var(--space-4) + 20px + var(--space-3));
  border: 1px solid var(--label-3);
  border-radius: var(--radius-pill);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  font-size: 16px;
  outline: none;
  appearance: none;
  transition:
    border-color var(--dur-micro) var(--ease-standard),
    box-shadow var(--dur-micro) var(--ease-standard);
}

.help-search__input::placeholder {
  color: var(--label-2);
}

.help-search__input::-webkit-search-cancel-button {
  margin-right: var(--space-2);
}

.help-search__input:hover {
  border-color: var(--label-2);
}

.help-search__input:focus {
  border-color: var(--accent);
  outline: 2px solid transparent;
  box-shadow: 0 0 0 1px var(--accent), 0 0 0 4px var(--accent-soft);
}

.help-search__kbd {
  position: absolute;
  right: var(--space-4);
  min-width: 22px;
  padding: 0 var(--space-1);
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-xs);
  color: var(--label-2);
  font-family: var(--font-sans);
  font-size: var(--type-caption-size);
  text-align: center;
  pointer-events: none;
}

.help-search__input:focus ~ .help-search__kbd,
.help-search__input:not(:placeholder-shown) ~ .help-search__kbd {
  display: none;
}

/* ---- categories --------------------------------------------------------------- */
.help-categories__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: var(--space-4);
  list-style: none;
}

.help-category {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: flex-start;
  justify-content: flex-start;
  font-weight: inherit;
  line-height: inherit;
  white-space: normal;
  user-select: auto;
  width: 100%;
  min-height: 112px;
  padding: var(--space-5);
  border: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    box-shadow var(--dur-toggle) var(--ease-standard),
    transform var(--dur-toggle) var(--ease-standard);
}

.help-category:hover {
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-2);
  transform: translateY(-1px);
}

.help-category:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.help-category.is-selected {
  box-shadow: 0 0 0 2px var(--accent), var(--shadow-1);
}

.help-category__icon {
  margin-bottom: var(--space-2);
  color: var(--accent);
}

.help-category__name {
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.help-category__count {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

/* ---- results ------------------------------------------------------------------ */
.help-results__title {
  margin-bottom: var(--space-4);
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
}

.help-list {
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
  list-style: none;
}

.help-list__item {
  position: relative;
  display: flex;
  align-items: center;
}

.help-list__item + .help-list__item {
  border-top: 1px solid var(--separator);
}

.help-list__link {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-4) var(--space-12) var(--space-4) var(--space-6);
  color: inherit;
  text-decoration: none;
}

.help-list__link:hover {
  background: var(--fill-1);
}

.help-list__link:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.help-list__title {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.help-list__excerpt {
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.help-list__meta {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.help-list__chevron {
  position: absolute;
  right: var(--space-4);
  color: var(--label-3);
  pointer-events: none;
}

/* ---- article ------------------------------------------------------------------ */
.help-article {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  width: 100%;
  max-width: var(--size-content-read);
  margin: 0 auto;
}

.help-article__back {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  align-self: flex-start;
  font-size: var(--type-callout-size);
  text-decoration: none;
}

.help-article__back:hover,
.help-toc__link:hover {
  text-decoration: underline;
}

.help-article__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: calc(var(--space-4) * -1);
}

.help-article__title {
  font-size: var(--type-title-1-size);
  font-weight: var(--type-title-1-weight);
  line-height: var(--type-title-1-line);
  letter-spacing: var(--type-title-1-tracking);
}

.help-article__title:focus {
  outline: none;
}

.help-article__meta {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.help-toc {
  padding: var(--space-5) var(--space-6);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.help-toc__title {
  margin-bottom: var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.help-toc__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  list-style: none;
}

.help-toc__item--3 {
  padding-left: var(--space-4);
}

.help-toc__link {
  font-size: var(--type-callout-size);
  text-decoration: none;
}

.help-pager {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
  padding-top: var(--space-8);
  border-top: 1px solid var(--separator);
}

.help-pager__link {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-4) var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
  color: inherit;
  text-decoration: none;
}

.help-pager__link:hover {
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-2);
}

.help-pager__link:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.help-pager__link--next {
  align-items: flex-end;
  text-align: right;
}

.help-pager__label {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--accent);
  font-size: var(--type-callout-size);
}

.help-pager__title {
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.help-contact {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  text-align: center;
}

@media (prefers-reduced-motion: reduce) {
  .help-category {
    transition: none;
  }

  .help-category:hover {
    transform: none;
  }
}

@media (max-width: 833px) {
  .help {
    gap: var(--space-8);
  }

  .help-hero {
    align-items: stretch;
    text-align: left;
  }

  .help-search__kbd {
    display: none;
  }

  .help-categories__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .help-category {
    min-height: 96px;
    padding: var(--space-4);
  }

  .help-list__link {
    padding-left: var(--space-4);
  }

  .help-pager {
    grid-template-columns: minmax(0, 1fr);
  }

  .help-pager__link--next {
    align-items: flex-start;
    text-align: left;
  }
}

/* Touch: a 44 px hit area around the back link without changing the look. */
@media (pointer: coarse) {
  .help-article__back {
    position: relative;
  }

  .help-article__back::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
