(async () => {
 const api = globalThis.browser || globalThis.chrome;
 const lines = [];
 const config = await (await fetch('config.json')).json();
 const check = async (label, fn) => {
   try { lines.push(label + ': ' + JSON.stringify(await fn())); }
   catch (e) { lines.push(label + ': ERROR ' + e.message); }
   document.querySelector('#status').textContent = lines.join('\n\n');
 };
 await check('Enabled rulesets', () => api.declarativeNetRequest.getEnabledRulesets());
 for (const origin of ['https://www.google.com/*','https://google.com/*',config.baseUrl + '*']) {
   await check('Permission ' + origin, () => api.permissions.contains({origins:[origin]}));
 }
 const rules = await (await fetch('rules.json')).json();
 for (const rule of rules) {
   await check('Rule ' + rule.id, () => api.declarativeNetRequest.isRegexSupported({regex:rule.condition.regexFilter,isCaseSensitive:true,requireCapturing:!!rule.action.redirect.regexSubstitution}));
 }
})().catch(e => { document.querySelector('#status').textContent += '\n' + e.message; });
