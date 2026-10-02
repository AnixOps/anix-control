<template>
  <div class="home">
    <header class="home__intro">
      <h1 class="home__greeting">{{ greeting }}</h1>
      <p v-if="sub.summary.value" class="home__summary" data-home-summary>
        {{ summaryText }}
        <template v-if="needsAction"> {{ isCommercial ? t('portal.home.next.commercial') : t('portal.home.next.community') }}</template>
      </p>
    </header>

    <div v-if="sub.loading.value && !sub.summary.value" class="home__hero-slot">
      <div v-if="showSkeleton" class="home-hero home-hero--skeleton" data-home-skeleton>
        <span class="home-hero__ring-bone" aria-hidden="true" />
        <UiSkeleton variant="text" :lines="5" :label="t('ui.loading')" />
      </div>
    </div>

    <LoadError
      v-else-if="sub.error.value"
      :title="t('portal.home.loadFailed')"
      :error="sub.error.value"
      @retry="sub.load()"
    />

    <section v-else class="home-hero" :aria-label="t('portal.home.ring.label', { remaining: remainingText, total: totalText })" data-home-hero>
      <div class="home-hero__ring">
        <svg viewBox="0 0 200 200" aria-hidden="true" focusable="false">
          <defs>
            <linearGradient :id="gradientId" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" class="home-hero__stop-start" />
              <stop offset="1" class="home-hero__stop-end" />
            </linearGradient>
          </defs>
          <circle class="home-hero__track" cx="100" cy="100" :r="RADIUS" />
          <circle
            v-if="ringLength > 0"
            class="home-hero__progress"
            cx="100"
            cy="100"
            :r="RADIUS"
            :stroke="`url(#${gradientId})`"
            :stroke-dasharray="`${drawn ? ringLength : 0} ${CIRCUMFERENCE}`"
          />
        </svg>
        <div class="home-hero__ring-label" role="img" :aria-label="t('portal.home.ring.label', { remaining: remainingText, total: totalText })" data-home-remaining>
          <span class="home-hero__number" aria-hidden="true">{{ remainingParts.number }}</span>
          <span class="home-hero__caption" aria-hidden="true">
            {{ sub.total.value > 0 ? t('portal.home.ring.caption', { unit: remainingParts.unit, total: totalText }) : t('portal.home.ring.empty') }}
          </span>
        </div>
      </div>

      <div class="home-hero__body">
        <dl class="home-hero__facts">
          <div class="home-fact">
            <dt>{{ t('portal.home.facts.status') }}</dt>
            <dd><UiBadge :tone="statusTone" :label="t(`portal.home.status.${sub.status.value}`)" data-home-status /></dd>
          </div>
          <div class="home-fact">
            <dt>{{ t('portal.home.facts.expires') }}</dt>
            <dd>
              {{ sub.expiresAt.value > 0 ? format.date(sub.expiresAt.value) : t('portal.home.facts.never') }}
              <small v-if="expiryNote">{{ expiryNote }}</small>
            </dd>
          </div>
          <div class="home-fact">
            <dt>{{ t('portal.home.facts.used') }}</dt>
            <dd>
              {{ heroBytes(sub.used.value) }}
              <small class="home-fact__detail">{{ t('portal.home.facts.upDown', { up: heroBytes(sub.upload.value), down: heroBytes(sub.download.value) }) }}</small>
            </dd>
          </div>
        </dl>
        <p v-if="sub.planName.value" class="home-hero__plan">
          {{ isCommercial ? t('portal.home.plan.commercial', { name: sub.planName.value }) : t('portal.home.plan.community', { name: sub.planName.value }) }}
        </p>
        <div class="home-hero__actions">
          <UiButton variant="primary" size="lg" :icon="Copy" :disabled="!sub.link.value" data-home-copy @click="copyLink">
            {{ t('portal.home.copyLink') }}
          </UiButton>
          <UiButton :as="RouterLink" to="/user/subscribe" size="lg" :icon="Download" data-home-import>
            {{ t('portal.home.importToClient') }}
          </UiButton>
        </div>
        <p class="home-hero__updated">
          <span v-if="sub.cachedAt.value" :title="format.dateTime(sub.cachedAt.value)">{{ t('portal.home.updatedAt', { time: format.relativeTime(sub.cachedAt.value) }) }}</span>
          <UiButton variant="tertiary" size="sm" :icon="RotateCw" :loading="sub.refreshing.value" data-home-refresh @click="refresh">
            {{ t('portal.home.refresh') }}
          </UiButton>
        </p>
      </div>
    </section>

    <!-- Commercial edition only: buying and renewing. -->
    <section v-if="isCommercial && sub.summary.value" class="home-shop" data-home-shop>
      <UiIcon :icon="ShoppingBag" :size="20" class="home-shop__icon" />
      <p class="home-shop__text">{{ sub.status.value === 'active' ? t('portal.plans.description') : t('portal.home.next.commercial') }}</p>
      <div class="home-shop__actions">
        <UiButton :as="RouterLink" to="/user/orders" variant="tertiary">{{ t('portal.home.orders') }}</UiButton>
        <UiButton :as="RouterLink" to="/user/plans" variant="secondary">{{ t('portal.home.buyPlan') }}</UiButton>
      </div>
    </section>

    <div class="home__columns">
      <section class="home-panel" aria-labelledby="home-help-title" data-home-help>
        <div class="home-panel__head">
          <h2 id="home-help-title" class="home-panel__title">{{ t('portal.home.help.title') }}</h2>
          <RouterLink to="/user/knowledge" class="home-panel__link">{{ t('portal.home.help.all') }}</RouterLink>
        </div>
        <div class="home-panel__card">
          <UiSkeleton v-if="articles.state === 'loading' && showListSkeleton" variant="text" :lines="3" />
          <p v-else-if="articles.state === 'failed'" class="home-panel__note">
            {{ t('portal.help.loadFailed') }}
            <UiButton variant="tertiary" size="sm" @click="loadArticles">{{ t('portal.state.retry') }}</UiButton>
          </p>
          <p v-else-if="articles.state === 'ready' && !articles.list.length" class="home-panel__note">{{ t('portal.home.help.empty') }}</p>
          <ul v-else-if="articles.state === 'ready'" class="home-list">
            <li v-for="(article, index) in articles.list" :key="article.id" class="home-list__item">
              <UiIcon :icon="index === 0 ? Megaphone : BookOpen" :size="20" class="home-list__icon" :class="{ 'is-accent': index === 0 }" />
              <div class="home-list__text">
                <RouterLink :to="{ path: '/user/knowledge', query: { article: String(article.id) } }" class="home-list__title">{{ article.title }}</RouterLink>
                <p class="home-list__meta">
                  <span v-if="articleSummary(article)">{{ articleSummary(article) }} · </span>
                  <span :title="format.dateTime(article.updated_at)">{{ format.date(article.updated_at) }}</span>
                </p>
              </div>
            </li>
          </ul>
        </div>
      </section>

      <section class="home-panel" aria-labelledby="home-tickets-title" data-home-tickets>
        <div class="home-panel__head">
          <h2 id="home-tickets-title" class="home-panel__title">{{ t('portal.home.tickets.title') }}</h2>
          <RouterLink :to="{ path: '/user/tickets', query: { new: '1' } }" class="home-panel__link">{{ t('portal.home.tickets.new') }}</RouterLink>
        </div>
        <div class="home-panel__card">
          <UiSkeleton v-if="tickets.state === 'loading' && showListSkeleton" variant="text" :lines="2" />
          <p v-else-if="tickets.state === 'failed'" class="home-panel__note">
            {{ t('portal.tickets.errors.load') }}
            <UiButton variant="tertiary" size="sm" @click="loadTickets">{{ t('portal.state.retry') }}</UiButton>
          </p>
          <p v-else-if="tickets.state === 'ready' && !tickets.list.length" class="home-panel__note">{{ t('portal.home.tickets.empty') }}</p>
          <ul v-else-if="tickets.state === 'ready'" class="home-list">
            <li v-for="ticket in tickets.list" :key="ticket.id" class="home-list__item">
              <UiIcon :icon="MessageSquare" :size="20" class="home-list__icon" />
              <div class="home-list__text">
                <div class="home-list__row">
                  <RouterLink :to="{ path: '/user/tickets', query: { ticket: String(ticket.id) } }" class="home-list__title">{{ ticket.subject }}</RouterLink>
                  <UiBadge :tone="ticketTone(ticket.status)" :label="ticketStatus(ticket.status)" />
                </div>
                <p class="home-list__meta">
                  {{ t('portal.tickets.number', { id: ticket.id }) }} ·
                  <span :title="format.dateTime(ticket.updated_at)">{{ format.relativeTime(ticket.updated_at) }}</span>
                </p>
              </div>
            </li>
          </ul>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
