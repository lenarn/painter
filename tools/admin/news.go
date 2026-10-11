// news — лента: список, форма (создание/правка), сохранение.
//
// Форма одной страницы закрывает типовой сценарий «новость о новых
// картинах»: блоки «новая картина» (файл + поля + чекбоксы разделов)
// создают записи каталога и сразу привязывают работы к новости.
// Блоков может быть несколько («+ ещё картина»). Дедуп как у
// tools/catalogimport: sha256 файла, ключ artwork-<10 hex>.
//
// Стор читается с диска на каждый запрос (контент параллельно правит
// агент) и передаётся обработчикам параметром.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// imageExts — принимаем форматы, которые читает импортёр и сборка.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// artworkView — работа для выбора в форме (поиск по названию, миниатюра).
type artworkView struct {
	Slug  string
	File  string
	Title string
	Year  int
}

// naForm — значения одного блока «новая картина» из формы.
type naForm struct {
	Categories []string
	Title      string
	Year       string
	Technique  string
	W, H       string
	Available  string // "on" или ""
}

// naBlock — блок «новая картина» для рендера формы: после ошибки
// валидации поля заполнены из формы (файлы браузер не возвращает —
// их выбирают заново).
type naBlock struct {
	Index     int
	Num       int
	Checked   map[string]bool
	Title     string
	Year      string
	Technique string
	W, H      string
	Available bool
}

// handleNewsList — /news: лента (физический порядок файла — новые сверху).
func (a *admin) handleNewsList(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	a.render(w, "news-list", map[string]interface{}{
		"Page":  "news",
		"News":  st.News.News,
		"Saved": r.URL.Query().Get("saved"),
		"Note":  r.URL.Query().Get("note"),
	})
}

// handleNewsForm — /news/new (пустая форма) и /news/edit?i=N (правка).
func (a *admin) handleNewsForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}

	var item NewsItem
	idx := -1
	if r.URL.Path == "/news/edit" {
		i, err := strconv.Atoi(r.URL.Query().Get("i"))
		if err != nil || i < 0 || i >= len(st.News.News) {
			http.Error(w, "нет такой записи", http.StatusNotFound)
			return
		}
		idx = i
		item = st.News.News[i]
	}

	a.render(w, "news-form", map[string]interface{}{
		"Page":       "news",
		"Index":      idx,
		"Item":       item,
		"Text":       strings.Join(item.Text, "\n\n"),
		"Photos":     item.Photos,
		"Linked":     linkedViews(st, item.Artworks),
		"Artworks":   artworkViews(st),
		"Categories": st.Categories(),
		"NABlocks":   []naBlock{{Index: 0, Num: 1, Checked: map[string]bool{}}},
		"Errors":     nil,
	})
}

// artworkViews — весь каталог для выбора, отсортированный по названию.
func artworkViews(st *Store) []artworkView {
	views := make([]artworkView, 0, len(st.Catalog.Artworks))
	for _, aw := range st.Catalog.Artworks {
		views = append(views, artworkView{Slug: aw.Slug, File: aw.File, Title: aw.Title, Year: aw.Year})
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Title < views[j].Title })
	return views
}

// linkedViews — работы, привязанные к новости (блок миниатюр в правке).
func linkedViews(st *Store, slugs []string) []artworkView {
	var out []artworkView
	for _, slug := range slugs {
		if aw, ok := st.ArtworkBySlug(slug); ok {
			out = append(out, artworkView{Slug: aw.Slug, File: aw.File, Title: aw.Title, Year: aw.Year})
		}
	}
	return out
}

