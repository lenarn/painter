---
name: painter
description: Работа с проектом painter (сайт-визитка художника: Astro 5 + Go-инструменты) — структура репозитория, запуск dev-сервера/админки/превью, импорт картин, правки контента и кода, обновление README (EN + RU), проверки ошибок (go vet, npm run build), подготовка публикации (zip для хостинга, push на GitHub)
---

# Painter — операционный справочник

Статика Astro 5 (`web/`) + Go-инструменты (`tools/`, только stdlib).
Личный контент — оригиналы картин, контентные JSON, папки сканов — в git
не входит (`.gitignore`). Это осознанное решение, не недоработка: не
предлагать «починить», а работать с контрактами ниже.

## Структура

```
web/                      сайт (Astro 5, статическая генерация)
  src/pages/              страницы и маршруты (gallery/[...category], artworks/[slug], news/[...page])
  src/components/         ArtworkCard, GalleryGrid, Header, Footer, Spoiler…
  src/layouts/            Layout.astro (шрифты @fontsource, самохостинг)
  src/lib/                catalog.ts (импорт картин через resolveImage), news.ts, seo.ts, content.ts
  src/content/            контент JSON — ЛИЧНОЕ, в git нет; catalog.json генерирует импортёр
  src/content/template/   заглушки контента для нового пользователя — в git есть
  src/assets/             artworks/ (сканы), about/ (портрет), news/ — ЛИЧНОЕ, в git нет
  public/                 favicon.svg, robots.txt, .htaccess (кэширование)
tools/catalogimport/      импортёр: sha256-дедуп → assets + catalog.json, мерж метаданных по hash
tools/admin/              локальная админка: Go html/template, порт 8150, правит content JSON,
                          спеллер, сборка, zip, превью
scripts/                  start-admin.ps1, publish-pack.ps1, publish-preview.ps1,
                          make-contact-list.mjs, compare-catalogs.mjs
reports/                  планы и handoff — ЛИЧНОЕ, в git нет
publish/                  архивы деплоя — в git нет
```

## Запуск

| Что | Команда |
|---|---|
| Dev-сервер | `cd web; npm run dev` → http://localhost:4321 |
| Сборка | `cd web; npm run build` → `web/dist/` |
| Админка | `powershell -ExecutionPolicy Bypass -File scripts\start-admin.ps1` → http://localhost:8150, остановка Ctrl+C; превью сайта гасится отдельно кнопкой «Остановить превью» на странице «Сборка» |
| Превью публикации | `powershell -ExecutionPolicy Bypass -File scripts\publish-preview.ps1` |
| Импорт картин | `go run ./tools/catalogimport "папка1" "папка2" …` из корня |

Импортёр: идемпотентен (ключ — sha256), один файл в нескольких папках =
одна запись с несколькими категориями, вложенная папка = составная
категория. Импортёр **пересоздаёт каталог целиком из перечисленных папок**:
прогон с неполным списком снесёт записи остальных. Ручные метаданные
известных работ (по hash) переносятся, перед записью — бэкап `.bak`.

## Контентные контракты

- `catalog.json`: `artworks[]` — `slug`, `file`, `categories`, `needs_metadata`,
  ручные поля: `title`, `year`, `technique`, `dimensions{w,h,unit}` (w —
  ширина; дробные размеры допустимы), `caption`, `alt`, `featured`,
  `available` («в наличии»), `sort{категория:позиция}`, `related` (slug'и,
  ссылки ЗАПИСЫВАЮТСЯ ВЗАИМНО), `seriesOrder`, `featuredOrder`, `hash`.
- `news.json`: `news[]` — `date` (YYYY-MM-DD), `title`, `text[]` (абзацы,
  `\n` внутри абзаца = pre-line), `artworks[]` (всегда массив, null ломает
  news.ts), `photos[]` (файлы из `web/src/assets/news/`).
- `about/education/exhibitions/contacts.json` — схемы смотреть в
  `web/src/content/template/` (заглушки) или в реальных файлах.
- Заглушки в `template/` обязаны отставать от схем не более чем на одну
  правку: изменил поле в коде сайта — проверь, что заглушка валидна.
- Админка и агент пишут одни и те же файлы: стор перечитывается с диска на
  каждый запрос, запись атомарная (temp + rename) с `.bak`.

## Процесс правок

1. План → подтверждение владельца → изменение. Никаких правок «по дороге».
2. После правок Go: `go vet ./tools/...`; после правок сайта:
   `cd web; npm run build` — эталон чистой сборки ~150 страниц.
3. Клиентскую логику (формы, лайтбоксы, JS в шаблонах админки) проверять
   headless Edge по CDP, не curl'ом; при очистке процессов Edge фильтр
   `edge-admin-test` в командной строке — браузер владельца не трогать.
4. Изображения не открывать в чате инструментом Read — результаты проверок
   текстом/JSON.
5. PowerShell 5.1: нет `&&`; ps1-скрипты с кириллицей — только UTF-8 с BOM.
6. curl на https с этой машины — `--ssl-no-revoke`; кириллические URL на
   сервер — процент-кодировать.
7. Порт 4321 занят после упавшего dev: убить слушателя
   `Get-NetTCPConnection -LocalPort 4321 -State Listen`.

## README

Два файла, менять **синхронно**: `README.md` (английский, главный,
рендерится на странице репозитория) и `README.ru.md` (перекрёстные ссылки
вверху обоих). Секции: описание + живой пример, Stack, Features, Repository
layout, Requirements, шаги 1–7 (clone → scans → import → template → dev →
metadata → build), Admin panel, Deploy as an archive, Images and the
repository, License. Обновлять при: новых инструментах/скриптах, изменении
контрактов JSON, изменении команд запуска, изменении состава .gitignore.

## Проверка ошибок

- `go vet ./tools/...` — статический анализ Go.
- `cd web; npm run build` — сборка падает на несогласованных контрактах
  (битые import, невалидный JSON).
- Спеллер для черновиков текстов — Яндекс Спеллер (владелец знает, что
  черновик уходит на его API).
- Если что-то странное с CSS в dev — кэш Vite:
  `Remove-Item -Recurse -Force web\node_modules\.vite`, версту проверять
  по собранному `npm run preview`, не по dev.

## Публикация

Два независимых пути:

1. **Сайт на хостинг** (shared, статика): `scripts\publish-pack.ps1` →
   `publish/site-<дата>.zip` (dist в корне архива) + txt-манифест + отчёт
   diff с прошлым архивом (новые/изменённые/удалённые; удалённые > 0 —
   владельцу чистить корень перед распаковкой). Заливает владелец через
   панель хостинга; агент на сервер не ходит.
2. **GitHub** (`lenarn/painter`): изменения → `git status` глазами
   просмотреть (личное защищено .gitignore, но проверять всё равно) →
   план → подтверждение → commit → push. Identity репозитория:
   `lenarn <lenarn@users.noreply.github.com>`. Никаких force-push без
   прямого распоряжения.