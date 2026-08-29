#!/usr/bin/env node
// Real-browser acceptance test for the configurable CLI launch commands.
// Drives the actual chat/admin UI served by scripts/ui-test-server (the same
// desktop.NewHandler + engine.New stack as the Mac app, minus the native
// shell) with Playwright chromium, and asserts:
//
//   1. Saving "<wrapper> claude" in the settings form returns 204, shows the
//      success feedback, and persists the command to <data-dir>/config.yaml.
//   2. The command survives a page reload (the form is prefilled from
//      /admin/status when the admin view opens).
//   3. An invalid first word ("nosuchcommand-xyz arg") is rejected with a 400,
//      a visible error, and config.yaml left unchanged.
//   4. After a full server restart on the same data dir, a project is added
//      through the admin form, then a chat session created through the real
//      composer (project picker, prompt, send button) streams the wrapper's
//      canned token, proving the persisted command is what actually spawns
//      the CLI.
//
// No API shortcuts: every UI assertion comes from real clicks/typing in the
// browser. Reading config.yaml from disk is evidence gathering, not a UI
// bypass. Prerequisites and usage:
//
//   go build -o /tmp/ui-test-server ./scripts/ui-test-server
//   NODE_PATH=$(npm root -g) node scripts/ui-test-cli-settings.js
//
// Screenshots are written under docs/testing/artifacts/<run>/ (git-ignored).

'use strict';

const { spawn } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { chromium } = require('playwright');

const REPO_ROOT = path.resolve(__dirname, '..');
const SERVER_BIN = process.env.UI_TEST_SERVER || '/tmp/ui-test-server';
const DESKTOP_ADDR = process.env.UI_TEST_ADDR || '127.0.0.1:9887';
const ADMIN_TOKEN = 'ui-test-admin-token';
const ARTIFACT_ROOT = process.env.UI_TEST_ARTIFACTS || path.join(REPO_ROOT, 'docs', 'testing', 'artifacts');
// The globally installed playwright package expects a newer browser revision
// than the one cached on this machine; pin to the cached chromium build.
const CHROMIUM_EXECUTABLE =
  process.env.UI_TEST_CHROMIUM ||
  path.join(
    os.homedir(),
    'Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
  );
const TIMEOUT = 20_000;

const runID = `${Date.now()}-${process.pid}`;
const workRoot = fs.mkdtempSync(path.join(os.tmpdir(), `codeafar-ui-${runID}-`));
const projectDir = path.join(workRoot, 'project');
const dataDir = path.join(workRoot, 'data');
const artifactDir = path.join(ARTIFACT_ROOT, runID);
for (const dir of [projectDir, dataDir, artifactDir]) fs.mkdirSync(dir, { recursive: true });

let server = null;
let browser = null;
let exitCode = 0;
const jsErrors = [];
const step = (name) => console.log(`\n=== ${name} ===`);

// Same wrapper shape as pkg/engine/fake_wrapper_e2e_test.go: it fails unless
// its first argument is the prepend word, then answers every stdin line with
// a canned stream-json turn carrying "hi from wrapper". It also answers
// `<command> --version` because the server probes the CLI version at startup.
function writeWrapper() {
  const wrapper = path.join(workRoot, 'fake-wrapper');
  const script = [
    '#!/bin/sh',
    'if [ "$1" = "--version" ] || [ "$2" = "--version" ]; then',
    '  echo "fake-wrapper 9.9.9"',
    '  exit 0',
    'fi',
    'if [ "$1" != "claude" ]; then',
    '  echo "wrapper got bad first arg: $1" >&2',
    '  exit 1',
    'fi',
    'while read -r _line; do',
    `  printf '%s\\n' '{"type":"system","subtype":"init","session_id":"ui-wrapper-session"}'`,
    `  printf '%s\\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi from wrapper"}}}'`,
    `  printf '%s\\n' '{"type":"result","subtype":"success","result":"hi from wrapper","is_error":false}'`,
    'done',
    ''
  ].join('\n');
  fs.writeFileSync(wrapper, script, { mode: 0o755 });
  return wrapper;
}

function startServer() {
  server = spawn(SERVER_BIN, [
    '-data-dir', dataDir,
    '-workdir', projectDir,
    '-desktop-addr', DESKTOP_ADDR,
    '-admin-token', ADMIN_TOKEN
  ], { stdio: ['ignore', 'pipe', 'pipe'] });
  server.stdout.on('data', (chunk) => process.stdout.write(`[server] ${chunk}`));
  server.stderr.on('data', (chunk) => process.stdout.write(`[server:err] ${chunk}`));
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('server did not report UI_TEST_READY in 30s')), 30_000);
    const onReady = (chunk) => {
      const match = /UI_TEST_READY (\S+)/.exec(String(chunk));
      if (match) {
        clearTimeout(timer);
        server.stdout.off('data', onReady);
        resolve(match[1]);
      }
    };
    server.stdout.on('data', onReady);
    server.once('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`server exited early with code ${code}`));
    });
  });
}

