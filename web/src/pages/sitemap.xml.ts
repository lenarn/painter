// sitemap.xml — статический endpoint, без пакета @astrojs/sitemap.
// Список URL собирается из каталога и новостей; домен — из astro.config
// (`site`). Пока в config заглушка example.com, файл валиден, но пуст
// (см. siteReady в lib/seo.ts) — впиши домен, и он заполнится сам.
import type { APIRoute } from 'astro';
import {
  loadCatalog,
  categoriesWithCounts,
  subcategoriesWithCounts,
  categoryHref,
} from '../lib/catalog';
import { sortedNews, NEWS_PAGE_SIZE } from '../lib/news';
import { siteReady } from '../lib/seo';

export const GET: APIRoute = ({ site }) => {
  const urls: string[] = [];
  if (siteReady && site) {
    const base = site.toString().replace(/\/$/, '');
    const push = (path: string) => urls.push(base + path);

    push('/');
    push('/about');
    push('/afisha');
    push('/contacts');
    push('/exhibitions');
    push('/gallery');
    push('/gallery/v-nalichii');

    // Новости: страницы пагинации /news, /news/2…
    const lastPage = Math.max(1, Math.ceil(sortedNews().length / NEWS_PAGE_SIZE));
    for (let p = 1; p <= lastPage; p++) push(p === 1 ? '/news' : `/news/${p}`);

    // Разделы и подразделы галереи.
    const arts = loadCatalog();
    for (const c of categoriesWithCounts(arts)) {
      push(categoryHref(c.name));
      for (const s of subcategoriesWithCounts(arts, c.name)) push(categoryHref(s.path));
    }

    // Страницы работ.
    for (const a of arts) push(`/artworks/${a.slug}`);
  }

  const xml =
    '<?xml version="1.0" encoding="UTF-8"?>\n' +
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
    urls.map((u) => `  <url><loc>${u}</loc></url>`).join('\n') +
    (urls.length > 0 ? '\n' : '') +
    '</urlset>\n';

  return new Response(xml, { headers: { 'Content-Type': 'application/xml; charset=utf-8' } });
};