// handleNewsSave — POST multipart: текстовые поля, выбранные работы,
// файлы (новые фото новости, файлы новых картин).
func (a *admin) handleNewsSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}

	// При ошибке валидации форма перерисовывается с введёнными значениями.
	reRender := func(errs []string) {
		a.render(w, "news-form", map[string]interface{}{
			"Page":   "news",
			"Index":  formIndex(r),
			"Item": map[string]interface{}{
				"Date":     r.FormValue("date"),
				"Title":    strings.TrimSpace(r.FormValue("title")),
				"Artworks": r.Form["artworks"],
				"Photos":   []string{},
			},
			"Text":       r.FormValue("text"),
			"Photos":     []string{},
			"Linked":     linkedViews(st, r.Form["artworks"]),
			"Artworks":   artworkViews(st),
			"Categories": st.Categories(),
			"NABlocks":   naBlocks(r),
			"Errors":     errs,
		})
	}

	var errs []string

	// Дата и обязательные поля.
	date := strings.TrimSpace(r.FormValue("date"))
	if !dateRe.MatchString(date) {
		errs = append(errs, "Дата — в формате ГГГГ-ММ-ДД.")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		errs = append(errs, "Такой даты не существует.")
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		errs = append(errs, "Заголовок пуст.")
	}
	paragraphs := splitParagraphs(r.FormValue("text"))
	if len(paragraphs) == 0 {
		errs = append(errs, "Текст новости пуст.")
	}

	// Выбранные работы: сервер не доверяет форме — slug обязаны существовать.
	// Инициализация пустым срезом: nil замаршалился бы как null и сломал
	// сайт (news.ts ждёт массив).
	artworks := make([]string, 0, 4)
	for _, slug := range r.Form["artworks"] {
		if _, ok := st.ArtworkBySlug(slug); ok {
			artworks = append(artworks, slug)
		} else {
			errs = append(errs, "Работа «"+slug+"» не найдена в каталоге.")
		}
	}

	// Новые картины (блоки «новая картина»): создаются до сохранения
	// новости, чтобы сразу привязать их slug. Пустые блоки (без файла)
	// пропускаются. Дедуп — не ошибка: привязывается существующая работа
	// с предупреждением. Сообщения блока помечаются его номером.
	catalogChanged := false
	var warns []string
	for i := 0; i < naCount(r); i++ {
		pfx := strconv.Itoa(i)
		fh := firstFile(r, "na_file_"+pfx)
		if fh == nil {
			continue
		}
		f := naForm{
			Categories: r.Form["na_categories_"+pfx],
			Title:      r.FormValue("na_title_"+pfx),
			Year:       r.FormValue("na_year_"+pfx),
			Technique:  r.FormValue("na_technique_"+pfx),
			W:          r.FormValue("na_w_"+pfx),
			H:          r.FormValue("na_h_"+pfx),
			Available:  r.FormValue("na_available_"+pfx),
		}
		slug, naErrs, naWarns, created := a.addArtwork(st, fh, f)
		for _, e := range naErrs {
			errs = append(errs, "Картина "+strconv.Itoa(i+1)+": "+e)
		}
		for _, wn := range naWarns {
			warns = append(warns, "Картина "+strconv.Itoa(i+1)+": "+wn)
		}
		if slug != "" && !containsStr(artworks, slug) {
			artworks = append(artworks, slug)
		}
		if created {
			catalogChanged = true
		}
	}

	if len(errs) > 0 {
		reRender(errs)
		return
	}

	// Фото новости: файлы в web/src/assets/news/, имена — в запись.
	var photos []string
	for _, fh := range r.MultipartForm.File["photos"] {
		name, err := a.saveNewsPhoto(fh)
		if err != nil {
			errs = append(errs, "Фото «"+fh.Filename+"»: "+err.Error())
			continue
		}
		photos = append(photos, name)
	}
	if len(errs) > 0 {
		reRender(errs)
		return
	}

	item := NewsItem{Date: date, Title: title, Text: paragraphs, Artworks: artworks}

	if idx := formIndex(r); idx >= 0 {
		// Правка: старые фото сохраняются, новые дописываются.
		// Удаления в админке нет (осознанно) — через агента.
		if idx >= len(st.News.News) {
			reRender([]string{"Запись, которую правили, исчезла — открой форму заново."})
			return
		}
		old := st.News.News[idx]
		if len(old.Photos) > 0 || len(photos) > 0 {
			item.Photos = append(append([]string{}, old.Photos...), photos...)
		}
		st.News.News[idx] = item
	} else {
		if len(photos) > 0 {
			item.Photos = photos
		}
		st.News.News = append(st.News.News, item)
	}
	// Физический порядок файла — по дате убыванием (как сейчас),
	// стабильная сортировка не перемешивает равные даты.
	sort.SliceStable(st.News.News, func(i, j int) bool {
		return st.News.News[i].Date > st.News.News[j].Date
	})

	if err := st.SaveNews(); err != nil {
		reRender([]string{"Не удалось записать news.json: " + err.Error()})
		return
	}
	if catalogChanged {
		if err := st.SaveCatalog(); err != nil {
			reRender([]string{"news.json записан, но catalog.json нет: " + err.Error()})
			return
		}
	}

	// Сохранено; предупреждения (дедуп и т.п.) — в redirect-параметре.
	loc := "/news?saved=1"
	if len(warns) > 0 {
		loc += "&note=" + url.QueryEscape(strings.Join(warns, " "))
	}
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

