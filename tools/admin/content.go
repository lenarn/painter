// content — контентные разделы (шаг 3): об авторе (имя, intro, союзы, фото),
// выставки, образование, контакты. Данные — те же JSON, что на сайте
// (web/src/content/*.json); порядок полей Go-структур = порядок ключей
// в файлах, чтобы правки не порождали лишний diff.
package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ───────────────────────── Об авторе ─────────────────────────

// handleAboutForm — /about: форма профиля.
func (a *admin) handleAboutForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	a.renderAboutForm(w, st.About, nil, r.URL.Query().Get("saved"))
}

// renderAboutForm — данные шаблона (в т.ч. перерисовка после ошибки,
// когда форма ещё не сохранена в стор).
func (a *admin) renderAboutForm(w http.ResponseWriter, data About, errs []string, saved string) {
	a.render(w, "about-form", map[string]interface{}{
		"Page":        "about",
		"Name":        data.Name,
		"Surname":     data.Surname,
		"Intro":       strings.Join(data.Intro, "\n"),
		"Memberships": data.Memberships,
		"Errors":      errs,
		"Saved":       saved,
	})
}

// handleAboutSave — POST /about/save: поля профиля + опционально новое фото
// (перезапись web/src/assets/about/photo1.jpg — осознанная, отдельным действием
// загрузки).
func (a *admin) handleAboutSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}

	data := About{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Surname:     strings.TrimSpace(r.FormValue("surname")),
		Intro:       splitParagraphs(r.FormValue("intro")),
		Memberships: membershipsFromForm(r),
	}

	var errs []string
	if data.Name == "" {
		errs = append(errs, "Имя пустое.")
	}
	if data.Surname == "" {
		errs = append(errs, "Фамилия пустая.")
	}
	if len(data.Intro) == 0 {
		errs = append(errs, "Текст «Об авторе» пустой.")
	}

	// Фото: jpg/jpeg → перезапись photo1.jpg.
	if fh := firstFile(r, "photo"); fh != nil {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext != ".jpg" && ext != ".jpeg" {
			errs = append(errs, "Фото — только jpg/jpeg (пересохрани, пожалуйста).")
		} else if len(errs) == 0 {
			src, err := fh.Open()
			if err != nil {
				errs = append(errs, "Фото: "+err.Error())
			} else {
				b, readErr := ioutil.ReadAll(src)
				src.Close()
				if readErr != nil {
					errs = append(errs, "Фото: "+readErr.Error())
				} else if err := ensureDir(a.assetsDir("about")); err != nil {
					errs = append(errs, "Фото: "+err.Error())
				} else if err := ioutil.WriteFile(a.assetsDir("about", "photo1.jpg"), b, 0o644); err != nil {
					errs = append(errs, "Фото: "+err.Error())
				}
			}
		}
	}

	if len(errs) > 0 {
		a.renderAboutForm(w, data, errs, "")
		return
	}

	st.About = data
	if err := st.SaveAbout(); err != nil {
		a.renderAboutForm(w, data, []string{"Не удалось записать about.json: " + err.Error()}, "")
		return
	}
	http.Redirect(w, r, "/about?saved=1", http.StatusSeeOther)
}

// membershipsFromForm — строки «союз + год»: mem_count + mem_title_<i> +
// mem_year_<i>. Удалённые в браузере строки оставляют дыры в нумерации —
// пустые названия пропускаются, порядок сохраняется.
func membershipsFromForm(r *http.Request) []Membership {
	n, _ := strconv.Atoi(r.FormValue("mem_count"))
	out := make([]Membership, 0, n)
	for i := 0; i < n; i++ {
		title := strings.TrimSpace(r.FormValue(fmt.Sprintf("mem_title_%d", i)))
		if title == "" {
			continue
		}
		year, _ := strconv.Atoi(strings.TrimSpace(r.FormValue(fmt.Sprintf("mem_year_%d", i))))
		out = append(out, Membership{Title: title, Year: year})
	}
	return out
}

// ───────────────────────── Выставки ─────────────────────────

// exRow — строка списка выставок: запись + её индекс в исходном массиве
// (правка по индексу, как у новостей).
type exRow struct {
	I int
	ExhibitionEntry
}

