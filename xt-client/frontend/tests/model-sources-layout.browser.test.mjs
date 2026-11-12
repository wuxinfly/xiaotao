import assert from 'node:assert/strict';
import {test} from 'node:test';
import {mkdirSync} from 'node:fs';
import {sourceBrowser, fillSource} from './source-browser-fixture.mjs';

test('model interfaces share page gutters and keep the empty state compact', {timeout:60000}, () => sourceBrowser(5205, async ({page, errors, navigate}) => {
  mkdirSync('test-results', {recursive:true});
  await page.setViewportSize({width:1280,height:800});
  await navigate('accounts');
  await page.getByRole('heading', {name:'添加第一个模型接口'}).waitFor();
  const measure = () => page.locator('.model-sources-page').evaluate(element => {
    const rect = node => {const r=node.getBoundingClientRect(); return {x:r.x,y:r.y,right:r.right,bottom:r.bottom,width:r.width,height:r.height};};
    const style=getComputedStyle(element);
    return {page:rect(element),header:rect(element.querySelector('.source-page-header')),empty:rect(element.querySelector('.source-empty')),gateway:rect(element.querySelector('.source-local-gateway')),plugins:rect(element.querySelector('.plugin-section')),gutter:parseFloat(style.getPropertyValue('--content-gutter')),max:parseFloat(style.getPropertyValue('--content-max')),padding:parseFloat(style.paddingLeft),overflow:document.querySelector('main').scrollWidth>document.querySelector('main').clientWidth};
  });
  let layout=await measure();
  assert.equal(layout.padding,layout.gutter);
  assert.ok(layout.header.x>=layout.gutter, 'title must not touch the window edge');
  assert.ok(Math.abs(layout.header.x-layout.page.x-layout.gutter)<1, 'header uses shared content gutter');
  assert.ok(layout.empty.height<300, 'empty state must not stretch with the viewport');
  assert.ok(Math.abs(layout.plugins.y-layout.empty.bottom-24)<1, 'empty source list must not create an extra grid gap');
  assert.ok(Math.abs(layout.gateway.y-layout.plugins.bottom-24)<1, 'gateway follows the plugin section with one shared gap');
  assert.ok(layout.gateway.bottom<800, 'local gateway should fit on the initial desktop screen');
  assert.equal(layout.overflow,false);
  const gatewayHeader=await page.locator('.source-local-gateway .endpoint-banner-header').boundingBox();
  assert.ok(gatewayHeader.width>600, 'gateway description must use a full row instead of a squeezed sidebar');
  await page.screenshot({path:'test-results/model-sources-layout-desktop.png'});
  await page.setViewportSize({width:1920,height:1080});
  layout=await measure();
  assert.ok(layout.page.width<=layout.max+1, 'wide screens use shared maximum content width');
  assert.ok(layout.empty.height<300, 'large screens must not inflate empty-state rows');
  for (const width of [540,375]) {
    await page.setViewportSize({width,height:812});
    layout=await measure();
    assert.equal(layout.padding,layout.gutter);
    assert.equal(layout.overflow,false);
    assert.ok(layout.header.x>=layout.gutter);
    assert.ok(layout.gateway.right<=width);
  }
  await page.screenshot({path:'test-results/model-sources-layout-mobile.png'});
  await fillSource(page,'Layout check');
  const input=page.getByLabel('API URL',{exact:true});
  const inputWidth=await input.evaluate(node=>node.getBoundingClientRect().width);
  const parentWidth=await input.evaluate(node=>node.parentElement.getBoundingClientRect().width);
  assert.ok(Math.abs(inputWidth-parentWidth)<1, 'global input max-width must not shrink source form fields');
  const checkbox=await page.getByRole('checkbox',{name:'支持图片'}).boundingBox();
  assert.ok(checkbox.width<=18, 'checkbox must not inherit the global text-input width');
  await page.keyboard.press('Escape');
  assert.deepEqual(errors,[]);
}));