// 概览 (plan §8.1): a greeting, one hero card with the remaining traffic
// (brand-gradient ring and number: a brand moment, guidelines/color.md
// rule 4), status, expiry and usage, 复制订阅链接 as the primary action and
// 导入到客户端 next to it; below, the first help articles (in the order the
// administrator sorted them) and the latest tickets. Data: GET
// /user/subscription and /user/profile (useUserSubscription), /user/knowledge
// and /user/ticket. The commercial edition adds a card for plans and orders.
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { BookOpen, Copy, Download, Megaphone, MessageSquare, RotateCw, ShoppingBag } from '@lucide/vue'
import { getKnowledgeList, getTickets } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import { useUserSubscription } from '@/composables/useUserSubscription'
import { useUserStore } from '@/stores/user'
import { listOf, unwrapPanel } from '@/utils/panelResponse'
import { articleExcerpt } from '@/utils/articleMarkup'
import LoadError from '@/components/common/LoadError.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const RADIUS = 88
const CIRCUMFERENCE = 2 * Math.PI * RADIUS
const gradientId = 'home-ring-gradient'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const userStore = useUserStore()
const { isCommercial } = useEdition()
const sub = useUserSubscription()
const showSkeleton = useDelayedLoading(sub.loading)
const drawn = ref(false)

