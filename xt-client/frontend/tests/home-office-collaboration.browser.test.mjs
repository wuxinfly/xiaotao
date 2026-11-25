import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {test} from 'node:test';
import {chromium} from 'playwright';

const now = '2026-11-21T00:00:00+08:00';
const project = {path: 'E:/office-fixture', name: '协作项目', saved: true};
const employee = {id: 'role_0123456789abcdef', name: '设计师', role: 'project', status: 'idle', todayTasks: 0};
const session = (id, title) => ({
  id, title, project, model: 'test/model', status: 'completed', createdAt: now, updatedAt: now,
  messages: [], activity: [], thoughts: [], runStats: [],
});
const main = session('main-office', '办公室协作验收');
const employeeSession = session('employee-scoped', '设计师');
employeeSession.messages.push({id: 'employee-question', role: 'assistant', content: '移动端也一起改吗？', createdAt: now});
const scope = '/api/office/conversations/main-office';
const chat = '/api/office/members/' + employee.id + '/chat';

test('home keeps the coordinator active and reopens the same employee turn', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5247', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw new Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true, args: ['--enable-unsafe-swiftshader']});
    const page = await browser.newPage({viewport: {width: 1440, height: 900}, reducedMotion: 'reduce'});
    await page.addInitScript(() => {
      const fetch = window.fetch.bind(window);
      window.fetch = (url, init) => {
        if (!window.testEmployeeStream || !String(url).includes('/members/') || !String(url).includes('/events?')) return fetch(url, init);
        const body = new ReadableStream({
          start(controller) {
            const encoder = new TextEncoder();
            let closed = false;
            window.pushEmployeeEvent = event => controller.enqueue(encoder.encode('data: ' + JSON.stringify(event) + '\n\n'));
            window.endEmployeeStream = () => {
              if (!closed) {
                closed = true;
                window.pushEmployeeEvent = undefined;
                controller.close();
              }
            };
            init?.signal?.addEventListener('abort', window.endEmployeeStream, {once: true});
          },
        });
        return Promise.resolve(new Response(body, {headers: {'Content-Type': 'text/event-stream'}}));
      };
    });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let conversation;
    let dispatched;
    let delegated;
    let directReply;
    let continued;
    let retries = 0;
    let streamDrafts;
    let closes = 0;
    let historyReads = 0;
    await page.route('**/api/**', route => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [main]}});
      if (path === '/api/agent/sessions/main-office/history') return route.fulfill({json: {session: main, before: ''}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test/model', label: '测试模型', supportsImages: true}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (path === '/api/agent/references/sessions') return route.fulfill({json: {sessions: []}});
      if (path === '/api/agent/projects') return route.fulfill({json: {projects: [project]}});
      if (path === '/api/office/snapshot') return route.fulfill({json: {project, accessMode: 'safe',
        tasks: [], members: [employee], artifacts: [], capabilities: {notes: []}}});
      if (path === '/api/office/team') return route.fulfill({json: {members: [employee], globalRoles: [],
        projectRoles: [], onboarding: 'completed', globalRevision: 0, projectRevision: 0, teamRevision: 0}});
      if (path === scope) return route.fulfill({json: conversation ? [conversation] : []});
      if (path === scope + '/dispatch') {
        dispatched = request.postDataJSON();
        const sourceMessageId = 'forwarded-user';
        main.messages.push({id: sourceMessageId, role: 'user', content: dispatched.content, createdAt: now});
        conversation = {memberId: employee.id, memberName: employee.name, sessionId: employeeSession.id, status: 'running',
          sourceMessageId, request: dispatched.content};
        return route.fulfill({json: conversation});
      }
      if (path === scope + '/reply') {
        const input = request.postDataJSON();
        if (input.delegate) {
          delegated = input;
          conversation = {...conversation, coordinatorQuestion: '你希望设计师覆盖哪些端？'};
        } else {
          directReply = input;
          conversation = {...conversation, question: '', coordinatorQuestion: '', status: 'completed', reply: '设计师会只处理桌面端。'};
        }
        return route.fulfill({json: conversation});
      }
      if (path === scope + '/close') {
        closes++;
        conversation = undefined;
        return route.fulfill({json: {ok: true}});
      }
      if (path === chat + '/messages') {
        continued = request.postDataJSON();
        conversation = {...conversation, status: 'running'};
        return route.fulfill({json: conversation});
      }
      if (path === scope + '/retry-summary') {
        assert.equal(request.postDataJSON().memberId, employee.id);
        retries++;
        conversation = {...conversation, summaryStatus: 'saved'};
        return route.fulfill({json: conversation});
      }
      if (path === chat + '/history') {
        assert.equal(url.searchParams.get('parentSessionId'), main.id);
        historyReads++;
        return route.fulfill({json: {session: employeeSession, before: '', streamDrafts}});
      }
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5247');
    await page.getByRole('button', {name: main.title, exact: true}).click();
    await page.locator('.agent-context-title strong').getByText(main.title, {exact: true}).waitFor();
    const editor = page.getByRole('textbox', {name: '给小涛发消息'});
    await editor.click();
    await page.keyboard.insertText('@设');
    await page.getByRole('option', {name: /设计师/}).click();
    await editor.press('End');
    await page.keyboard.insertText('请修一下登录按钮');
    await editor.press('Enter');
    const card = page.locator('.agent-office-card').filter({hasText: employee.name});
    await card.getByText('正在处理', {exact: true}).waitFor();
    assert.equal(dispatched.memberId, employee.id);
    assert.equal(await page.getByRole('complementary', {name: '办公室工作区'}).count(), 0);
    await editor.focus();
    conversation = {...conversation, question: employeeSession.messages[0].content, status: 'waiting'};
    await card.getByText('等你回复', {exact: true}).waitFor();
    const notice = page.locator('.toast-container--bottom-right');
    await notice.getByText('设计师等你回复').waitFor();
    assert.equal(await editor.evaluate(element => element === document.activeElement), true);
    await page.getByRole('button', {name: '员工动态，1 项待处理', exact: true}).waitFor();
    await card.getByRole('button', {name: '让小涛回复'}).click();
    await card.getByText('你希望设计师覆盖哪些端？').waitFor();
    assert.equal(delegated.memberId, employee.id);
    await card.getByRole('textbox', {name: '回复小涛的问题'}).fill('只改桌面端');
    await card.getByRole('button', {name: '回复', exact: true}).click();
    await card.getByText('设计师会只处理桌面端。').waitFor();
    assert.equal(directReply.content, '只改桌面端');
    await card.getByRole('button', {name: /设计师/}).click();
    const pane = page.getByRole('complementary', {name: '办公室工作区'});
    await pane.getByText(employeeSession.messages[0].content, {exact: true}).waitFor();
    const avatar = pane.locator('.agent-message-avatar .office-member-avatar');
    assert.equal(await avatar.textContent(), '设');
    assert.equal(await pane.locator('.agent-message-avatar .agent-droplet').count(), 0);
    const picker = pane.getByRole('button', {name: '员工模型'});
    const sendButton = pane.getByRole('button', {name: '发送消息', exact: true});
    assert.equal(await picker.evaluate(element => element.classList.contains('agent-model')), true);
    await picker.click();
    await page.getByRole('menuitemradio', {name: /测试模型/}).click();
    assert.ok(Math.abs((await picker.boundingBox()).y - (await sendButton.boundingBox()).y) < 8);
    await page.setViewportSize({width: 390, height: 760});
    assert.ok(Math.abs((await picker.boundingBox()).y - (await sendButton.boundingBox()).y) < 8);
    await page.setViewportSize({width: 1440, height: 900});
    conversation = {...conversation, summaryStatus: 'failed'};
    await pane.getByText('长期记忆未保存', {exact: true}).waitFor();
    await pane.getByRole('textbox', {name: '给设计师发消息'}).fill('继续讨论');
    const sent = page.waitForResponse(response => new URL(response.url()).pathname === chat + '/messages');
    await sendButton.click();
    assert.equal((await sent).status(), 200);
    assert.equal(continued.content, '继续讨论');
    await pane.locator('.agent-working-row').getByText('正在思考').waitFor();
    assert.equal(await pane.locator('.agent-working-row .office-member-avatar').textContent(), '设');
    employeeSession.runStats = [{runId: 'live-employee', startedAt: now, phase: 'working', filesRead: 0, filesWritten: 0, toolCalls: 0}];
    streamDrafts = [{id: 'stream-employee', runId: 'live-employee', role: 'assistant', content: '正在逐步回答', createdAt: now}];
    await pane.locator('.agent-run-timeline').getByText('正在逐步回答').waitFor();
    assert.equal(await pane.locator('.agent-working-row').count(), 0);
    streamDrafts = undefined;
    employeeSession.runStats = [];
    conversation = {...conversation, status: 'completed'};
    await pane.getByRole('button', {name: '重试保存', exact: true}).click();
    await pane.getByText('长期记忆未保存', {exact: true}).waitFor({state: 'hidden'});
    assert.equal(retries, 1);
    await pane.getByRole('button', {name: '收起设计师对话'}).click();
    assert.equal(closes, 0);
    await page.evaluate(() => {window.testEmployeeStream = true;});
    await page.getByRole('button', {name: '员工动态', exact: true}).click();
    const reopened = page.waitForResponse(response => new URL(response.url()).pathname === chat + '/history');
    await page.getByRole('dialog', {name: '员工动态'}).getByRole('button', {name: /设计师/}).click();
    assert.equal((await reopened).status(), 200);
    await pane.getByText(employeeSession.messages[0].content, {exact: true}).waitFor();
    assert.ok(historyReads >= 2);
    await page.waitForFunction(() => !!window.pushEmployeeEvent);
    const employeeEditor = pane.getByRole('textbox', {name: '给设计师发消息'});
    await employeeEditor.evaluate(element => {
      const canvas = document.createElement('canvas');
      canvas.width = canvas.height = 2;
      const bytes = Uint8Array.from(atob(canvas.toDataURL().split(',')[1]), char => char.charCodeAt(0));
      const clipboard = new DataTransfer();
      clipboard.items.add(new File([bytes], '截图.png', {type: 'image/png'}));
      element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: clipboard, bubbles: true, cancelable: true}));
    });
    await pane.getByRole('img', {name: '截图.png'}).waitFor();
    const liveSent = page.waitForResponse(response => new URL(response.url()).pathname === chat + '/messages');
    await sendButton.click();
    await liveSent;
    assert.equal(continued.content, '');
    assert.equal(continued.images.length, 1);
    assert.equal(continued.images[0].mimeType, 'image/png');
    await pane.getByRole('img', {name: '截图.png'}).waitFor({state: 'hidden'});
    const liveStats = {runId: 'streamed-turn', startedAt: now, phase: 'working', filesRead: 0, filesWritten: 0, toolCalls: 0};
    const user = {id: 'streamed-user', role: 'user', content: '', createdAt: now, runId: liveStats.runId,
      images: [{id: 'attached', url: 'data:image/png;base64,' + continued.images[0].data, width: 2, height: 2}]};
    const assistant = {id: 'streamed-answer', role: 'assistant', content: '第一段🌊', createdAt: now, runId: liveStats.runId};
    const push = event => page.evaluate(value => window.pushEmployeeEvent(value), {...event, sessionId: employeeSession.id});
    await push({type: 'session', status: 'running', message: user});
    await pane.getByRole('img', {name: '消息图片'}).waitFor();
    await push({type: 'run_stats', runStats: liveStats});
    await push({type: 'message_delta', message: assistant, streamOffset: 0});
    const liveBubble = pane.locator('[data-run-id="streamed-turn"] .agent-run-bubble');
    await liveBubble.getByText('第一段🌊', {exact: true}).waitFor();
    employeeSession.runStats = [liveStats];
    streamDrafts = [assistant];
    await page.evaluate(() => window.endEmployeeStream());
    await page.waitForFunction(() => !!window.pushEmployeeEvent);
    await push({type: 'message_delta', message: assistant, streamOffset: 0});
    await push({type: 'message_delta', message: {...assistant, content: '第二段'}, streamOffset: 5});
    await liveBubble.getByText('第一段🌊第二段', {exact: true}).waitFor();
    const final = {...assistant, content: '第一段🌊第二段。'};
    const endStats = {...liveStats, phase: 'completed', endedAt: now};
    employeeSession.messages.push(user, final);
    employeeSession.runStats = [endStats];
    streamDrafts = undefined;
    conversation = {...conversation, status: 'completed'};
    await push({type: 'answer_end', message: final});
    await push({type: 'run_stats', runStats: endStats});
    await push({type: 'status', status: 'completed'});
    await liveBubble.getByText(final.content, {exact: true}).waitFor();
    await page.getByRole('button', {name: '员工动态', exact: true}).click();
    await page.getByRole('button', {name: '关闭主会话'}).click();
    await page.getByRole('button', {name: '员工动态', exact: true}).waitFor({state: 'hidden'});
    assert.equal(closes, 1);
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
