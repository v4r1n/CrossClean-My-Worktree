import { test, expect, git } from "../support/app.js";

const rows = (page) => page.locator("#rows tr");
const row = (page, name) => rows(page).filter({ has: page.locator(".wt-name", { hasText: new RegExp(`^${name}$`) }) });

async function waitForScan(page, count = 3) {
  await expect(rows(page)).toHaveCount(count);
  await expect(page.locator("#phase")).toContainText("Scanned");
}

function linkedWorktrees(repo) {
  return git(repo, "worktree", "list", "--porcelain")
    .split("\n").filter((l) => l.startsWith("worktree ")).slice(1);
}

test("scans the home folder on launch and shows verdicts", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);

  await expect(row(page, "merged").locator(".verdict")).toHaveClass(/verdict-safe/);
  await expect(row(page, "dirty work").locator(".verdict")).toHaveClass(/verdict-risky/);
  await expect(row(page, "gone").locator(".verdict")).toHaveClass(/verdict-safe/);
  await expect(row(page, "dirty work")).toContainText("1 uncommitted");
  await expect(row(page, "gone")).toContainText("Folder already gone");
  await expect(row(page, "merged")).toContainText("feat/merged");

  await expect(page.locator("#repos li")).toHaveCount(2);
  await expect(page.locator("#repos li").nth(1)).toContainText("app");
  await expect(page.locator("#summary")).toContainText("3 worktrees");
  await expect(page.locator("#free")).toContainText("free");
});

test("filters, searches and sorts", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);

  await page.getByRole("button", { name: "Safe to delete", exact: true }).click();
  await expect(rows(page)).toHaveCount(2);
  await page.getByRole("button", { name: "Needs review" }).click();
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page)).toContainText("dirty work");
  await page.getByRole("button", { name: "All", exact: true }).click();

  await page.locator("#search").fill("feat/gone");
  await expect(rows(page)).toHaveCount(1);
  await page.locator("#search").fill("nothing-matches");
  await expect(page.locator("#empty")).toHaveText("No worktrees match the filter.");
  await page.locator("#search").fill("");

  await page.locator('th[data-sort="name"]').click();
  await expect(rows(page).first()).toContainText("dirty work");
  await page.locator('th[data-sort="name"]').click();
  await expect(rows(page).first()).toContainText("merged");
});

test("deletes safe worktrees without extra confirmation", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);

  await page.getByRole("button", { name: "Mark safe to delete" }).click();
  await expect(page.locator("#summary")).toContainText("2 marked");
  await page.getByRole("button", { name: "Delete marked…" }).click();

  const dialog = page.locator("#dialog");
  await expect(dialog).toContainText("Delete 2 worktrees?");
  await expect(dialog.locator(".warning")).toHaveCount(0);
  await dialog.getByRole("button", { name: "Delete 2 Worktrees" }).click();

  await expect(dialog).toContainText("Deleted 2 worktrees");
  await dialog.getByRole("button", { name: "Done" }).click();
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page)).toContainText("dirty work");

  const left = linkedWorktrees(app.fixture.repo);
  expect(left).toHaveLength(1);
  expect(left[0]).toContain("dirty work");
});

test("risky deletes need acknowledgement and can delete merged branches", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);

  // Shift-click marks the whole range.
  await rows(page).nth(0).locator("input.mark").click();
  await rows(page).nth(2).locator("input.mark").click({ modifiers: ["Shift"] });
  await expect(page.locator("#summary")).toContainText("3 marked");

  await page.keyboard.press("Delete");
  const dialog = page.locator("#dialog");
  await expect(dialog).toContainText("Delete 3 worktrees?");
  const confirm = dialog.getByRole("button", { name: "Delete 3 Worktrees" });
  await expect(dialog.locator(".warning")).toContainText("1 of these");
  await expect(confirm).toBeDisabled();

  await dialog.locator("#ack").check();
  await dialog.locator("#branches").check();
  await expect(confirm).toBeEnabled();
  await confirm.click();

  await expect(dialog).toContainText("Deleted 3 worktrees");
  // feat/dirty has no commits of its own, so `git branch -d` removes all three.
  await expect(dialog).toContainText("Deleted 3 merged branches");
  await dialog.getByRole("button", { name: "Done" }).click();
  await expect(page.locator("#empty")).toHaveText("No git worktrees found here.");

  expect(linkedWorktrees(app.fixture.repo)).toHaveLength(0);
  expect(git(app.fixture.repo, "branch", "--format=%(refname:short)").trim()).toBe("main");
});

test("cancelling the dialog deletes nothing", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);
  await row(page, "merged").locator("input.mark").check();
  await page.getByRole("button", { name: "Delete marked…" }).click();
  await page.locator("#dialog").getByRole("button", { name: "Cancel" }).click();
  await expect(page.locator("#dialog")).not.toBeVisible();
  await expect(rows(page)).toHaveCount(3);
  expect(linkedWorktrees(app.fixture.repo)).toHaveLength(3);
});

test("scans a folder chosen by path and remembers it", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);

  await page.locator("#target").selectOption("choose");
  await expect(page.locator("#folder-form")).toBeVisible();
  await page.locator("#folder-input").fill(app.fixture.dirty);
  await page.getByRole("button", { name: "Scan", exact: true }).click();

  await expect(page.locator("#folder-form")).toBeHidden();
  // A worktree's own folder still leads back to its repository.
  await waitForScan(page);
  await expect(page.locator("#target option:checked")).toContainText("dirty work");

  await page.reload();
  await waitForScan(page);
  await expect(page.locator("#target option:checked")).toContainText("dirty work");
});

test("rejects a folder that doesn't exist", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);
  await page.locator("#target").selectOption("choose");
  await page.locator("#folder-input").fill(app.home + "/no/such/folder");
  await page.locator("#folder-input").press("Enter");
  await expect(page.locator("#toast")).toHaveText("that folder doesn't exist");
});

test("row menu opens and closes", async ({ page, app }) => {
  await page.goto(app.url);
  await waitForScan(page);
  await row(page, "merged").getByRole("button", { name: "More actions" }).click();
  const menu = page.locator("#menu");
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("menuitem")).toHaveText(["Show in file manager", "Open in terminal", "Copy path"]);
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
});

test("a tab without the launch token can't use the API", async ({ page, app }) => {
  await page.goto(app.url.split("#")[0]);
  await expect(page.locator("#phase")).toContainText("isn't connected");
  await expect(rows(page)).toHaveCount(0);
});