function stopServer() {
  if (!server || server.exitCode !== null) return Promise.resolve();
  return new Promise((resolve) => {
    const killTimer = setTimeout(() => { if (server.exitCode === null) server.kill('SIGKILL'); }, 3_000);
    server.once('exit', () => { clearTimeout(killTimer); resolve(); });
    server.kill('SIGTERM');
  });
}

const readConfig = () => fs.readFileSync(path.join(dataDir, 'config.yaml'), 'utf8');

function assert(condition, message) {
  if (!condition) throw new Error(`assertion failed: ${message}`);
}

// Opens the chat page and waits until the WebSocket is authenticated and the
// composer is usable, i.e. the engine is ready for a real user.
async function openChatPage(context) {
  const page = await context.newPage();
  page.on('pageerror', (error) => jsErrors.push(`pageerror: ${error.message}`));
  page.on('console', (message) => {
    // "Failed to load resource" is how chromium logs non-2xx responses; the
    // negative case below intentionally triggers one and asserts on it.
    if (message.type() === 'error' && !message.text().startsWith('Failed to load resource:')) {
      jsErrors.push(`console.error: ${message.text()}`);
    }
  });
  await page.goto(`http://${DESKTOP_ADDR}/#token=${ADMIN_TOKEN}`);
  await page.waitForSelector('#connection-state:has-text("已连接")', { timeout: TIMEOUT });
  await page.waitForFunction(() => !document.querySelector('#prompt').disabled, null, { timeout: TIMEOUT });
  return page;
}

// The settings form lives in the admin view. The real user path is the
// "管理与诊断" button in the sidebar; the form is prefilled from
// /admin/status as part of opening the view.
async function openAdminView(page) {
  await page.click('#show-admin');
  await page.waitForSelector('#admin-view:not([hidden])', { state: 'attached', timeout: TIMEOUT });
  await page.waitForSelector('#settings-form', { state: 'visible', timeout: TIMEOUT });
  await page.waitForFunction(
    () => {
      const workdir = document.querySelector('#settings-workdir');
      return workdir && workdir.value !== '';
    },
    null,
    { timeout: TIMEOUT }
  );
}

async function fillCommandAndSave(page, claudeCommand) {
  await page.fill('#settings-claude-command', claudeCommand);
  const [patchResponse] = await Promise.all([
    page.waitForResponse(
      (response) => response.url().includes('/admin/settings') && response.request().method() === 'PATCH',
      { timeout: TIMEOUT }
    ),
    page.click('#settings-form button.primary')
  ]);
  return patchResponse;
}

