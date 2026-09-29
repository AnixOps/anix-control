import type { Env } from '../types'

export type AnalyticsEvent = AnalyticsEngineDataPoint

export function writeAnalyticsEvent(env: Env, event: AnalyticsEvent): void {
  env.ANALYTICS?.writeDataPoint(event)
}
