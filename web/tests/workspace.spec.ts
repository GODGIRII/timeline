import { test, expect, type Page } from '@playwright/test';

async function register(page: Page, name: string) {
  await page.goto('/');
  await page.getByRole('button', { name: 'Create an account', exact: true }).click();
  await page.getByLabel('Your name').fill(name);
  await page.getByLabel('Username').fill(`${name.toLowerCase()}_${Date.now()}`);
  await page.getByLabel('Password', { exact: true }).fill('test-password-for-timeline');
  await page.getByRole('button', { name: 'Create account', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Big plans start with a little space.' })).toBeVisible();
}
async function createSpace(page: Page) {
  await page.getByRole('button', { name: 'Create your first space' }).click();
  await page.getByLabel('Space name').fill('The Sunday Studio');
  await page.getByRole('button', { name: 'Create space', exact: true }).click();
  await expect(page.getByRole('button', { name: 'New item', exact: true })).toBeEnabled();
}
async function createTask(page: Page, title: string) {
  await page.getByRole('button', { name: 'New item', exact: true }).click();
  await page.getByLabel('Title', { exact: true }).fill(title);
  await page.getByLabel('Description').fill('Bring the details together and share the next steps with the team.');
  await page.getByRole('group', { name: 'Priority', exact: true }).getByRole('button', { name: 'high', exact: true }).click();
  await page.getByRole('button', { name: 'Create task', exact: true }).click();
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
}

test('create, filter, edit, conflict, calendar, removal, and responsive layout', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/'); await page.screenshot({ path: '/tmp/sequence-auth.png', fullPage: true });
  await register(page, 'Alex'); await createSpace(page);
  await createTask(page, 'Shape the launch story');
  await page.getByRole('button', { name: 'Complete Shape the launch story', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Reopen Shape the launch story' })).toBeVisible();
  await page.getByRole('button', { name: 'Reopen Shape the launch story' }).click();
  await expect(page.getByRole('button', { name: 'Complete Shape the launch story', exact: true })).toBeVisible();
  const spaces = await (await page.request.get('/api/spaces')).json(); const id = spaces[0].id;
  for (const [index, title] of ['Design review & feedback', 'Gather the little details', 'Share a first look', 'Plan next week together'].entries()) {
    const date = new Date(); date.setDate(date.getDate() + (index < 2 ? 1 : 3));
    const day = `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
    const res = await page.request.post(`/api/spaces/${id}/items`, { headers: { Origin: 'http://localhost:4180' }, data: { operation_id: `seed-${index}`, type: index % 2 ? 'task' : 'event', title, description: ['A fresh perspective makes all the difference.', 'Everything in its right place.', 'Something worth putting out into the world.', 'A moment to look ahead.'][index], deadline: { date: day }, priority: index % 2 ? 'low' : 'medium', status: index === 3 ? 'done' : 'open' } }); expect(res.ok()).toBeTruthy();
  }
  await expect(page.getByTestId('item-card')).toHaveCount(5);
  await page.locator('.toast-stack').evaluate(el => el.querySelectorAll('button').forEach(button => button.click()));
  await expect(page.locator('.toast')).toHaveCount(0);
  await page.screenshot({ path: '/tmp/sequence-desktop.png', fullPage: true });
  await page.getByLabel('Search items').fill('Shape'); await expect(page.getByTestId('item-card')).toHaveCount(1); await page.getByLabel('Search items').fill('');
  await page.getByLabel('Filter priority').selectOption('low'); await expect(page.getByTestId('item-card')).toHaveCount(2); await page.getByLabel('Filter priority').selectOption('all');
  await page.getByRole('heading', { name: 'Shape the launch story', exact: true }).click();
  const result = await (await page.request.get(`/api/spaces/${id}/items`)).json(); const item = result.items.find((i: { title: string }) => i.title === 'Shape the launch story');
  const conflict = await page.request.put(`/api/spaces/${id}/items/${item.id}`, { headers: { Origin: 'http://localhost:4180' }, data: { operation_id: 'other-client-edit', base_version: item.version, type: item.type, title: item.title, description: 'Changed on another device', deadline: item.deadline, priority: item.priority, status: item.status } }); expect(conflict.ok()).toBeTruthy();
  await page.getByLabel('Title', { exact: true }).fill('My conflicting title');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('alert')).toContainText('changed since you opened it');
  await page.getByRole('button', { name: 'Reload latest version' }).click();
  await expect(page.getByLabel('Description')).toHaveValue('Changed on another device');
  await page.getByLabel('Title', { exact: true }).fill('A clear launch story');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('heading', { name: 'A clear launch story', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Calendar view', exact: true }).click();
  await expect(page.locator('.calendar-item')).toHaveCount(5);
  await page.getByRole('button', { name: 'List view', exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => page.locator('.sidebar').evaluate(el => el.getBoundingClientRect().right)).toBeLessThanOrEqual(0);
  await page.screenshot({ path: '/tmp/sequence-mobile.png', fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.getByRole('button', { name: 'Open navigation' }).click();
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('button', { name: 'Activity', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'The latest happenings' })).toBeVisible();
  await page.getByRole('button', { name: 'Open navigation' }).click();
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('button', { name: /Timeline/ }).click();
  await page.getByRole('heading', { name: 'A clear launch story', exact: true }).click();
  await page.getByRole('button', { name: 'Remove item', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm removal' }).click();
  await expect(page.getByRole('heading', { name: 'A clear launch story', exact: true })).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('two people join, receive live changes, and obey admin permissions', async ({ page, browser }) => {
  await register(page, 'Owner'); await createSpace(page);
  await page.getByRole('button', { name: 'Invite people' }).click();
  const key = await page.getByLabel('Invitation key').inputValue();
  const memberContext = await browser.newContext(); const member = await memberContext.newPage();
  await register(member, 'Member');
  await member.getByRole('main').getByRole('button', { name: 'Join a space', exact: true }).click();
  await member.getByLabel('Invitation key').fill(key);
  await member.getByRole('button', { name: 'Request to join' }).click();
  await expect(member.getByRole('heading', { name: 'You’re on the list.' })).toBeVisible();
  await member.getByRole('button', { name: 'Got it' }).click();
  await page.getByRole('button', { name: 'Allow viewing' }).click();
  await expect(member.getByText('You have viewing access to this shared space.')).toBeVisible();
  await expect(member.getByRole('button', { name: 'New item', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await createTask(page, 'A shared moment');
  await expect(member.getByRole('heading', { name: 'A shared moment', exact: true })).toBeVisible();
  await expect(member.getByRole('button', { name: 'Complete A shared moment', exact: true })).toBeDisabled();
  await expect(member.locator('.toast')).toContainText('A shared moment');
  await page.getByRole('button', { name: 'Invite people' }).click();
  await page.getByLabel('Permission for Member').selectOption('editor');
  await expect(member.getByRole('button', { name: 'New item', exact: true })).toBeEnabled();
  await createTask(member, 'Member’s contribution');
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.getByRole('heading', { name: 'Member’s contribution', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Invite people' }).click();
  await page.getByRole('button', { name: 'Remove Member', exact: true }).click();
  await expect(member.getByRole('heading', { name: 'A shared moment', exact: true })).toHaveCount(0);
  await expect(member.getByRole('button', { name: 'New item', exact: true })).toHaveCount(0);
  await memberContext.close();
});

test('uncertain saves can be retried without duplicate tasks', async ({ page }) => {
  await register(page, 'Retry'); await createSpace(page);
  // Let the server commit, but drop its response to model lost acknowledgment.
  let lost = false;
  await page.route('**/api/spaces/*/items', async route => {
    if (route.request().method() === 'POST' && !lost) { lost = true; await route.fetch(); await route.abort(); }
    else await route.continue();
  });
  await page.getByRole('button', { name: 'New item', exact: true }).click();
  await page.getByLabel('Title', { exact: true }).fill('Saved just once');
  await page.getByRole('button', { name: 'Create task', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('reach the server');
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.getByRole('button', { name: 'Confirm change' })).toBeVisible();
  await page.getByRole('button', { name: 'Confirm change' }).click();
  await expect(page.getByRole('button', { name: 'Confirm change' })).toHaveCount(0);
  await expect(page.getByTestId('item-card')).toHaveCount(1);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Saved just once', exact: true })).toBeVisible();
});
