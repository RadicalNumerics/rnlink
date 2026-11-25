import fs from 'node:fs';
import assert from 'node:assert/strict';
const rules = JSON.parse(fs.readFileSync(new URL('../dist/rules.json', import.meta.url)));
const target = 'https://rn.example.ts.net/';
function redirect(url, type = 'main_frame') {
  for (const r of rules) {
    if (!r.condition.resourceTypes.includes(type)) continue;
    const match = new RegExp(r.condition.regexFilter).exec(url);
    if (match) return r.action.redirect.url ?? r.action.redirect.regexSubstitution.replace(/\\(\d)/g, (_, n) => match[Number(n)] ?? '');
  }
  return null;
}
let count = 0;
for (const name of ['', 'test', 'gpus', 'onboarding', 'gpu-console', 'gpu_console', 'v1.0']) {
  for (const slash of ['/', '%2F', '%2f']) {
    for (const root of ['https://www.google.com/search?q=', 'https://google.com/search?client=safari&q=']) {
      assert.equal(redirect(root + 'rn' + slash + name + '&sourceid=chrome&ie=UTF-8'), target + name);
      count++;
    }
  }
}
for (const url of [
  'https://www.google.com/search?q=weather',
  'https://www.google.com/search?q=what+is+rn%2Ftest',
  'https://www.google.com/search?q=rn%2Ftest+help',
  'https://www.google.com/search?q=rn%2Ftest%20help',
  'https://www.google.com/search?q=rn%2Fhttps%3A%2F%2Fevil.com',
  'https://www.google.com/search?q=rn%2F%2Fevil.com',
  'https://www.google.com/search?q=rn%2F..',
  'https://www.google.com/search?q=rn%2F-test',
  'https://www.google.com/search?q=rn%2Ftest%26extra',
  'https://www.google.com/search?q=rn%252Ftest',
  'https://www.google.com/search?q=RN%2Ftest',
  'https://www.google.com.evil.com/search?q=rn%2Ftest',
  'https://evil.com/search?q=rn%2Ftest',
  'https://www.google.com/other?q=rn%2Ftest',
  target + 'test'
]) { assert.equal(redirect(url), null, url); count++; }
assert.equal(redirect('https://www.google.com/search?q=rn%2Ftest', 'xmlhttprequest'), null); count++;
console.log(`${count} routing cases passed. Browser loading and address-bar behavior still require manual verification.`);