// handleExhibitionsList — /exhibitions: год desc, внутри года sortStart desc
// (порядок как на сайте, content.ts).
func (a *admin) handleExhibitionsList(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	rows := make([]exRow, 0, len(st.Exhibitions))
	for i := range st.Exhibitions {
		rows = append(rows, exRow{I: i, ExhibitionEntry: st.Exhibitions[i]})
	}
	sort.SliceStable(rows, func(x, y int) bool {
		if rows[x].Year != rows[y].Year {
			return rows[x].Year > rows[y].Year
		}
		return rows[x].SortStart > rows[y].SortStart
	})
	a.render(w, "exhibitions-list", map[string]interface{}{
		"Page":  "exhibitions",
		"Items": rows,
		"Saved": r.URL.Query().Get("saved"),
	})
}

// handleExhibitionForm — /exhibitions/new и /exhibitions/edit?i=.
func (a *admin) handleExhibitionForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	item := ExhibitionEntry{Note: "", Place: ""}
	idx := formIndex(r)
	if idx >= 0 {
		if idx >= len(st.Exhibitions) {
			http.Error(w, "нет такой записи", http.StatusNotFound)
			return
		}
		item = st.Exhibitions[idx]
	}
	a.render(w, "exhibition-form", map[string]interface{}{
		"Page":            "exhibitions",
		"Index":           idx,
		"Item":            item,
		"Saved":           r.URL.Query().Get("saved"),
		"Artworks":        artworkViews(st),
		"CheckedArtworks": checkedMap(item.Artworks),
		"Linked":          linkedViews(st, item.Artworks),
		"Errors":          []string{},
	})
}

// handleExhibitionSave — POST: новая запись или правка по индексу.
func (a *admin) handleExhibitionSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	idx := formIndex(r)
	item := exhibitionFromForm(r)

	var errs []string
	if item.Title == "" {
		errs = append(errs, "Название пустое.")
	}
	if item.Period == "" {
		errs = append(errs, "Период пустой (как на сайте: «сен 2026», «24.01 — 25.02.2024»).")
	}
	if item.Year == 0 {
		errs = append(errs, "Год не указан.")
	}
	if item.SortStart == "" {
		// Сортировка на сайте — по sortStart; новая запись без него встаёт
		// последней в своём году.
		if item.Year == 0 {
			errs = append(errs, "Сортировка не указана (дата вида 2026-09-01).")
		} else {
			item.SortStart = fmt.Sprintf("%d-01-01", item.Year)
		}
	}
	if idx >= len(st.Exhibitions) {
		errs = append(errs, "Запись не найдена — открой список заново.")
		idx = -1
	}

	renderForm := func(errs []string) {
		a.render(w, "exhibition-form", map[string]interface{}{
			"Page":            "exhibitions",
			"Index":           idx,
			"Item":            item,
			"Artworks":        artworkViews(st),
			"CheckedArtworks": checkedMap(item.Artworks),
			"Linked":          linkedViews(st, item.Artworks),
			"Errors":          errs,
		})
	}
	if len(errs) > 0 {
		renderForm(errs)
		return
	}

	if idx < 0 {
		st.Exhibitions = append(st.Exhibitions, item)
		idx = len(st.Exhibitions) - 1 // правка новой записи по её индексу
	} else {
		st.Exhibitions[idx] = item
	}
	if err := st.SaveExhibitions(); err != nil {
		renderForm([]string{"Не удалось записать exhibitions.json: " + err.Error()})
		return
	}
	// Возврат на форму правки (решение Елены 2026-10-14), не на список.
	http.Redirect(w, r,
		fmt.Sprintf("/exhibitions/edit?i=%d&saved=1", idx), http.StatusSeeOther)
}

func exhibitionFromForm(r *http.Request) ExhibitionEntry {
	year, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("year")))
	return ExhibitionEntry{
		Year:      year,
		SortStart: strings.TrimSpace(r.FormValue("sortStart")),
		Period:    strings.TrimSpace(r.FormValue("period")),
		Title:     strings.TrimSpace(r.FormValue("title")),
		Place:     strings.TrimSpace(r.FormValue("place")),
		Note:      strings.TrimSpace(r.FormValue("note")),
		Artworks:  trimList(r.Form["artworks"]),
	}
}

// ───────────────────────── Образование ─────────────────────────

// eduRow — строка списка образования: запись + её индекс в исходном массиве.
type eduRow struct {
	I int
	EducationEntry
}

// handleEducationList — /education: sortStart desc (порядок как на сайте).
func (a *admin) handleEducationList(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	rows := make([]eduRow, 0, len(st.Education))
	for i := range st.Education {
		rows = append(rows, eduRow{I: i, EducationEntry: st.Education[i]})
	}
	sort.SliceStable(rows, func(x, y int) bool {
		return rows[x].SortStart > rows[y].SortStart
	})
	a.render(w, "education-list", map[string]interface{}{
		"Page":  "education",
		"Items": rows,
		"Saved": r.URL.Query().Get("saved"),
	})
}

