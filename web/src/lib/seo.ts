// SEO-хелперы: canonical, Open Graph, JSON-LD (schema.org).
//
// ДОМЕН берётся из astro.config.mjs (поле `site`). Пока там заглушка
// example.com, все absolute-поля (canonical, og:image, og:url,
// sitemap) НЕ выводятся — после подключения домена впиши его в config,
// и они появятся при следующей сборке сами, без правок здесь.

import { getImage } from 'astro:assets';
import type { ImageMetadata } from 'astro';
import { aboutData } from './content';

export const fullName = `${aboutData.name} ${aboutData.surname}`.trim();

// import.meta.env.SITE — значение `site` из astro.config.mjs.
const SITE = import.meta.env.SITE ?? '';
export const siteReady = (() => {
  try {
    return new URL(SITE).hostname !== 'example.com';
  } catch {
    return false;
  }
})();

// Абсолютный URL (для canonical/OG/sitemap); undefined, пока домен-заглушка.
export function absUrl(path: string): string | undefined {
  if (!siteReady) return undefined;
  return new URL(path, SITE).toString();
}

// og:image: JPEG до 1200 px по ширине. Соцсети рендерят превью максимум
// ~1200 px; полноразмерный скан в og:image не нужен (тяжёлые og:image
// Telegram/ВК часто не показывают). Заодно оригиналы больше не
// упоминаются в сборке — и не попадают в dist.
// ВАЖНО: getImage асинхронный (Astro 5) — вызовы только через await,
// иначе .src молча даёт undefined (og:image пропадал со всех страниц).
export async function ogImageSrc(img: ImageMetadata | undefined): Promise<string | undefined> {
  if (!img) return undefined;
  return (await getImage({ src: img, width: Math.min(1200, img.width), format: 'jpeg' })).src;
}

// Публичные страницы художника (для sameAs в Person — каналы и группы).
const socialPages = ['https://t.me/eromanova_art', 'https://vk.com/eromanova_art'];

// schema.org: автор сайта (выводится на всех страницах).
export function personSchema(url?: string) {
  return {
    '@context': 'https://schema.org',
    '@type': 'Person',
    name: fullName,
    jobTitle: 'Художник',
    knowsAbout: ['живопись', 'графика'],
    url,
    sameAs: socialPages,
  };
}

// schema.org: сам сайт (выводится на всех страницах).
export function websiteSchema(url?: string) {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    name: `${fullName} — художник`,
    url,
  };
}