// Каталог картин: парсер имени файла → подпись, загрузка catalog.json,
// разрешение изображений из web/src/assets/artworks/.
//
// Грамматика имени файла: <Год>_<Название>_<Техника>.<ext>
// Подпись: «Название, Год, Техника». Размер — отдельное поле каталога.

import type { ImageMetadata } from 'astro';
import catalogData from '../content/catalog.json';

export interface Dimensions {
  w?: number;
  h?: number;
  unit?: string;
}

export interface Artwork {
  slug: string;
  file: string;
  title?: string;
  year?: number;
  technique?: string;
  dimensions?: Dimensions;
  categories: string[];
  caption?: string;
  alt?: string;
  featured?: boolean;
  // «В наличии» — работа готова к продаже (пометка в галерее и на странице).
  available?: boolean;
  needs_metadata: boolean;
  hash?: string;
  // Ручной порядок внутри категории: меньше число — раньше.
  // Ключ — имя категории; работы без записи идут по дефолтной сортировке.
  sort?: Record<string, number>;
  // Ручной порядок в баннере на главной (featured-работы).
  featuredOrder?: number;
  // Связанные работы серии: в разделе галереи стоят рядом.
  related?: string[];
  // Ручной порядок внутри серии (related-кластера): меньше число — раньше.
  seriesOrder?: number;
}

interface Catalog {
  artworks: Artwork[];
}

// import.meta.glob: eager-импорт изображений из artworks/.
// Ключ — путь, значение — модуль с default export = ImageMetadata.
const imageModules = import.meta.glob<ImageMetadata>(
  '../assets/artworks/*.{jpg,jpeg,png,JPG,JPEG,PNG}',
  { eager: true },
);

// Карта: basename файла → { метаданные, путь исходника }.
const imageByBasename = new Map<string, { mod: ImageMetadata; srcPath: string }>();
for (const [path, mod] of Object.entries(imageModules)) {
  const base = path.split('/').pop()!;
  imageByBasename.set(base, {
    mod: mod.default,
    srcPath: new URL(path, import.meta.url).href,
  });
}

export function loadCatalog(): Artwork[] {
  return (catalogData as Catalog).artworks;
}

// Метаданные для показа. Astro оборачивает eager-импорт в Proxy: чтение
// любого свойства помечает оригинал «referenced» — и его копия остаётся
// в dist (все сканы, ~150 МБ). Чтение `.clone` пометку не ставит: оно
// возвращает чистый объект {src, width, height, format}. fsPath — маркер
// исходника: по нему Astro после генерации вариантов удаляет копию
// оригинала из dist; подставляем сами, т.к. в клоне его нет.
export function resolveImage(art: Artwork): ImageMetadata | undefined {
  const entry = imageByBasename.get(art.file);
  if (!entry) return undefined;
  // .clone — свойство-объект (structuredClone), чтение пометку не ставит.
  const clone = (entry.mod as ImageMetadata & { clone?: ImageMetadata }).clone;
  // Нет clone (изменилась внутренняя механика Astro) — деградируем
  // к прокси: оригиналы останутся в dist, но ничего не сломается.
  if (!clone) return entry.mod;
  return Object.defineProperty(clone, 'fsPath', { value: entry.srcPath }) as ImageMetadata;
}

// Парсер имени файла → {title, year, technique, caption}.
// Возвращает null, если имя не соответствует грамматике.
export function parseFilename(file: string): {
  title: string;
  year: number;
  technique: string;
  caption: string;
} | null {
  const dot = file.lastIndexOf('.');
  const stem = dot >= 0 ? file.slice(0, dot) : file;
  const parts = stem.split('_').map((s) => s.trim());
  if (parts.length !== 3) return null;
  const [title, yearStr, technique] = parts;
  if (!/^\d{4}$/.test(yearStr)) return null;
  if (!title || !technique) return null;
  const year = parseInt(yearStr, 10);
  return { title, year, technique, caption: `${title}, ${year}, ${technique}` };
}

