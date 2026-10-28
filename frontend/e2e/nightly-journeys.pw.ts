import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const fixtureAPI = 'http://127.0.0.1:8765';

async function setScenario(request: APIRequestContext, scenario: 'review' | 'passed') {
  const response = await request.post(`${fixtureAPI}/__fixture/reset`, { data: { scenario } });
  expect(response.ok()).toBeTruthy();
}

async function visitQueue(page: Page) {
  await page.goto('/assets');
  await expect(page.getByText('episode.mkv').first()).toBeVisible();
  await page.getByRole('link', { name: 'Queue', exact: true }).click();
  await expect(page).toHaveURL(/\/queue$/);
  await expect(page.getByRole('heading', { name: 'Queue' })).toBeVisible();
  await expect(page.getByText('#1 · episode.mkv', { exact: true })).toBeVisible();
}

async function expectReadOnlyFixture(request: APIRequestContext) {
  const response = await request.get(`${fixtureAPI}/__fixture/metrics`);
  expect(response.ok()).toBeTruthy();
  const metrics = await response.json() as { unexpected: string[]; queueMutations: string[] };
  expect(metrics.unexpected).toEqual([]);
  expect(metrics.queueMutations).toEqual([]);
}

test('Assets -> Queue -> job status -> Validation', async ({ page, request }) => {
  await setScenario(request, 'review');
  await visitQueue(page);

  const job = page.getByTestId('queue-job-28701');
  await expect(job.getByText('Completed', { exact: true })).toBeVisible();
  await job.getByRole('link', { name: 'Validate for Job #1' }).click();
  await expect(page).toHaveURL(/\/validation$/);
  await expect(page.getByRole('heading', { name: 'Validation' })).toBeVisible();
  await expect(page.getByText(/episode\.mkv/).first()).toBeVisible();
  await expectReadOnlyFixture(request);
});

test('Queue -> Validation Review', async ({ page, request }) => {
  await setScenario(request, 'review');
  await page.goto('/queue');
  await expect(page.getByRole('heading', { name: 'Queue' })).toBeVisible();
  await expect(page.getByText('#1 · episode.mkv', { exact: true })).toBeVisible();

  await page.getByRole('link', { name: 'Validation', exact: true }).click();
  await expect(page).toHaveURL(/\/validation$/);
  await expect(page.getByText('Review exceptions from immutable Validation runs.')).toBeVisible();
  await expect(page.getByText(/episode\.mkv/).first()).toBeVisible();
  await expectReadOnlyFixture(request);
});

test('Validation PASS -> Publisher', async ({ page, request }) => {
  await setScenario(request, 'passed');
  await page.goto('/queue');
  await expect(page.getByRole('heading', { name: 'Queue' })).toBeVisible();
  const job = page.getByTestId('queue-job-28701');
  await expect(job.getByText('Completed', { exact: true })).toBeVisible();
  await job.getByRole('link', { name: 'Publish for Job #1' }).click();
  await expect(page).toHaveURL(/\/publisher$/);
  await expect(page.getByRole('heading', { name: 'Publisher', exact: true })).toBeVisible();
  await expect(page.getByText(/episode\.mkv/).first()).toBeVisible();
  await expect(page.getByText('Eligibility: ALLOWED', { exact: true })).toBeVisible();
  await expect(page.getByText('PASS', { exact: true })).toBeVisible();
  await expectReadOnlyFixture(request);
});
