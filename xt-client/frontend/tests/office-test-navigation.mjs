export const officePane = page => page.getByRole('complementary', {name: '工作区'});
export const workspaceToggle = page => page.locator('.agent-context-bar').getByRole('button', {name: '工作区', exact: true});

export async function pickOfficeProject(page, name) {
  await page.locator('.agent-project-trigger').click();
  await page.getByRole('dialog', {name: '选择项目'}).getByRole('button', {name, exact: true}).click();
  await page.locator('.agent-project-trigger').getByText(name, {exact: true}).waitFor();
}

export async function openOffice(page, name) {
  if (name) await pickOfficeProject(page, name);
  const pane = officePane(page);
  if (await pane.isVisible()) await pane.getByRole('button', {name: '办公室', exact: true}).click();
  else await workspaceToggle(page).click();
  await pane.locator('.office-live-page').waitFor();
  return pane;
}
