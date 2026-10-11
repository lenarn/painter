// make-contact-list — генератор контакт-листа: локальный HTML-отчёт
// всех работ каталога без метаданных (needs_metadata: true).
// Елена открывает reports/catalog-contacts.html, смотрит превью
// и диктует подписи по номерам («№12 — „Осень“, 2024, холст/масло, 40×50»).
//
// Запуск из корня проекта:  node scripts/make-contact-list.mjs
// Выход: reports/catalog-contacts.html + reports/catalog-contacts.json
// (мапа «номер → slug», чтобы вносить подписи без угадывания).

import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const catalog = JSON.parse(
  readFileSync(join(root, 'web/src/content/catalog.json'), 'utf8'),
);
const outDir = join(root, 'reports');

// Работы без подписей, по верхней категории (первый сегмент categories).
const pending = catalog.artworks.filter((a) => a.needs_metadata);
const groups = new Map();
for (const art of pending) {
  const top = (art.categories[0] ?? 'без категории').split('/')[0];
  if (!groups.has(top)) groups.set(top, []);
  groups.get(top).push(art);
}

// Сквозная нумерация: группы по алфавиту (ru), внутри — по slug.
const names = [...groups.keys()].sort((a, b) => a.localeCompare(b, 'ru'));
const byNumber = [];
let n = 0;
const cards = names
  .map((cat) => {
    const items = groups
      .get(cat)
      .slice()
      .sort((a, b) => a.slug.localeCompare(b.slug))
      .map((art) => {
        n += 1;
        byNumber.push({ number: n, slug: art.slug });
        // Другие категории работы — пометка под номером.
        const others = art.categories
          .map((c) => c.split('/')[0])
          .filter((c) => c !== cat);
        const extra = others.length
          ? `<span class="also">также: ${others.join(', ')}</span>`
          : '';
        return `
          <figure class="card">
            <img src="../web/src/assets/artworks/${art.file}" alt="" loading="lazy">
            <figcaption>
              <span class="num">№ ${n}</span>
              <span class="slug">${art.slug}</span>
              ${extra}
            </figcaption>
          </figure>`;
      })
      .join('');
    return `<section class="group"><h2>${cat} <span class="count">${groups.get(cat).length}</span></h2><div class="grid">${items}</div></section>`;
  })
  .join('');

const html = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Контакт-лист: работы без подписей (${pending.length})</title>
<style>
  body { font-family: Georgia, serif; margin: 0; background: #f6f4ef; color: #222; }
  header { padding: 24px 32px; background: #fff; border-bottom: 1px solid #ddd; }
  h1 { font-size: 1.3rem; margin: 0 0 4px; font-weight: 500; }
  .sub { color: #888; font-size: 0.85rem; font-family: Arial, sans-serif; }
  .group { padding: 24px 32px 8px; }
  h2 { font-size: 1.1rem; font-weight: 500; margin: 0 0 4px; }
  .count { color: #999; font-size: 0.85rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 18px; padding: 12px 0; }
  .card { margin: 0; background: #fff; border: 1px solid #ddd; border-radius: 6px; padding: 8px; }
  .card img { width: 100%; height: 200px; object-fit: contain; background: #eee; display: block; }
  figcaption { padding-top: 8px; font-family: Arial, sans-serif; font-size: 0.8rem; display: flex; flex-wrap: wrap; gap: 8px; align-items: baseline; }
  .num { font-weight: bold; }
  .slug { color: #999; font-size: 0.7rem; }
  .also { color: #b60; width: 100%; }
</style>
</head>
<body>
<header>
  <h1>Работы без подписей — ${pending.length} шт.</h1>
  <div class="sub">Диктуй по номерам: «№ … — „Название“, год, техника, размер» (размер — ширина×высота, см; можно добавлять «в наличии»).</div>
</header>
${cards}
</body>
</html>
`;

writeFileSync(join(outDir, 'catalog-contacts.html'), html);
writeFileSync(
  join(outDir, 'catalog-contacts.json'),
  JSON.stringify({ generated: new Date().toISOString(), byNumber }, null, 2),
);
console.log(`контакт-лист: ${pending.length} работ → reports/catalog-contacts.html`);