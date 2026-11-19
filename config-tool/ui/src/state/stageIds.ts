/** Stage identifiers, apart from stages.ts so the store does not import stage components. */
export const StageId = {
  Target: 'target',
  Role: 'role',
  Foundations: 'foundations',
  Infrastructure: 'infrastructure',
  Features: 'features',
  Configuration: 'configuration',
  Review: 'review',
  Install: 'install',
} as const
export type StageId = (typeof StageId)[keyof typeof StageId]

/** The stage the chrome opens on. */
export const FIRST_STAGE: StageId = StageId.Target

/** Rail label of each stage; here so views outside the rail can name a stage without importing it. */
export const STAGE_LABELS: Record<StageId, string> = {
  [StageId.Target]: 'Target',
  [StageId.Role]: 'Role',
  [StageId.Foundations]: 'Foundations',
  [StageId.Infrastructure]: 'Infrastructure',
  [StageId.Features]: 'Features',
  [StageId.Configuration]: 'Configuration',
  [StageId.Review]: 'Review',
  [StageId.Install]: 'Install',
}
