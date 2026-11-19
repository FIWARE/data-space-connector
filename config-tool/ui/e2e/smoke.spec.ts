/** Smoke test of the built binary: authentication, S0, S1, the placeholder stages, opening a values.yaml and a profile round trip. */
import { expect, test, type Page } from '@playwright/test'
import { CROSS_SITE_URL_ENV, FIXTURE_CURRENT_CONTEXT, FIXTURE_OTHER_CONTEXT, KUBECONFIG_FIXTURE, PROFILE_DIR_ENV, PROVIDER_VALUES, TOKEN_URL_ENV, required } from './env.ts'

const HTTP_UNAUTHORIZED = 401
const START_HEADING = 'FIWARE Data Space Connector — Configurator'
const TARGET_HEADING = 'Where is this going?'
const PLACEHOLDER_STAGE = 'Role'
const NAMESPACE = 'provider'
const RELEASE_NAME = 'dsc-provider'
const OPEN_INPUT_LABEL = 'values.yaml to open'
const SAVE_PATH_LABEL = 'Profile path'
const PROFILE_NAME = 'smoke'
// Exact counts follow the bundle's recognition rules; the smoke test only pins that both sides are filled.
const LOADED_HEADING = /^Opened provider\.yaml: understood [1-9]\d*, carried over untouched [1-9]\d*$/
const CLASS_COUNT = 4

const tokenUrl = () => required(TOKEN_URL_ENV)
const origin = () => new URL(tokenUrl()).origin

async function expectStart(page: Page) {
  await expect(page.getByRole('heading', { name: START_HEADING })).toBeVisible()
}

async function openTarget(page: Page) {
  await page.goto(tokenUrl())
  await expectStart(page)
  await page.getByRole('button', { name: /New configuration/ }).click()
  await expect(page.getByRole('heading', { name: TARGET_HEADING })).toBeVisible()
}

test('a request without the token is refused', async ({ request }) => {
  for (const path of ['/', '/api/model']) {
    const response = await request.get(origin() + path)
    expect(response.status(), path).toBe(HTTP_UNAUTHORIZED)
  }
})

test('the token URL opens the start screen', async ({ page }) => {
  await page.goto(tokenUrl())
  await expectStart(page)
})

test('the token URL followed from another site opens the start screen', async ({ page }) => {
  await page.goto(required(CROSS_SITE_URL_ENV))
  await page.getByRole('link', { name: 'open dsc-config' }).click()
  await expectStart(page)
})

test('Target keeps every value across a jump to a placeholder stage', async ({ page }) => {
  await openTarget(page)
  const kubeconfig = page.getByLabel('Kubeconfig')
  const context = page.getByLabel('Context', { exact: true })
  const namespace = page.getByLabel('Namespace')
  const createNamespace = page.getByRole('checkbox', { name: 'create it' })
  const releaseName = page.getByLabel('Release name')
  const chart = page.getByLabel('Chart', { exact: true })
  const version = page.getByLabel('Version')
  const cluster = page.getByRole('radio', { name: /Kubernetes cluster/ })
  const production = page.getByRole('radio', { name: /Production/ })

  await test.step('fill all fields', async () => {
    await cluster.check()
    await expect(kubeconfig).toHaveValue(KUBECONFIG_FIXTURE)
    await expect(context).toHaveValue(FIXTURE_CURRENT_CONTEXT)
    await context.selectOption(FIXTURE_OTHER_CONTEXT)
    await namespace.fill(NAMESPACE)
    await createNamespace.check()
    await releaseName.fill(RELEASE_NAME)
    await production.check()
    await expect(version).not.toHaveValue('')
  })
  const chosenChart = await chart.inputValue()
  const chosenVersion = await version.inputValue()

  await test.step('jump to a placeholder stage', async () => {
    await page.getByRole('navigation', { name: 'Stages' }).getByRole('button', { name: PLACEHOLDER_STAGE }).click()
    await expect(page.getByRole('heading', { name: `${PLACEHOLDER_STAGE} — not available yet` })).toBeVisible()
  })

  await test.step('return to Target with all values intact', async () => {
    await page.getByRole('navigation', { name: 'Stages' }).getByRole('button', { name: 'Target' }).click()
    await expect(page.getByRole('heading', { name: TARGET_HEADING })).toBeVisible()
    await expect(cluster).toBeChecked()
    await expect(kubeconfig).toHaveValue(KUBECONFIG_FIXTURE)
    await expect(context).toHaveValue(FIXTURE_OTHER_CONTEXT)
    await expect(namespace).toHaveValue(NAMESPACE)
    await expect(createNamespace).toBeChecked()
    await expect(releaseName).toHaveValue(RELEASE_NAME)
    await expect(chart).toHaveValue(chosenChart)
    await expect(version).toHaveValue(chosenVersion)
    await expect(production).toBeChecked()
  })
})

test('opening k3s/provider.yaml lands on Review with the load report', async ({ page }) => {
  await page.goto(tokenUrl())
  await expectStart(page)
  await page.getByLabel(OPEN_INPUT_LABEL).setInputFiles(PROVIDER_VALUES)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(LOADED_HEADING)
  await expect(page.getByLabel('Counts').getByRole('definition')).toHaveCount(CLASS_COUNT)
  await expect(page.getByRole('navigation', { name: 'Stages' }).getByRole('button', { name: /Expert/ })).toBeEnabled()
})

test('a saved profile reopens from the recent list on Review with its open question', async ({ page }) => {
  const path = `${required(PROFILE_DIR_ENV)}/${PROFILE_NAME}.dscprofile.yaml`
  await openTarget(page)
  await page.getByLabel('Namespace').fill(NAMESPACE)
  await page.getByRole('button', { name: 'Ask Namespace later' }).click()
  await page.getByRole('navigation', { name: 'Stages' }).getByRole('button', { name: /Save profile/ }).click()
  await page.getByLabel(SAVE_PATH_LABEL).fill(path)
  await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('navigation', { name: 'Stages' }).getByRole('status')).toHaveText(`Saved ${PROFILE_NAME}`)

  await page.goto(tokenUrl())
  await page.getByRole('region', { name: 'Recent' }).getByRole('button', { name: new RegExp(`^${PROFILE_NAME} ·`) }).click()
  await expect(page.getByRole('navigation', { name: 'Stages' }).getByRole('button', { name: /Review/ })).toHaveAttribute('aria-current', 'step')
  await expect(page.getByRole('heading', { name: /Open questions/ }).locator('..')).toContainText('Namespace')
})
