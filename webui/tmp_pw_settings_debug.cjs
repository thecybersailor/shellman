const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const endpoint = `https://openrouter.ai/api/v1/${Date.now()}`;
  const model = `openai/gpt-5-mini-${Date.now()}`;

  await page.goto('http://cli:4621');
  await page.getByRole('button', { name: 'Settings' }).first().click();
  await page.getByTestId('shellman-settings-helper-openai-endpoint').first().fill(endpoint);
  await page.getByTestId('shellman-settings-helper-openai-model').first().fill(model);
  await page.getByTestId('shellman-settings-save').first().click();
  await page.waitForTimeout(1000);

  await page.getByRole('button', { name: 'Settings' }).first().click();

  const report = await page.evaluate(() => {
    const nodes = Array.from(document.querySelectorAll('[data-test-id="shellman-settings-helper-openai-endpoint"]'));
    return nodes.map((el) => {
      const rect = el.getBoundingClientRect();
      const style = window.getComputedStyle(el);
      return {
        value: el.value,
        placeholder: el.getAttribute('placeholder'),
        display: style.display,
        visibility: style.visibility,
        opacity: style.opacity,
        width: rect.width,
        height: rect.height,
        offsetParent: !!el.offsetParent,
      };
    });
  });

  const config = await page.evaluate(async () => {
    const r = await fetch('/api/v1/config');
    const j = await r.json();
    return j?.data?.helper_openai ?? null;
  });

  console.log(JSON.stringify({ endpoint, model, report, config }, null, 2));
  await browser.close();
})();
