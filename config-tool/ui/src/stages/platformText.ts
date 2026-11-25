/** The Platform panel's copy. */
export const PLATFORM_PANEL = 'Platform'
export const PROFILE_PENDING = 'Derived once Stage 1 names the target environment.'
export const PROFILE_DERIVED = 'Derived from the target environment; choose another profile to pin it.'
export const CUSTOM_PROFILE = "Sets nothing: the chart's defaults stay in force."
export const COMPONENT_HEADER = 'Component'
export const REQUESTS_HEADER = 'Requests'
export const LIMITS_HEADER = 'Limits'
export const SIZING_LINK = 'Sizing guidance ↗'

/** The caption of the figures table. */
export function measuredCaption(measuredOn: string): string {
  return `Figures from the sizing guidance, measured on chart ${measuredOn}.`
}
