// Run with Playwright's browser_run_code_unsafe filename option after rebuilding
// gf-sandbox. Uses only the supplied Page; no npm install or app JavaScript.
// Sign in and open an unused pool patient first. This check shares that patient
// and retrieves its source data. Never use it against a non-demo environment.
async (page) => {
  const recordURL = page.url().replace(/\/(share|retrieve)$/, '');
  if (!/^http:\/\/localhost:8091\/demo\/ehr\/patients\/(anna|pool-\d+)$/.test(recordURL)) {
    throw new Error('Start on an opened demo patient record at localhost:8091.');
  }
  const results = [];
  const widths = [768, 1024, 1280, 1440, 1920, 390];
  const measure = () => {
    const root = document.documentElement;
    const problems = [];
    if (root.scrollWidth > innerWidth) problems.push('Document scrolls horizontally');
    for (const element of document.querySelectorAll('body *')) {
      if (!(element instanceof HTMLElement) || element.closest('svg,[inert]') ||
          !element.getClientRects().length || ['SCRIPT', 'STYLE'].includes(element.tagName)) continue;
      const parent = element.parentElement;
      // The fixed dock and its edge tab intentionally extend outside the app.
      // Inline parents can span several line boxes; compare their block instead.
      if (!parent || ['gf-tab', 'hood-dock'].includes(element.id) ||
          getComputedStyle(parent).display === 'inline') continue;
      const box = element.getBoundingClientRect();
      const bounds = parent.getBoundingClientRect();
      if (box.left < bounds.left - 1 || box.right > bounds.right + 1) {
        problems.push(`${element.tagName}.${element.className} exceeds ${parent.tagName}.${parent.className}`);
      }
      if (element.clientWidth && element.scrollWidth > element.clientWidth + 1 &&
          !['auto', 'scroll'].includes(getComputedStyle(element).overflowX)) {
        problems.push(`${element.tagName}.${element.className} clips or overflows its contents`);
      }
    }
    const top = document.querySelector('.top').getBoundingClientRect();
    for (const element of document.querySelectorAll('.top h2, .dezi-pill, .top-right > form')) {
      const box = element.getBoundingClientRect();
      if (box.top < top.top || box.bottom > top.bottom) problems.push('Top bar child spills vertically');
    }
    const blocks = [...document.querySelector('main').children];
    for (let index = 1; index < blocks.length; index++) {
      if (blocks[index].getBoundingClientRect().top - blocks[index - 1].getBoundingClientRect().bottom < 19) {
        problems.push('Page blocks lost their vertical rhythm');
      }
    }
    return { documentWidth: root.scrollWidth, topHeight: top.height, problems };
  };
  const sweep = async (screen) => {
    for (const width of widths) {
      await page.setViewportSize({ width, height: 1000 });
      await page.evaluate(() => document.fonts.ready);
      for (const open of [false, true]) {
        if (await page.locator('#gf-tab').getAttribute('aria-expanded') !== String(open)) {
          await page.locator('#gf-tab').click();
        }
        await page.waitForFunction((open) => {
          const dock = document.querySelector('#hood-dock');
          return document.querySelector('#hood-content').inert === !open &&
            dock.getAnimations().every(animation => animation.playState === 'finished');
        }, open);
        if (!open) {
          const focusEntered = await page.locator('.hd-close').evaluate(element => {
            element.focus();
            return document.activeElement === element;
          });
          if (focusEntered) throw new Error('Collapsed viewer accepts focus');
        }
        const measured = await page.evaluate(measure);
        results.push({ screen, width, open, ...measured });
      }
    }
  };
  await page.goto('http://localhost:8091/demo/ehr');
  await sweep('patients');
  await page.goto(recordURL);
  await sweep('record');
  await page.goto(`${recordURL}/share`);
  await sweep('share');
  await page.locator('#gf-tab').click(); // Sweep ends with the viewer open.
  await page.locator('main form[action$="/share"] button').click();
  await page.locator('.confirm-grid').waitFor();
  await sweep('share-result');
  await page.goto(`${recordURL}/retrieve`);
  await sweep('retrieve');
  await page.locator('#gf-tab').click();
  await page.locator('main form[action$="/retrieve"] button').first().click();
  await page.locator('.auth-hero').waitFor();
  if (await page.locator('.auth-hero.denied').count()) {
    throw new Error('Retrieval was denied; enriched-record coverage requires an allowed demo patient.');
  }
  await sweep('authorization');
  await page.goto(recordURL);
  if (!await page.locator('.data-item .src.zb').count()) {
    throw new Error('The record has no retrieved external-source items.');
  }
  await sweep('enriched-record');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.locator('#gf-tab').focus();
  await page.locator('#gf-tab').press('Enter');
  await page.waitForFunction(() => document.querySelector('#gf-tab').getAttribute('aria-expanded') === 'false');
  await page.locator('#gf-tab').press('Enter');
  await page.waitForFunction(() => document.querySelector('#gf-tab').getAttribute('aria-expanded') === 'true');
  const failures = results.filter(result => result.problems.length);
  if (failures.length) throw new Error(JSON.stringify(failures, null, 2));
  return { checked: results.length, screens: [...new Set(results.map(result => result.screen))], widths, failures };
}