const name = computed(() => String(userStore.userInfo?.email || '').split('@')[0])
const greeting = computed(() => (name.value ? t('portal.home.greeting', { name: name.value }) : t('portal.home.greetingAnonymous')))

const percentText = computed(() => format.percent(sub.remainingRatio.value))
const summaryText = computed(() => {
  switch (sub.status.value) {
    case 'expired': return t('portal.home.summary.expired', { date: format.date(sub.expiresAt.value) })
    case 'none': return t('portal.home.summary.none')
    case 'exhausted': return t('portal.home.summary.exhausted')
    default: return sub.remainingRatio.value < 0.1
      ? t('portal.home.summary.low', { percent: percentText.value })
      : t('portal.home.summary.active', { percent: percentText.value })
  }
})
const needsAction = computed(() => ['expired', 'none', 'exhausted'].includes(sub.status.value))

const statusTone = computed(() => ({ active: 'success', expired: 'danger', exhausted: 'warning', none: 'neutral' })[sub.status.value])

const expiryNote = computed(() => {
  if (sub.expiresAt.value <= 0) return ''
  if (sub.status.value === 'expired') return t('portal.home.facts.ended')
  const days = sub.daysLeft.value
  if (days === null) return ''
  return days === 0 ? t('portal.home.facts.lastDay') : t('portal.home.facts.daysLeft', { n: days }, days)
})

