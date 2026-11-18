// Install @alicloud/esa20240910 and its dependencies in the operator's tooling,
// or supply NODE_PATH. Credentials are read from the existing private ossutil file.
const fs = require('node:fs');
const path = require('node:path');
const sdk = require('@alicloud/esa20240910');
const { $OpenApiUtil } = require('@alicloud/openapi-core');
const tea = require('@alicloud/tea-util');
const args = process.argv.slice(2);
const option = (name, fallback) => args.includes(name) ? args[args.indexOf(name) + 1] : fallback;
const config = JSON.parse(fs.readFileSync(option('--config', path.join(__dirname, '../deploy/aliyun-mirror.json')), 'utf8'));
const backup = option('--backup');
if (!backup) throw new Error('--backup is required');
const ini = fs.readFileSync(option('--credentials', '/root/.ossutilconfig'), 'utf8');
const value = key => ini.match(new RegExp('^\\s*' + key + '\\s*=\\s*(.+)$', 'im'))?.[1].trim() || '';
const Client = sdk.default?.default || sdk.default;
const client = new Client(new $OpenApiUtil.Config({ accessKeyId: value('accessKeyID'), accessKeySecret: value('accessKeySecret'), regionId: 'cn-hangzhou', connectTimeout: 5000, readTimeout: 15000 }));
const runtime = new tea.RuntimeOptions({ autoretry: false, connectTimeout: 5000, readTimeout: 15000 });

async function main() {
  const sites = await client.listSitesWithOptions(new sdk.ListSitesRequest({ siteName: config.esa.siteName, siteSearchType: 'exact', pageSize: 10, pageNumber: 1 }), runtime);
  const siteId = sites.body.sites.find(site => site.siteName === config.esa.siteName)?.siteId;
  if (!siteId) throw new Error('ESA site unavailable');
  const list = async () => (await client.listCacheRulesWithOptions(new sdk.ListCacheRulesRequest({ siteId, pageNumber: 1, pageSize: 100 }), runtime)).body.configs;
  const before = await list();
  fs.writeFileSync(backup, JSON.stringify(before, null, 2), { mode: 0o600 });
  for (const rule of config.esa.cacheRules) console.log(rule.ruleName, 'TTL', rule.edgeCacheTtl, 'priority', rule.sequence);
  if (!args.includes('--apply')) return;
  if (JSON.stringify(await list()) !== JSON.stringify(before)) throw new Error('ESA rules changed concurrently');
  for (const rule of config.esa.cacheRules) {
    const existing = before.find(item => item.ruleName === rule.ruleName);
    if (existing) {
      const changed = Object.entries(rule).some(([key, wanted]) => String(existing[key]) !== String(wanted));
      if (changed) await client.updateCacheRuleWithOptions(new sdk.UpdateCacheRuleRequest({ siteId, configId: existing.configId, ...rule }), runtime);
    } else {
      await client.createCacheRuleWithOptions(new sdk.CreateCacheRuleRequest({ siteId, ...rule }), runtime);
    }
  }
  const after = await list();
  fs.writeFileSync(backup + '.after.json', JSON.stringify(after, null, 2), { mode: 0o600 });
  for (const rule of config.esa.cacheRules) {
    const actual = after.find(item => item.ruleName === rule.ruleName);
    if (!actual || Object.entries(rule).some(([key, wanted]) => String(actual[key]) !== String(wanted))) throw new Error('ESA rule verification failed: ' + rule.ruleName);
  }
  for (const prior of before.filter(item => !config.esa.cacheRules.some(rule => rule.ruleName === item.ruleName))) {
    const actual = after.find(item => item.configId === prior.configId);
    if (JSON.stringify(actual) !== JSON.stringify(prior)) throw new Error('Unrelated ESA rule changed');
  }
  console.log('ESA public distribution rules verified');
}
main().catch(error => { console.error({ code: error.code || error.name, message: error.message }); process.exitCode = 1; });