// handleEducationForm — /education/new и /education/edit?i=.
func (a *admin) handleEducationForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	item := EducationEntry{}
	idx := formIndex(r)
	if idx >= 0 {
		if idx >= len(st.Education) {
			http.Error(w, "нет такой записи", http.StatusNotFound)
			return
		}
		item = st.Education[idx]
	}
	a.renderEducationForm(w, idx, item, nil)
}

func (a *admin) renderEducationForm(w http.ResponseWriter, idx int, item EducationEntry, errs []string) {
	a.render(w, "education-form", map[string]interface{}{
		"Page":   "education",
		"Index":  idx,
		"Item":   item,
		"Errors": errs,
	})
}

// handleEducationSave — POST: новая запись или правка по индексу.
func (a *admin) handleEducationSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	idx := formIndex(r)
	item := EducationEntry{
		SortStart: strings.TrimSpace(r.FormValue("sortStart")),
		Period:    strings.TrimSpace(r.FormValue("period")),
		Title:     strings.TrimSpace(r.FormValue("title")),
		Place:     strings.TrimSpace(r.FormValue("place")),
		Highlight: r.FormValue("highlight") == "on",
	}

	var errs []string
	if item.Period == "" {
		errs = append(errs, "Период пустой (как на сайте: «сен 2025 – май 2026»).")
	}
	if item.Title == "" {
		errs = append(errs, "Название пустое.")
	}
	if item.SortStart == "" {
		errs = append(errs, "Сортировка не указана (вид 2025-09 — месяц начала).")
	}
	if idx >= len(st.Education) {
		errs = append(errs, "Запись не найдена — открой список заново.")
		idx = -1
	}

	if len(errs) > 0 {
		a.renderEducationForm(w, idx, item, errs)
		return
	}

	if idx < 0 {
		st.Education = append(st.Education, item)
	} else {
		st.Education[idx] = item
	}
	if err := st.SaveEducation(); err != nil {
		a.renderEducationForm(w, idx, item, []string{"Не удалось записать education.json: " + err.Error()})
		return
	}
	http.Redirect(w, r, "/education?saved=1", http.StatusSeeOther)
}

// ───────────────────────── Контакты ─────────────────────────

// handleContactsForm — /contacts: почта + соцсети.
func (a *admin) handleContactsForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	a.renderContactsForm(w, st.Contacts, nil, r.URL.Query().Get("saved"))
}

func (a *admin) renderContactsForm(w http.ResponseWriter, data Contacts, errs []string, saved string) {
	a.render(w, "contacts-form", map[string]interface{}{
		"Page":    "contacts",
		"Data":    data,
		"Errors":  errs,
		"Saved":   saved,
	})
}

// handleContactsSave — POST /contacts/save: почта + строки соцсетей.
func (a *admin) handleContactsSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	data := Contacts{
		Email:   strings.TrimSpace(r.FormValue("email")),
		Socials: socialsFromForm(r),
	}

	var errs []string
	if data.Email == "" {
		errs = append(errs, "Почта пустая.")
	}
	for i, s := range data.Socials {
		if s.Main.Href == "" {
			errs = append(errs, fmt.Sprintf("Соцсеть %d: не указан публичный адрес.", i+1))
		}
	}

	if len(errs) > 0 {
		a.renderContactsForm(w, data, errs, "")
		return
	}

	st.Contacts = data
	if err := st.SaveContacts(); err != nil {
		a.renderContactsForm(w, data, []string{"Не удалось записать contacts.json: " + err.Error()}, "")
		return
	}
	http.Redirect(w, r, "/contacts?saved=1", http.StatusSeeOther)
}

// socialsFromForm — строки соцсетей: soc_count + поля soc_<name|main_label|
// main_href|write_label|write_href>_<i>. Пустые строки (без названия) пропускаются.
func socialsFromForm(r *http.Request) []ContactSocial {
	n, _ := strconv.Atoi(r.FormValue("soc_count"))
	out := make([]ContactSocial, 0, n)
	for i := 0; i < n; i++ {
		p := func(field string) string {
			return strings.TrimSpace(r.FormValue(fmt.Sprintf("soc_%s_%d", field, i)))
		}
		if p("name") == "" {
			continue
		}
		out = append(out, ContactSocial{
			Name:  p("name"),
			Main:  ContactLink{Label: p("main_label"), Href: p("main_href")},
			Write: ContactLink{Label: p("write_label"), Href: p("write_href")},
		})
	}
	return out
}