// The hero number: "128.4" large, the unit in the caption under it.
// One decimal, without a trailing ".0" (200 GB, not 200.0 GB).
function heroBytes(value) {
  return format.bytes(value, { precision: 1, empty: '0 B' }).replace(/[.,]0(?= )/, '')
}
const remainingText = computed(() => heroBytes(sub.remaining.value))
const totalText = computed(() => heroBytes(sub.total.value))
const remainingParts = computed(() => {
  const text = remainingText.value
  const split = text.lastIndexOf(' ')
  const unit = sub.remaining.value > 0 ? text.slice(split + 1) : (totalText.value.split(' ').pop() || 'GB')
  return { number: sub.remaining.value > 0 ? text.slice(0, split) : '0', unit }
})
const ringLength = computed(() => CIRCUMFERENCE * sub.remainingRatio.value)

async function copyLink() {
  if (await copyText(sub.link.value)) toast.success(t('portal.home.copied'))
  else toast.error(t('portal.home.copyFailed'))
}

async function refresh() {
  await sub.load({ refresh: true })
}

// --- side panels -------------------------------------------------------------
const articles = reactive({ state: 'loading', list: [] })
const tickets = reactive({ state: 'loading', list: [] })
const showListSkeleton = useDelayedLoading(computed(() => articles.state === 'loading' || tickets.state === 'loading'))

async function loadArticles() {
  articles.state = 'loading'
  try {
    articles.list = listOf(unwrapPanel(await getKnowledgeList())).slice(0, 3)
    articles.state = 'ready'
  } catch {
    articles.state = 'failed'
  }
}

// The first words of the article, else its category.
function articleSummary(article) {
  return articleExcerpt(article.body, 36) || article.category || ''
}

function timeOf(value) {
  const n = typeof value === 'number' ? (value > 1e12 ? value : value * 1000) : Date.parse(value)
  return Number.isFinite(n) ? n : 0
}

async function loadTickets() {
  tickets.state = 'loading'
  try {
    tickets.list = [...listOf(unwrapPanel(await getTickets()))]
      .sort((a, b) => timeOf(b.updated_at) - timeOf(a.updated_at))
      .slice(0, 3)
    tickets.state = 'ready'
  } catch {
    tickets.state = 'failed'
  }
}

function ticketStatus(status) {
  return t(`portal.tickets.status.${['open', 'answered', 'closed'][status] || 'open'}`)
}

function ticketTone(status) {
  return ['info', 'success', 'neutral'][status] || 'neutral'
}

onMounted(async () => {
  loadArticles()
  loadTickets()
  await sub.load()
  // Draw the ring in after the first paint (no motion under reduced motion: CSS).
  requestAnimationFrame(() => { drawn.value = true })
})
</script>

<style scoped>
.home {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.home__intro {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.home__greeting {
  font-size: var(--type-display-size);
  font-weight: var(--type-display-weight);
  line-height: var(--type-display-line);
  letter-spacing: var(--type-display-tracking);
  overflow-wrap: anywhere;
}

.home__summary {
  max-width: 60ch;
  color: var(--label-2);
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-regular);
  line-height: var(--type-title-3-line);
}

.home__hero-slot {
  min-height: 280px;
}

/* ---- hero ---------------------------------------------------------------- */
.home-hero {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-10);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.home-hero--skeleton {
  min-height: 280px;
}

.home-hero__ring-bone {
  width: 200px;
  height: 200px;
  border: 12px solid var(--fill-2);
  border-radius: 50%;
}

.home-hero__ring {
  position: relative;
  width: 200px;
  height: 200px;
}

.home-hero__ring svg {
  display: block;
  width: 100%;
  height: 100%;
  transform: rotate(-90deg);
}

.home-hero__track,
.home-hero__progress {
  fill: none;
  stroke-width: 12;
}

.home-hero__track {
  stroke: var(--fill-2);
}

.home-hero__progress {
  stroke-linecap: round;
  transition: stroke-dasharray 900ms var(--ease-emphasized);
}

.home-hero__stop-start {
  stop-color: var(--brand-gradient-start);
}

.home-hero__stop-end {
  stop-color: var(--brand-gradient-end);
}

.home-hero__ring-label {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}

/* Brand moment: the hero number in the brand gradient. The first stop is
   mixed 30 % towards indigo so the lightest glyph edge keeps 3:1 against
   white (large bold text, WCAG 1.4.3); dark mode passes either way. The
   size is the prototype's 40 px: Display (48) crowds the 200 px ring. */
.home-hero__number {
  --home-ring-number: 40px;

  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--brand-gradient-start) 70%, var(--brand-gradient-end)),
    var(--brand-gradient-end)
  );
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  font-size: var(--home-ring-number);
  font-weight: var(--weight-bold);
  font-variant-numeric: tabular-nums;
  line-height: 1;
  letter-spacing: -0.03em;
}

