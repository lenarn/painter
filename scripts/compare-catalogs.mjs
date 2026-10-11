// compare-catalogs.mjs — сравнение двух catalog.json по записям (slug).
// Печатает расхождения по полям; «каталоги идентичны» = мерж ничего
// не потерял. Запуск: node scripts/compare-catalogs.mjs <a.json> <b.json>

import { readFileSync } from 'node:fs';

const a = JSON.parse(readFileSync(process.argv[2], 'utf8')).artworks;
const b = JSON.parse(readFileSync(process.argv[3], 'utf8')).artworks;
const ma = new Map(a.map((x) => [x.slug, x]));
const mb = new Map(b.map((x) => [x.slug, x]));

let diffs = 0;
for (const s of new Set([...ma.keys(), ...mb.keys()])) {
  if (!ma.has(s) || !mb.has(s)) {
    console.log(`есть только в одном файле: ${s}`);
    diffs++;
    continue;
  }
  const x = ma.get(s);
  const y = mb.get(s);
  for (const k of new Set([...Object.keys(x), ...Object.keys(y)])) {
    if (JSON.stringify(x[k]) !== JSON.stringify(y[k])) {
      console.log(`${s}: ${k}: A=${JSON.stringify(x[k])} B=${JSON.stringify(y[k])}`);
      diffs++;
    }
  }
}
console.log(`записей: A=${a.length}, B=${b.length}`);
console.log(diffs ? `РАСХОЖДЕНИЙ: ${diffs}` : 'каталоги идентичны');