// naCount — сколько блоков «новая картина» прислала форма.
// Нет поля/мусор — один блок (совместимость с прежней разметкой).
func naCount(r *http.Request) int {
	n, err := strconv.Atoi(r.FormValue("na_count"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// naBlocks — блоки «новая картина» для перерисовки формы после ошибки:
// текстовые поля и галочки восстанавливаются из формы, файлы — нет.
func naBlocks(r *http.Request) []naBlock {
	blocks := make([]naBlock, naCount(r))
	for i := range blocks {
		pfx := strconv.Itoa(i)
		blocks[i] = naBlock{
			Index:     i,
			Num:       i + 1,
			Checked:   checkedMap(r.Form["na_categories_"+pfx]),
			Title:     r.FormValue("na_title_"+pfx),
			Year:      r.FormValue("na_year_"+pfx),
			Technique: r.FormValue("na_technique_"+pfx),
			W:         r.FormValue("na_w_"+pfx),
			H:         r.FormValue("na_h_"+pfx),
			Available: r.FormValue("na_available_"+pfx) == "on",
		}
	}
	return blocks
}

// addArtwork — новая картина: sha256 файла, дедуп (файл уже в каталоге —
// дубль не создаём, привязываем существующую работу с предупреждением),
// копия в web/src/assets/artworks/artwork-<10 hex>.<ext>, запись каталога.
// Недозаполненное (нет техники/размеров/года) — черновик needs_metadata:true,
// как у импортёра. Сообщения без префикса — номер блока добавляет вызов.
// Возвращает (slug, ошибки, предупреждения, создана_ли).
func (a *admin) addArtwork(st *Store, fh *multipart.FileHeader, f naForm) (string, []string, []string, bool) {
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !imageExts[ext] {
		return "", []string{"файл не jpg/jpeg/png."}, nil, false
	}
	src, err := fh.Open()
	if err != nil {
		return "", []string{err.Error()}, nil, false
	}
	defer src.Close()
	data, err := ioutil.ReadAll(src)
	if err != nil {
		return "", []string{err.Error()}, nil, false
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	// Дедуп: такой файл уже в каталоге — привязываем существующую работу,
	// запись каталога не меняем.
	if old, ok := st.ArtworkByHash(hash); ok {
		title := old.Title
		if title == "" {
			title = old.Slug
		}
		return old.Slug, nil, []string{"файл уже есть в каталоге («" + title +
			"») — к новости привязана существующая работа, разделы не менялись."}, false
	}

	short := hash[:10]
	slug := "artwork-" + short
	fileName := slug + ext

	// Разделы: хотя бы один обязателен — работа без раздела невидима.
	var categories []string
	for _, c := range f.Categories {
		if c = strings.TrimSpace(c); c != "" {
			categories = append(categories, c)
		}
	}
	var errs []string
	if len(categories) == 0 {
		errs = append(errs, "отметь хотя бы один раздел галереи.")
	}

	// Поля. Название обязательно; пропущенные техника/размеры/год — черновик.
	title := strings.TrimSpace(f.Title)
	if title == "" {
		errs = append(errs, "название пустое.")
	}
	year, _ := strconv.Atoi(strings.TrimSpace(f.Year))
	technique := strings.TrimSpace(f.Technique)
	dims := parseDimensions(f.W, f.H)
	needs := technique == "" || dims == nil || year == 0

	if len(errs) > 0 {
		return "", errs, nil, false
	}

	// Файл в assets (коллизия имён невозможна: имя = хеш).
	if err := ensureDir(a.assetsDir("artworks")); err != nil {
		return "", []string{err.Error()}, nil, false
	}
	if err := ioutil.WriteFile(a.assetsDir("artworks", fileName), data, 0o644); err != nil {
		return "", []string{err.Error()}, nil, false
	}

	aw := Artwork{
		Slug:          slug,
		File:          fileName,
		Categories:    categories,
		NeedsMetadata: needs,
		Hash:          hash,
		Title:         title,
		Year:          year,
		Technique:     technique,
		Available:     f.Available == "on",
	}
	if dims != nil {
		aw.Dimensions = dims
	}
	// Каталог физически отсортирован по slug (как пишет импортёр).
	st.Catalog.Artworks = append(st.Catalog.Artworks, aw)
	sort.SliceStable(st.Catalog.Artworks, func(i, j int) bool {
		return st.Catalog.Artworks[i].Slug < st.Catalog.Artworks[j].Slug
	})
	st.reindex()
	return slug, nil, nil, true
}

// saveNewsPhoto — файл в web/src/assets/news/; имя исходное (чищеное),
// при коллизии — числовой суффикс. Возвращает имя для записи.
func (a *admin) saveNewsPhoto(fh *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !imageExts[ext] {
		return "", fmt.Errorf("не jpg/jpeg/png")
	}
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	data, err := ioutil.ReadAll(src)
	if err != nil {
		return "", err
	}
	if err := ensureDir(a.assetsDir("news")); err != nil {
		return "", err
	}
	base := sanitizeName(strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename)))
	if base == "" {
		base = "photo-" + time.Now().Format("20060102-150405")
	}
	name := base + ext
	for i := 1; ; i++ {
		if _, err := os.Stat(a.assetsDir("news", name)); os.IsNotExist(err) {
			break
		}
		name = base + "-" + strconv.Itoa(i) + ext
	}
	if err := ioutil.WriteFile(a.assetsDir("news", name), data, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// splitParagraphs — textarea → абзацы: пустая строка разделяет, одиночные
// переводы строк внутри абзаца сохраняются (стихи — как в news.json).
// Браузерный multipart нормализует переводы строк в \r\n (спека HTML
// FormData) — сначала приводим их к \n.
func splitParagraphs(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, p := range regexp.MustCompile(`\n[ \t]*\n+`).Split(s, -1) {
		p = strings.Trim(p, " \t\n")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseDimensions — "54" × "70" → {54, 70, "см"}; пустое — nil.
func parseDimensions(ws, hs string) *Dimensions {
	w, errW := strconv.ParseFloat(strings.Replace(ws, ",", ".", -1), 64)
	h, errH := strconv.ParseFloat(strings.Replace(hs, ",", ".", -1), 64)
	if errW != nil || errH != nil {
		return nil
	}
	return &Dimensions{W: w, H: h, Unit: "см"}
}

// formIndex — скрытое поле i: -1 — новая запись, иначе правка.
func formIndex(r *http.Request) int {
	i, err := strconv.Atoi(r.FormValue("i"))
	if err != nil || i < 0 {
		return -1
	}
	return i
}

func checkedMap(values []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// firstFile — первый загруженный файл поля (nil — поле пустое).
func firstFile(r *http.Request, field string) *multipart.FileHeader {
	if r.MultipartForm == nil {
		return nil
	}
	fhs := r.MultipartForm.File[field]
	if len(fhs) == 0 || fhs[0].Size == 0 {
		return nil
	}
	return fhs[0]
}

// sanitizeName — имя файла: буквы/цифры/дефисы/подчёркивания/точки,
// пробелы → дефис; кириллица сохраняется (news.ts читает по basename).
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r >= 0x400 && r <= 0x4FF: // кириллица
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}