.home-hero__caption {
  max-width: 150px;
  margin-top: var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.home-hero__facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-6);
  margin: 0 0 var(--space-6);
  padding-bottom: var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.home-fact dt {
  margin-bottom: var(--space-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.home-fact dd {
  margin: 0;
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.home-fact dd small {
  display: block;
  margin-top: var(--space-0-5);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-regular);
  white-space: normal;
}

.home-hero__plan {
  margin-bottom: var(--space-4);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.home-hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
}

.home-hero__updated {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin-top: var(--space-4);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

/* ---- commercial card ----------------------------------------------------- */
.home-shop {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  align-items: center;
  margin-top: calc(var(--space-6) * -1);
  padding: var(--space-5) var(--space-6);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.home-shop__icon {
  color: var(--accent);
}

.home-shop__text {
  flex: 1 1 240px;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.home-shop__actions {
  display: flex;
  gap: var(--space-2);
}

/* ---- announcements and tickets ------------------------------------------- */
.home__columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-6);
}

.home-panel__head {
  display: flex;
  gap: var(--space-3);
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--space-4);
}

.home-panel__title {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
}

.home-panel__link {
  position: relative;
  font-size: var(--type-callout-size);
  text-decoration: none;
}

/* Touch: a 44 px hit area around the text link. */
@media (pointer: coarse) {
  .home-panel__link::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}

.home-panel__link:hover {
  text-decoration: underline;
}

.home-panel__card {
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.home-panel__note {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.home-list {
  list-style: none;
}

.home-list__item {
  position: relative;
  display: flex;
  gap: var(--space-3);
  padding: var(--space-4) 0;
  border-top: 1px solid var(--separator);
}

.home-list__item:first-child {
  padding-top: 0;
  border-top: 0;
}

.home-list__item:last-child {
  padding-bottom: 0;
}

.home-list__icon {
  flex: none;
  margin-top: 1px;
  color: var(--label-2);
}

.home-list__icon.is-accent {
  color: var(--accent);
}

.home-list__text {
  flex: 1;
  min-width: 0;
}

.home-list__row {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  justify-content: space-between;
}

.home-list__title {
  color: var(--label-1);
  font-weight: var(--weight-medium);
  text-decoration: none;
  overflow-wrap: anywhere;
}

/* The whole row is the link's target (a 44 px+ touch target). */
.home-list__title::after {
  position: absolute;
  inset: 0;
  content: '';
}

.home-list__title:hover {
  color: var(--accent);
}

.home-list__meta {
  margin-top: var(--space-0-5);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

@media (prefers-reduced-motion: reduce) {
  .home-hero__progress {
    transition: none;
  }
}

@media (max-width: 833px) {
  .home {
    gap: var(--space-8);
  }

  .home-hero {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-6);
    justify-items: center;
    padding: var(--space-6);
  }

  .home-hero__body {
    width: 100%;
  }

  .home-hero__ring,
  .home-hero__ring-bone {
    width: 180px;
    height: 180px;
  }

  .home-hero__facts {
    gap: var(--space-3);
  }

  .home-fact dd {
    font-size: var(--type-callout-size);
    white-space: normal;
  }

  /* Upload and download do not fit a third of a phone. */
  .home-fact__detail {
    display: none;
  }

  .home-hero__actions :deep(.ui-button) {
    flex: 1 1 100%;
  }

  .home-shop {
    margin-top: calc(var(--space-4) * -1);
  }

  .home__columns {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
