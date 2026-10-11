// artworks — страница «Картины»: список каталога, новая картина без
// новости, правка существующих (поля, разделы, related-серии) и
// замена файла.
//
// Замена файла = новая запись (hash/slug/имя derive от содержимого
// файла) с переносом всех ссылок: related у соседей серии, artworks
// в новостях. Старая запись и старый файл удаляются — это осознанное
// удаление внутри явной операции (случайных удалений в админке нет).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// artworkRow — строка списка: работа + счётчик related для бейджа «серия».
type artworkRow struct {
	Artwork
	RelatedN int
}

// handleArtworksList — /artworks: весь каталог, сортировка по названию.
func (a *admin) handleArtworksList(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	rows := make([]artworkRow, 0, len(st.Catalog.Artworks))
	for _, aw := range st.Catalog.Artworks {
		rows = append(rows, artworkRow{Artwork: aw, RelatedN: len(aw.Related)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	a.render(w, "artworks-list", map[string]interface{}{
		"Page":  "artworks",
		"Items": rows,
		"Saved": r.URL.Query().Get("saved"),
		"Note":  r.URL.Query().Get("note"),
	})
}

// handleArtworkForm — /artworks/new (пустая) и /artworks/edit?slug= (правка).
func (a *admin) handleArtworkForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	var item Artwork
	if slug := r.URL.Query().Get("slug"); slug != "" {
		aw, ok := st.ArtworkBySlug(slug)
		if !ok {
			http.Error(w, "нет такой работы", http.StatusNotFound)
			return
		}
		item = *aw
	}
	a.renderArtworkForm(w, st, item, dimStr(item, 'w'), dimStr(item, 'h'), nil)
}

// renderArtworkForm — общие данные шаблона формы. Техника пустая —
// подставляется «холст, масло» (решение Елены 2026-10-09).
func (a *admin) renderArtworkForm(w http.ResponseWriter, st *Store, item Artwork, wStr, hStr string, errs []string) {
	technique := item.Technique
	if technique == "" {
		technique = "холст, масло"
	}
	a.render(w, "artwork-form", map[string]interface{}{
		"Page":            "artworks",
		"IsEdit":          item.Slug != "",
		"Item":            item,
		"Technique":       technique,
		"W":               wStr,
		"H":               hStr,
		"Artworks":        artworkViews(st),
		"Categories":      st.Categories(),
		"CheckedCats":     checkedMap(item.Categories),
		"CheckedRelated":  checkedMap(item.Related),
		"Errors":          errs,
	})
}

// dimStr — размер строкой для поля формы (14.5, не 14.500000).
func dimStr(item Artwork, which byte) string {
	if item.Dimensions == nil {
		return ""
	}
	v := item.Dimensions.W
	if which == 'h' {
		v = item.Dimensions.H
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// handleArtworkSave — POST: новая картина (файл обязателен) или правка
// существующей (поля, разделы, related).
func (a *admin) handleArtworkSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	title := strings.TrimSpace(r.FormValue("title"))
	year, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("year")))
	technique := strings.TrimSpace(r.FormValue("technique"))
	categories := trimList(r.Form["categories"])
	related := trimList(r.Form["related"])
	available := r.FormValue("available") == "on"

	reRender := func(errs []string) {
		item := Artwork{Slug: slug, Title: title, Year: year, Technique: technique,
			Categories: categories, Available: available}
		if dims := parseDimensions(r.FormValue("w"), r.FormValue("h")); dims != nil {
			item.Dimensions = dims
		}
		a.renderArtworkForm(w, st, item, r.FormValue("w"), r.FormValue("h"), errs)
	}

	var errs []string
	if title == "" {
		errs = append(errs, "Название пустое.")
	}
	if len(categories) == 0 {
		errs = append(errs, "Отметь хотя бы один раздел галереи.")
	}

	if slug == "" {
		// Новая: файл обязателен. Создание — тот же путь, что блок
		// «новая картина» в новости (дедуп, черновик, сортировка каталога).
		fh := firstFile(r, "file")
		if fh == nil {
			errs = append(errs, "Файл картины не выбран.")
		}
		if len(errs) > 0 {
			reRender(errs)
			return
		}
		newSlug, naErrs, _, created := a.addArtwork(st, fh, naForm{
			Categories: categories,
			Title:      title,
			Year:       strconv.Itoa(year),
			Technique:  technique,
			W:          r.FormValue("w"),
			H:          r.FormValue("h"),
			Available:  availStr(available),
		})
		if len(naErrs) > 0 {
			reRender(append(errs, naErrs...))
			return
		}
		if !created {
			// Дедуп: файл уже в каталоге — правь существующую работу.
			reRender([]string{"Файл уже есть в каталоге — новая запись не создана. Открой существующую работу из списка."})
			return
		}
		slug = newSlug
	} else {
		// Правка: файл не меняется (для этого — /artworks/replace).
		aw, ok := st.ArtworkBySlug(slug)
		if !ok {
			http.Error(w, "нет такой работы", http.StatusNotFound)
			return
		}
		if len(errs) > 0 {
			reRender(errs)
			return
		}
		aw.Title = title
		aw.Year = year
		aw.Technique = technique
		aw.Dimensions = parseDimensions(r.FormValue("w"), r.FormValue("h"))
		aw.Categories = categories
		aw.Available = available
		// Черновик = недозаполненное (конвенция импортёра).
		aw.NeedsMetadata = technique == "" || aw.Dimensions == nil || year == 0
	}

	setRelated(st, slug, related)

	if err := st.SaveCatalog(); err != nil {
		reRender([]string{"Не удалось записать catalog.json: " + err.Error()})
		return
	}
	http.Redirect(w, r, "/artworks?saved=1", http.StatusSeeOther)
}

// handleArtworkReplace — POST: замена файла работы. Slug/имя меняются
// (derive от hash нового файла); ссылки (related, новости) переносятся
// на новый slug; старый файл удаляется.
func (a *admin) handleArtworkReplace(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	aw, ok := st.ArtworkBySlug(slug)
	if !ok {
		http.Error(w, "нет такой работы", http.StatusNotFound)
		return
	}
	fail := func(msg string) {
		a.renderArtworkForm(w, st, *aw, dimStr(*aw, 'w'), dimStr(*aw, 'h'), []string{msg})
	}

	fh := firstFile(r, "file")
	if fh == nil {
		fail("Файл не выбран.")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !imageExts[ext] {
		fail("Файл не jpg/jpeg/png.")
		return
	}
	src, err := fh.Open()
	if err != nil {
		fail(err.Error())
		return
	}
	defer src.Close()
	data, err := ioutil.ReadAll(src)
	if err != nil {
		fail(err.Error())
		return
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	// Дедуп: такой файл уже у другой работы — заменять нельзя.
	if other, ok := st.ArtworkByHash(hash); ok && other.Slug != slug {
		fail("Такой файл уже в каталоге («" + other.Title + "») — замена не выполнена.")
		return
	}

	newSlug := "artwork-" + hash[:10]
	newFile := newSlug + ext
	if newSlug == slug && newFile == aw.File {
		// Тот же файл: перезаписи и миграции не нужны.
		fail("Файл не изменился (то же содержимое) — замена не требуется.")
		return
	}

	if err := ensureDir(a.assetsDir("artworks")); err != nil {
		fail(err.Error())
		return
	}
	if err := ioutil.WriteFile(a.assetsDir("artworks", newFile), data, 0o644); err != nil {
		fail(err.Error())
		return
	}

	oldSlug, oldFile := slug, aw.File
	aw.Slug, aw.File, aw.Hash = newSlug, newFile, hash

	// Перенос ссылок: related у соседей серии, artworks в новостях.
	for i := range st.Catalog.Artworks {
		other := &st.Catalog.Artworks[i]
		if other.Slug == newSlug {
			continue
		}
		for j, s := range other.Related {
			if s == oldSlug {
				other.Related[j] = newSlug
			}
		}
	}
	newsChanged := false
	for i := range st.News.News {
		for j, s := range st.News.News[i].Artworks {
			if s == oldSlug {
				st.News.News[i].Artworks[j] = newSlug
				newsChanged = true
			}
		}
	}

	// Старый файл больше не нужен (осознанное удаление внутри замены).
	if oldFile != "" && oldFile != newFile {
		os.Remove(a.assetsDir("artworks", oldFile))
	}

	st.reindex()
	if err := st.SaveCatalog(); err != nil {
		fail("Не удалось записать catalog.json: " + err.Error())
		return
	}
	if newsChanged {
		if err := st.SaveNews(); err != nil {
			fail("catalog.json записан, но news.json нет: " + err.Error())
			return
		}
	}

	note := "Файл заменён. Адрес страницы работы изменился: /artworks/" + newSlug
	http.Redirect(w, r, "/artworks?saved=1&note="+url.QueryEscape(note), http.StatusSeeOther)
}

// setRelated — взаимные ссылки (конвенция каталога: все related двусторонние).
// У работы slug — переданный список; соседям из списка добавляется
// обратная ссылка, у исключённых из списка — убирается.
func setRelated(st *Store, slug string, related []string) {
	aw, ok := st.ArtworkBySlug(slug)
	if !ok {
		return
	}
	clean := make([]string, 0, len(related))
	for _, s := range related {
		s = strings.TrimSpace(s)
		if s == "" || s == slug || containsStr(clean, s) {
			continue
		}
		if _, ok := st.ArtworkBySlug(s); ok {
			clean = append(clean, s)
		}
	}
	aw.Related = clean

	for i := range st.Catalog.Artworks {
		other := &st.Catalog.Artworks[i]
		if other.Slug == slug {
			continue
		}
		should := containsStr(clean, other.Slug)
		has := containsStr(other.Related, slug)
		switch {
		case should && !has:
			other.Related = append(other.Related, slug)
		case !should && has:
			filtered := make([]string, 0, len(other.Related))
			for _, s := range other.Related {
				if s != slug {
					filtered = append(filtered, s)
				}
			}
			other.Related = filtered
		}
	}
}

// trimList — значения формы без пустых и повторов, с обрезкой пробелов.
func trimList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !containsStr(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// availStr — «on» для чекбокса в naForm (новая картина через addArtwork).
func availStr(on bool) string {
	if on {
		return "on"
	}
	return ""
}