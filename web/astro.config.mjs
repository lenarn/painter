// @ts-check
import { defineConfig } from 'astro/config';

// Конфигурация Astro для сайт-визитки художника.
// Статическая генерация; islands подключаются через client:directives.
// sharp (идёт в комплекте) включён на production для WebP/AVIF/srcset.
export default defineConfig({
  // Домен сайта: включает canonical, og:image, og:url, sitemap.xml
  // и Sitemap-строку в robots.txt (см. web/src/lib/seo.ts).
  site: 'https://eromanova.ru',
  output: 'static',
  devToolbar: { enabled: false },
});