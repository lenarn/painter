// Лента новостей: web/src/content/news.json.
//
// Запись: {date: 'YYYY-MM-DD', title, text: [абзацы],
//          artworks: [slug работ], photos: [имена файлов]}.
// Работы — по slug из catalog.json; порядок показа — как в списке.
// Произвольные фото (вернисаж, вид зала) — имена файлов из
// web/src/assets/news/ (инбокс для Елены — папка news/ в корне проекта).

import type { ImageMetadata } from 'astro';
import type { Artwork } from './catalog';
import newsData from '../content/news.json';

export interface NewsItem {
  date: string;
  title: string;
  text: string[];
  artworks: string[];
  photos?: string[];
}

interface News {
  news: NewsItem[];
}

// import.meta.glob: фото новостей из web/src/assets/news/.
const photoModules = import.meta.glob<ImageMetadata>(
  '../assets/news/*.{jpg,jpeg,png,JPG,JPEG,PNG}',
  { eager: true },
);

// Карта: basename файла → ImageMetadata.
const photoByBasename = new Map<string, ImageMetadata>();
for (const [path, mod] of Object.entries(photoModules)) {
  const base = path.split('/').pop()!;
  photoByBasename.set(base, mod.default);
}

// Фото новости по имени файла. Отсутствующий файл — undefined
// (рендер пропускает его молча).
export function resolveNewsPhoto(name: string): ImageMetadata | undefined {
  return photoByBasename.get(name);
}

export function loadNews(): NewsItem[] {
  return (newsData as News).news;
}

// Вся лента по убыванию даты (новые сверху).
export function sortedNews(): NewsItem[] {
  return loadNews().slice().sort((a, b) => b.date.localeCompare(a.date));
}

// Последняя (самая свежая) запись с изображениями:
// хотя бы одна работа или хотя бы одно фото.
// null — если таких новостей нет.
export function latestNewsWithImages(): NewsItem | null {
  return (
    sortedNews().find(
      (n) => n.artworks.length > 0 || (n.photos ?? []).length > 0,
    ) ?? null
  );
}

// Работы новости по slug, порядок — как в списке записи.
// Несуществующие slug молча пропускаются.
export function newsArtworks(arts: Artwork[], item: NewsItem): Artwork[] {
  const bySlug = new Map(arts.map((a) => [a.slug, a]));
  return item.artworks
    .map((slug) => bySlug.get(slug))
    .filter((a): a is Artwork => Boolean(a));
}

// Новостей на странице ленты (/news).
export const NEWS_PAGE_SIZE = 10;

// Страница ленты, на которой лежит запись с этой датой:
// 1-я — /news, дальше — /news/2, /news/3…
export function newsPageForDate(date: string): number {
  const idx = sortedNews().findIndex((n) => n.date === date);
  return idx === -1 ? 1 : Math.floor(idx / NEWS_PAGE_SIZE) + 1;
}

// Ссылка на запись: страница + якорь /news#news-<дата>.
export function newsAnchorHref(date: string): string {
  const page = newsPageForDate(date);
  const base = page === 1 ? '/news' : `/news/${page}`;
  return `${base}#news-${date}`;
}

// «22 сентября 2026» из 'YYYY-MM-DD'.
const dateFmt = new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric',
  month: 'long',
  year: 'numeric',
});

export function formatDate(date: string): string {
  return dateFmt.format(new Date(`${date}T00:00:00`));
}