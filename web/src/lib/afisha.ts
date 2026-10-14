// Афиша «Куда пойти» на главной: web/src/content/afisha.json — отбор из
// сырых подборок агента-скаута (админка, страница /afisha, файлы
// exhibitions/*.json и lectures/*.json в корне проекта).
//
// Прошедшие события фильтруются на этапе сборки (сравнение дат-строк
// YYYY-MM-DD): страница никогда не показывает прошлое. У выставки без
// даты окончания ничего не вычислить — до следующего обновления отбора
// остаётся на сайте.

import afishaData from '../content/afisha.json';

export interface AfishaEvent {
  type: 'exhibition' | 'lecture';
  title: string;
  venue: string;
  address?: string;
  url?: string; // ссылка на первоисточник (мазея/площадка)
  dateStart?: string; // выставка: начало
  dateEnd?: string; // выставка: окончание
  date?: string; // лекция: день
  time?: string; // лекция: HH:MM
  description?: string;
  // Пометка «на главной» (вторая галочка в админке); у записей старого
  // отбора поле отсутствует — они показаны в блоке, как раньше.
  home?: boolean;
}

// Сегодня в зоне сборки, 'YYYY-MM-DD'.
function todayISO(): string {
  const d = new Date();
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function isActual(e: AfishaEvent): boolean {
  if (e.type === 'lecture') return (e.date ?? '') >= todayISO();
  // Выставка: прошла, если известен конец и он в прошлом. Без даты
  // окончания конец неизвестен — показываем до следующего обновления.
  return e.dateEnd ? e.dateEnd >= todayISO() : true;
}

export function loadAfisha(): AfishaEvent[] {
  return (afishaData as AfishaEvent[]).filter(isActual);
}

// Блок на главной: только события с пометкой «на главной» (у записей без
// пометки поле отсутствует — они были до деления, показываем как раньше).
// Прошедшее фильтруется в обоих случаях.
export function loadHomeAfisha(): AfishaEvent[] {
  return loadAfisha().filter((e) => e.home !== false);
}

// Даты карточки: выставка — «8 окт — 29 ноя», лекция — «24 октября, 19:00».
const shortFmt = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short' });
const longFmt = new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric',
  month: 'long',
});

export function eventDates(e: AfishaEvent): string {
  if (e.type === 'lecture') {
    const day = e.date ? longFmt.format(new Date(`${e.date}T00:00:00`)) : '';
    return e.time && day ? `${day}, ${e.time}` : day || (e.time ?? '');
  }
  const fmt = (iso?: string) =>
    iso ? shortFmt.format(new Date(`${iso}T00:00:00`)) : '';
  const a = fmt(e.dateStart);
  const b = fmt(e.dateEnd);
  if (a && b) return `${a} — ${b}`;
  return b ? `до ${b}` : a || '';
}

// Подпись типа события.
export function eventKind(e: AfishaEvent): string {
  return e.type === 'lecture' ? 'Лекция' : 'Выставка';
}

// Последняя известная дата события ('YYYY-MM-DD') — атрибут data-till
// в разметке: браузер посетителя прячет прошедшее сам, без пересборки.
export function eventTillISO(e: AfishaEvent): string {
  if (e.type === 'lecture') return e.date ?? '';
  return e.dateEnd ?? e.dateStart ?? '';
}

// Скрыть прошедшее в браузере: у карточек афиши с data-till, чья дата
// раньше сегоднешнего дня (местная дата посетителя), убрать из страницы.
// События без даты не трогаем — остаются до обновления отбора.
export function hidePastInBrowser(): void {
  const d = new Date();
  const pad = (n: number): string => String(n).padStart(2, '0');
  const today = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  document.querySelectorAll<HTMLElement>('.ev[data-till]').forEach((card) => {
    const till = card.getAttribute('data-till');
    if (till && till < today) card.remove();
  });
}