async function main() {
  const wrapper = writeWrapper();
  const goodCommand = `${wrapper} claude`;
  console.log(`work dir:  ${workRoot}`);
  console.log(`data dir:  ${dataDir}`);
  console.log(`wrapper:   ${goodCommand}`);
  console.log(`artifacts: ${artifactDir}`);

  browser = await chromium.launch({ executablePath: CHROMIUM_EXECUTABLE, headless: true });
  const context = await browser.newContext({ viewport: { width: 1180, height: 760 } });

  step('start server (first run)');
  console.log(`server ready: ${await startServer()}`);

  step('open chat page, navigate to admin settings through the UI');
  const page = await openChatPage(context);
  await openAdminView(page);
  await page.screenshot({ path: path.join(artifactDir, '01-admin-settings.png') });

  step('save valid wrapper command: expect PATCH 204, feedback, config.yaml');
  const patch = await fillCommandAndSave(page, goodCommand);
  assert(patch.status() === 204, `expected PATCH 204, got ${patch.status()}`);
  await page.waitForSelector('#admin-feedback:not(.error):has-text("运行设置已保存")', { timeout: TIMEOUT });
  const configAfterSave = readConfig();
  assert(configAfterSave.includes(`claudeCommand: ${goodCommand}`), `config.yaml missing wrapper command:\n${configAfterSave}`);
  console.log(`PATCH ${patch.status()} → feedback "运行设置已保存"; config.yaml persisted claudeCommand`);

  step('reload page, settings form still shows the wrapper command');
  await page.reload();
  await page.waitForSelector('#connection-state:has-text("已连接")', { timeout: TIMEOUT });
  await openAdminView(page);
  await page.waitForFunction(
    (expected) => document.querySelector('#settings-claude-command').value === expected,
    goodCommand,
    { timeout: TIMEOUT }
  );
  await page.screenshot({ path: path.join(artifactDir, '02-settings-reloaded.png') });
  console.log('form re-prefilled from /admin/status with the wrapper command');

  step('negative: nonexistent executable rejected, config.yaml untouched');
  const badPatch = await fillCommandAndSave(page, 'nosuchcommand-xyz arg');
  assert(badPatch.status() === 400, `expected PATCH 400, got ${badPatch.status()}`);
  await page.waitForSelector('#admin-feedback.error', { timeout: TIMEOUT });
  const feedbackText = (await page.textContent('#admin-feedback')).trim();
  assert(feedbackText.includes('Claude'), `unexpected error feedback: ${feedbackText}`);
  const configAfterBad = readConfig();
  assert(configAfterBad === configAfterSave, `config.yaml changed after a rejected save:\n${configAfterBad}`);
  await page.screenshot({ path: path.join(artifactDir, '03-invalid-command-rejected.png') });
  console.log(`PATCH 400 → feedback "${feedbackText}"; config.yaml unchanged`);

  step('restart the server with the same data dir');
  await page.close();
  await stopServer();
  console.log(`server restarted: ${await startServer()}`);

  step('authorize a project through the admin form, then reload');
  const page2 = await openChatPage(context);
  await openAdminView(page2);
  await page2.fill('#project-name', 'ui-project');
  await page2.fill('#project-path', projectDir);
  const [addResponse] = await Promise.all([
    page2.waitForResponse((response) => response.url().endsWith('/admin/projects') && response.request().method() === 'POST', { timeout: TIMEOUT }),
    page2.click('#project-form button.primary')
  ]);
  assert(addResponse.status() === 201, `expected POST /admin/projects 201, got ${addResponse.status()}`);
  await page2.waitForSelector('#admin-feedback:not(.error):has-text("工作目录已添加")', { timeout: TIMEOUT });
  console.log('project authorized through the admin form (工作目录已添加)');
  // The project picker is populated on WebSocket hello, so the real path to
  // see the new project in the composer is a fresh page load.
  await page2.reload();
  await page2.waitForSelector('#connection-state:has-text("已连接")', { timeout: TIMEOUT });
  await page2.waitForFunction(() => !document.querySelector('#prompt').disabled, null, { timeout: TIMEOUT });
  await page2.waitForFunction(
    (dir) => [...document.querySelectorAll('#draft-project option')].some((option) => option.value === dir),
    projectDir,
    { timeout: TIMEOUT }
  );

  step('create a session through the real composer and drive one turn through the wrapper');
  // The wrapper CLI must be the selected provider before composing.
  await page2.waitForSelector('#provider-switcher button:has-text("Claude"):not([disabled])', { timeout: TIMEOUT });
  await page2.selectOption('#draft-project', projectDir);
  await page2.fill('#prompt', 'hi');
  await page2.screenshot({ path: path.join(artifactDir, '04-draft-ready.png') });
  await page2.click('#composer button.primary');
  // The first send creates the session and then delivers the same prompt.
  await page2.waitForFunction(
    () => [...document.querySelectorAll('#messages .message-content')].some((node) => node.textContent.includes('hi from wrapper')),
    null,
    { timeout: TIMEOUT }
  );
  const transcript = (await page2.locator('#messages .message-content').allTextContents()).join('\n---\n');
  assert(transcript.includes('hi from wrapper'), `assistant bubble missing the wrapper token:\n${transcript}`);
  await page2.screenshot({ path: path.join(artifactDir, '05-wrapper-turn.png') });
  console.log('assistant bubble contains the wrapper token "hi from wrapper"');
  await page2.close();

  assert(jsErrors.length === 0, `browser reported JS errors: ${jsErrors.join(' | ')}`);
  console.log('\n=== result ===');
  console.log('all assertions passed');
  console.log(`screenshots: ${artifactDir}`);
}

process.on('exit', () => {
  // Best-effort cleanup; the child is also killed in the finally block.
  if (server && server.exitCode === null) server.kill('SIGKILL');
});

main()
  .catch((error) => {
    exitCode = 1;
    console.error(`\nFAILED: ${error && error.stack ? error.stack : error}`);
  })
  .finally(async () => {
    await stopServer();
    if (browser) await browser.close().catch(() => {});
    process.exit(exitCode);
  });
