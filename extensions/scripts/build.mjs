import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

// Public forks must not contain a company's private tailnet hostname.
const input = process.argv[2];
if (!input) throw new Error('Usage: node extensions/scripts/build.mjs https://rn.YOUR-TAILNET.ts.net/');
const url = new URL(input);
if (url.protocol !== 'https:' || url.username || url.password || url.pathname !== '/' || url.search || url.hash) {
  throw new Error('Destination must be an HTTPS origin without credentials, path, query, or fragment.');
}
const target = url.href;
const out = new URL('../dist/', import.meta.url);
fs.mkdirSync(out, { recursive: true });
const rules = [];
// WebKit rejects alternation (|). Separate literal/encoded slashes into rules.
for (const slash of ['%2[fF]', '/']) {
  const prefix = '^https://(www\\.)?google\\.com/search\\?([^#]*&)?q=rn' + slash;
  const suffix = '(&[^#]*)?(#.*)?$';
  rules.push({
    id: rules.length + 1, priority: 1,
    action: { type: 'redirect', redirect: { regexSubstitution: target + '\\3' } },
    condition: { regexFilter: prefix + '([A-Za-z0-9_][A-Za-z0-9_.-]*)' + suffix, isUrlFilterCaseSensitive: true, resourceTypes: ['main_frame'] }
  });
  rules.push({
    id: rules.length + 1, priority: 1,
    action: { type: 'redirect', redirect: { url: target } },
    condition: { regexFilter: prefix + suffix, isUrlFilterCaseSensitive: true, resourceTypes: ['main_frame'] }
  });
}
const manifest = {
  manifest_version: 3, name: 'RN Links', version: '0.1.1',
  description: 'Open rn/link shortcuts with your organization’s golink service.',
  permissions: ['declarativeNetRequestWithHostAccess'],
  host_permissions: ['https://www.google.com/*', 'https://google.com/*', target + '*'],
  declarative_net_request: { rule_resources: [{ id: 'rn_links', enabled: true, path: 'rules.json' }] },
  action: { default_title: 'RN Links diagnostics', default_popup: 'diagnostics.html' }
};
for (const [name, data] of Object.entries({ 'manifest.json': manifest, 'rules.json': rules, 'config.json': { baseUrl: target } })) {
  fs.writeFileSync(new URL(name, out), JSON.stringify(data, null, 2) + '\n');
}
const escapeHTML = value => value.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
for (const name of ['diagnostics.html', 'diagnostics.js']) {
  const source = fs.readFileSync(new URL('../src/' + name, import.meta.url), 'utf8');
  fs.writeFileSync(new URL(name, out), source.replaceAll('__BASE_URL__', escapeHTML(target)));
}
console.log('Built extension: ' + fileURLToPath(out));
