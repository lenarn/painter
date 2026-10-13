// Загрузка контентных разделов: «Об авторе», «Образование», «Выставки».
// Источник — JSON-файлы в src/content/. Пополняются без кода: новая запись
// в соответствующем JSON. Позже правится через админку (Фаза 3).
import about from '../content/about.json';
import education from '../content/education.json';
import exhibitions from '../content/exhibitions.json';

export interface Membership {
  title: string;
  year: number;
}

export interface AboutData {
  name: string;
  surname: string;
  intro: string[];
  memberships?: Membership[];
}

export interface EducationEntry {
  period: string;
  title: string;
  place?: string;
  sortStart: string; // «YYYY-MM» — для сортировки от новых к старым
  highlight?: boolean; // выделить жирным (диплом, ДПО, повышение квалификации)
}

export interface ExhibitionEntry {
  year: number;
  sortStart: string; // «YYYY-MM-DD» — для порядка внутри года
  period: string; // отображаемые даты
  title: string;
  place?: string;
  type?: string; // «персональная» | «групповая» (опционально)
  note?: string;
  artworks?: string[]; // slug работ из catalog.json (миниатюры под записью)
}

export const aboutData = about as AboutData;

export const educationData: EducationEntry[] = (education as EducationEntry[])
  .slice()
  .sort((a, b) => (b.sortStart < a.sortStart ? -1 : b.sortStart > a.sortStart ? 1 : 0));

// Выставки — по году обратно, внутри года — по дате начала обратно.
export const exhibitionsData: ExhibitionEntry[] = (exhibitions as ExhibitionEntry[])
  .slice()
  .sort((a, b) => {
    if (b.year !== a.year) return b.year - a.year;
    return b.sortStart < a.sortStart ? -1 : b.sortStart > a.sortStart ? 1 : 0;
  });