// Подпись для отображения. Если метаданные не заполнены — заглушка.
export function captionFor(art: Artwork): string {
  if (art.needs_metadata || !art.title) return 'Без названия';
  const fromFile = parseFilename(art.file);
  if (fromFile) return fromFile.caption;
  return art.caption ?? `${art.title}, ${art.year ?? ''}, ${art.technique ?? ''}`.trim();
}

// Краткая подпись для карточки (только название).
export function titleFor(art: Artwork): string {
  if (art.needs_metadata || !art.title) return 'Без названия';
  const fromFile = parseFilename(art.file);
  return fromFile?.title ?? art.title;
}

export function dimensionsLabel(art: Artwork): string {
  const d = art.dimensions;
  if (!d || !d.w || !d.h) return '';
  const unit = d.unit ?? 'см';
  return `${d.w}×${d.h} ${unit}`;
}

// Полная мета-строка: «Название, Год, Техника, Размер».
export function metaLine(art: Artwork): string {
  const parts: string[] = [captionFor(art)];
  const dim = dimensionsLabel(art);
  if (dim) parts.push(dim);
  return parts.join(' · ');
}

// slugify с транслитерацией кириллицы.
export function slugify(text: string): string {
  const map: Record<string, string> = {
    а: 'a', б: 'b', в: 'v', г: 'g', д: 'd', е: 'e', ё: 'e', ж: 'zh',
    з: 'z', и: 'i', й: 'i', к: 'k', л: 'l', м: 'm', н: 'n', о: 'o',
    п: 'p', р: 'r', с: 's', т: 't', у: 'u', ф: 'f', х: 'h', ц: 'ts',
    ч: 'ch', ш: 'sh', щ: 'sch', ъ: '', ы: 'y', ь: '', э: 'e', ю: 'yu',
    я: 'ya',
  };
  return text
    .toLowerCase()
    .split('')
    .map((ch) => map[ch] ?? ch)
    .join('')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

// Категория может быть путём «раздел/подраздел» (вложенность из папок).
const CATEGORY_SEP = '/';

// Ссылка на страницу категории: каждый сегмент кодируется отдельно,
// чтобы '/' между ними остался разделителем пути.
export function categoryHref(path: string): string {
  return '/gallery/' + path.split(CATEGORY_SEP).map(encodeURIComponent).join('/');
}

// Верхний уровень галереи: первый сегмент пути категории → число работ
// (работа считается один раз в разделе, сколько бы подразделов ни касалось).
export function categoriesWithCounts(arts: Artwork[]): { name: string; count: number }[] {
  const counts = new Map<string, Set<string>>();
  for (const a of arts) {
    const seen = new Set<string>();
    for (const c of a.categories) {
      const top = c.split(CATEGORY_SEP)[0];
      if (!seen.has(top)) {
        seen.add(top);
        if (!counts.has(top)) counts.set(top, new Set());
        counts.get(top)!.add(a.slug);
      }
    }
  }
  return [...counts.entries()]
    .map(([name, set]) => ({ name, count: set.size }))
    .sort((a, b) => a.name.localeCompare(b.name, 'ru'));
}

// Подразделы категории (следующий сегмент путей, начинающихся с path).
export function subcategoriesWithCounts(
  arts: Artwork[],
  path: string,
): { name: string; path: string; count: number }[] {
  const counts = new Map<string, Set<string>>();
  for (const a of arts) {
    for (const c of a.categories) {
      if (!c.startsWith(path + CATEGORY_SEP)) continue;
      const rest = c.slice(path.length + CATEGORY_SEP.length);
      const name = rest.split(CATEGORY_SEP)[0];
      const subPath = path + CATEGORY_SEP + name;
      if (!counts.has(subPath)) counts.set(subPath, new Set());
      counts.get(subPath)!.add(a.slug);
    }
  }
  return [...counts.entries()]
    .map(([p, set]) => ({ name: p.split(CATEGORY_SEP).pop()!, path: p, count: set.size }))
    .sort((a, b) => a.name.localeCompare(b.name, 'ru'));
}

// Порядок работ в категории:
// 1) ручной sort[category] (меньше — раньше);
// 2) дефолт: год по убыванию (новое сверху), работы без года — в конец;
// 3) по названию по алфавиту, 4) по hash — стабильность между сборками.
// Связанные работы (related) одной серии стоят рядом: кластер серии
// занимает позицию по самому позднему году своих работ,
// внутри кластера — тот же порядок (год по убыванию и т.д.).
export function artworksByCategory(arts: Artwork[], category: string): Artwork[] {
  const items = arts.filter((a) => a.categories.includes(category));

  // Union-find по related-ссылкам (ссылка действует, только если обе
  // работы есть в этом разделе).
  const inCategory = new Set(items.map((a) => a.slug));
  const parent = new Map<string, string>(items.map((a) => [a.slug, a.slug]));
  const find = (slug: string): string => {
    const p = parent.get(slug)!;
    if (p === slug) return slug;
    const root = find(p);
    parent.set(slug, root);
    return root;
  };
  for (const a of items) {
    for (const slug of a.related ?? []) {
      if (!inCategory.has(slug)) continue;
      const ra = find(a.slug);
      const rb = find(slug);
      if (ra !== rb) parent.set(ra, rb);
    }
  }

  // Ключ кластера: самый поздний год серии + минимальное название —
  // стабильное сравнение кластеров между сборками.
  const clusterYear = new Map<string, number>();
  const clusterTitle = new Map<string, string>();
  for (const a of items) {
    const root = find(a.slug);
    clusterYear.set(root, Math.max(clusterYear.get(root) ?? 0, a.year ?? 0));
    const t = clusterTitle.get(root);
    const at = titleFor(a);
    if (t === undefined || at.localeCompare(t, 'ru') < 0) clusterTitle.set(root, at);
  }

  return [...items].sort((a, b) => {
    const sa = a.sort?.[category] ?? Number.POSITIVE_INFINITY;
    const sb = b.sort?.[category] ?? Number.POSITIVE_INFINITY;
    if (sa !== sb) return sa - sb;
    const ra = find(a.slug);
    const rb = find(b.slug);
    if (ra !== rb) {
      const ya = clusterYear.get(ra)!;
      const yb = clusterYear.get(rb)!;
      if (ya !== yb) return yb - ya;
      const byTitle = clusterTitle.get(ra)!.localeCompare(clusterTitle.get(rb)!, 'ru');
      if (byTitle !== 0) return byTitle;
      return ra.localeCompare(rb);
    }
    // Внутри серии: ручной seriesOrder (меньше — раньше), затем как обычно.
    const soa = a.seriesOrder ?? Number.POSITIVE_INFINITY;
    const sob = b.seriesOrder ?? Number.POSITIVE_INFINITY;
    if (soa !== sob) return soa - sob;
    const ya = a.year ?? 0;
    const yb = b.year ?? 0;
    if (ya !== yb) return yb - ya;
    const byTitle = titleFor(a).localeCompare(titleFor(b), 'ru');
    if (byTitle !== 0) return byTitle;
    return (a.hash ?? a.slug).localeCompare(b.hash ?? b.slug);
  });
}

// Сквозной раздел «В наличии»: все работы с флагом available из любых
// тематических разделов. Порядок общий для страницы раздела и листания
// с неё (год по убыванию, затем название и hash — стабильность между
// сборками).
export function availableArtworks(arts: Artwork[]): Artwork[] {
  return arts
    .filter((a) => a.available)
    .sort(
      (a, b) =>
        (b.year ?? 0) - (a.year ?? 0) ||
        titleFor(a).localeCompare(titleFor(b), 'ru') ||
        (a.hash ?? a.slug).localeCompare(b.hash ?? b.slug),
    );
}

export function featuredArtworks(arts: Artwork[]): Artwork[] {
  const f = arts.filter((a) => a.featured);
  const list = f.length > 0 ? f : arts.slice(0, 6);
  // Порядок баннера: ручной featuredOrder, затем год по убыванию.
  return list.sort((a, b) => {
    const fa = a.featuredOrder ?? Number.POSITIVE_INFINITY;
    const fb = b.featuredOrder ?? Number.POSITIVE_INFINITY;
    if (fa !== fb) return fa - fb;
    return (b.year ?? 0) - (a.year ?? 0);
  });
}