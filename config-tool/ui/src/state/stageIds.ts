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
