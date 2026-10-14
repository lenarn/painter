// afisha — страница «Афиша» (решение Елены 2026-10-14): блок «Куда пойти»
// на главной страницы сайта. Источник — сырые подборки агента-скаута
// (exhibitions/*.json, lectures/*.json в корне проекта); в админке только
// отбор: чекбоксы + «Сохранить» → web/src/content/afisha.json.
//
// Прошедшие события на сайте фильтруются по датам автоматически —
// руками ничего удалять не нужно.
package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScoutedEvent — событие из подборки агента. Поля как в его JSON
// (snake_case), только нужное.
type ScoutedEvent struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // exhibition | lecture
	Title       string `json:"title"`
	Venue       string `json:"venue"`
	Address     string `json:"address,omitempty"`
	URL         string `json:"url,omitempty"`
	DateStart   string `json:"date_start,omitempty"`
	DateEnd     string `json:"date_end,omitempty"`
	Date        string `json:"date,omitempty"`
	Time        string `json:"time,omitempty"`
	Description string `json:"description,omitempty"`
}

// eventKey — идентификатор события: один и тот же музейный сюжет агент
// находит в разных прогонах с разными id («<дата-прогона>-<slug>»), поэтому
// ключ строится из содержимого, а не из id.
func eventKey(e ScoutedEvent) string {
	day := e.DateStart
	if e.Type == "lecture" {
		day = e.Date
	}
	return strings.Join([]string{e.Type, e.Title, e.Venue, day}, "|")
}

// loadScoutedEvents — все события из сырых подборок (дедуп по eventKey,
// первый встретившийся побеждает). Битые/некалендарные файлы пропускаются
// молча: подборка — черновик, её битость не должна ломать страницу.
func loadScoutedEvents(root string) []ScoutedEvent {
	paths, _ := filepath.Glob(filepath.Join(root, "exhibitions", "*.json"))
	lectPaths, _ := filepath.Glob(filepath.Join(root, "lectures", "*.json"))
	paths = append(paths, lectPaths...)

	seen := map[string]bool{}
	var out []ScoutedEvent
	for _, p := range paths {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			continue
		}
		var list []ScoutedEvent
		if err := json.Unmarshal(b, &list); err != nil {
			continue
		}
		for _, e := range list {
			k := eventKey(e)
			if e.Title == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, e)
		}
	}
	sortScouted(out)
	return out
}

// sortScouted — хронологический порядок по дате НАЧАЛА (решение Елены
// 2026-10-14): что раньше начнётся — то сверху, что позже или ещё успеешь —
// ниже; без даты («даты уточняются») — в самый низ.
func sortScouted(evs []ScoutedEvent) {
	startDate := func(e ScoutedEvent) string {
		if e.Type == "lecture" {
			return e.Date
		}
		if e.DateStart == "" && e.DateEnd != "" {
			return e.DateEnd // начало неизвестно — ставим по концу
		}
		return e.DateStart
	}
	sort.SliceStable(evs, func(x, y int) bool {
		xd, yd := startDate(evs[x]), startDate(evs[y])
		if xd == "" != (yd == "") { // без даты — в конец
			return yd == ""
		}
		if xd != yd {
			return xd < yd
		}
		return evs[x].Title < evs[y].Title
	})
}

// afishaRow — строка панели «Отобрано на сайт»: индекс в afisha.json
// (для «убрать») + запись сайта.
type afishaRow struct {
	I int
	E AfishaEntry
}

// eventView — строка пикера/панели отбора: готовые строки для шаблона.
type eventView struct {
	Key    string
	Type   string // «выставка» / «лекция»
	Title  string
	Venue  string
	Dates  string // «08.10 — 29.11» / «24.10, 19:00»
	URL    string // источник для предпросмотра (оценить перед отбором)
	Past   bool   // последняя известная дата уже прошла
}

// eventDates — компактные даты для показа в админке.
func eventDates(e ScoutedEvent) string {
	if e.Type == "lecture" {
		s := shortDate(e.Date)
		if e.Time != "" {
			s += ", " + e.Time
		}
		return s
	}
	if e.DateStart == "" && e.DateEnd == "" {
		return "даты уточняются"
	}
	if e.DateEnd == "" {
		return "с " + shortDate(e.DateStart)
	}
	return shortDate(e.DateStart) + " — " + shortDate(e.DateEnd)
}

// shortDate — 'YYYY-MM-DD' → 'DD.MM' (без года: афиша короткая).
func shortDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02.01")
}

// isPast — последняя известная дата события раньше сегодня
// (в YYYY-MM-DD лексикографический порядок = хронологический).
func isPast(e ScoutedEvent) bool {
	return isoDate(e) != "" && isoDate(e) < time.Now().Format("2006-01-02")
}

// isoDate — последняя известная дата события.
func isoDate(e ScoutedEvent) string {
	if e.Type == "lecture" {
		return e.Date
	}
	if e.DateEnd != "" {
		return e.DateEnd
	}
	return e.DateStart
}

// afishaKey — ключ отобранной записи (зеркало eventKey): по нему чекбоксы
// находят сохранённое.
func afishaKey(e AfishaEntry) string {
	s := e.DateStart
	if e.Type == "lecture" {
		s = e.Date
	}
	return strings.Join([]string{e.Type, e.Title, e.Venue, s}, "|")
}

// isPastAfisha — то же для записи сайта afisha.json (для панели отбора).
func isPastAfisha(e AfishaEntry, today string) bool {
	s := e.DateEnd
	if e.Type == "lecture" {
		s = e.Date
	} else if s == "" {
		s = e.DateStart
	}
	return s != "" && s < today
}

// typeName — подпись типа события.
func typeName(t string) string {
	if t == "lecture" {
		return "лекция"
	}
	return "выставка"
}

// toAfishaEntry — нормализованная запись для сайта (camelCase).
func toAfishaEntry(e ScoutedEvent) AfishaEntry {
	return AfishaEntry{
		Type:        e.Type,
		Title:       e.Title,
		Venue:       e.Venue,
		Address:     e.Address,
		URL:         e.URL,
		DateStart:   e.DateStart,
		DateEnd:     e.DateEnd,
		Date:        e.Date,
		Time:        e.Time,
		Description: e.Description,
	}
}

// handleAfishaAdd — POST /afisha/add: своё событие (не из подборок агента)
// сразу в отбор (web/src/content/afisha.json). Даты:
// выставка — начало/окончание, лекция — день + время.
func (a *admin) handleAfishaAdd(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	trim := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	entry := AfishaEntry{
		Type:        trim("addType"),
		Title:       trim("addTitle"),
		Venue:       trim("addVenue"),
		URL:         trim("addUrl"),
		DateStart:   trim("addDateStart"),
		DateEnd:     trim("addDateEnd"),
		Date:        trim("addDate"),
		Time:        trim("addTime"),
		Description: strings.TrimSpace(r.FormValue("addDescription")),
	}

	errs := []string{}
	if entry.Title == "" {
		errs = append(errs, "Название пустое.")
	}
	if entry.Type != "lecture" && entry.Type != "exhibition" {
		errs = append(errs, "Тип события — выставка или лекция.")
	}
	if entry.Venue == "" {
		errs = append(errs, "Место пустое.")
	}
	if entry.Type == "lecture" && entry.Date == "" {
		// Лекция без дня не пройдёт фильтр дат на сайте — не показалась бы вовсе.
		errs = append(errs, "У лекции нужен день (YYYY-MM-DD).")
	}
	if len(errs) > 0 {
		a.renderAfisha(w, st, errs, "")
		return
	}

	entry.Manual = true // своё — переживает любое сохранение отбора
	entry.Home = true   // и всегда на главной
	// Первое в списке (решение Елены): своя выставка — самое живое и
	// важное, пусть первой встречает и на главной, и на странице.
	st.Afisha = append([]AfishaEntry{entry}, st.Afisha...)
	if err := st.SaveAfisha(); err != nil {
		a.renderAfisha(w, st, []string{"Не удалось записать afisha.json: " + err.Error()}, "")
		return
	}
	http.Redirect(w, r, "/afisha?saved=1", http.StatusSeeOther)
}

// handleAfishaRemove — POST /afisha/remove: убрать запись из отбора по
// индексу (в основном для своих событий — отобранных агентом достаточно
// снять галочку).
func (a *admin) handleAfishaRemove(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(r.FormValue("i")))
	if err != nil || idx < 0 || idx >= len(st.Afisha) {
		http.Error(w, "нет такой записи", http.StatusNotFound)
		return
	}
	st.Afisha = append(st.Afisha[:idx], st.Afisha[idx+1:]...)
	if err := st.SaveAfisha(); err != nil {
		http.Error(w, "не удалось записать afisha.json: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/afisha?saved=1", http.StatusSeeOther)
}

// handleAfishaForm — /afisha: пикер сырых событий + панель отбора.
func (a *admin) handleAfishaForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	a.renderAfisha(w, st, nil, r.URL.Query().Get("saved"))
}

func (a *admin) renderAfisha(w http.ResponseWriter, st *Store, errs []string, saved string) {
	raw := loadScoutedEvents(a.root)
	sv := a.scoutView()
	checked := map[string]bool{}
	for _, e := range st.Afisha {
		checked[afishaKey(e)] = true
	}
	homeChecked := map[string]bool{}
	for _, e := range st.Afisha {
		if e.Home || e.Manual {
			homeChecked[afishaKey(e)] = true
		}
	}
	views := make([]eventView, 0, len(raw))
	for _, e := range raw {
		views = append(views, eventView{
			Key:   eventKey(e),
			Type:  typeName(e.Type),
			Title: e.Title,
			Venue: e.Venue,
			Dates: eventDates(e),
			URL:   e.URL,
			Past:  isPast(e),
		})
	}
	selected := make([]afishaRow, 0, len(st.Afisha))
	nPast := 0
	for i, e := range st.Afisha {
		selected = append(selected, afishaRow{I: i, E: e})
		if isPastAfisha(e, time.Now().Format("2006-01-02")) {
			nPast++
		}
	}
	a.render(w, "afisha", map[string]interface{}{
		"Page":    "afisha",
		"Items":   views,
		"Checked": checked,
		"HomeChecked": homeChecked, // галочки «на главной»
		"Count":   len(st.Afisha),
		"Picked":  selected,
		"Past":    nPast,
		"Scout":   sv, // статус запуска поиска агентом
		"Errors":  errs,
		"Saved":   saved,
	})
}

// handleAfishaSave — POST /afisha/save: чекбоксы → afisha.json.
// Событие, которого уже нет в сырых подборках (агент отчитистил старые
// прогоны), сохраняется с ходу из текущего afisha.json — отбор не теряется.
func (a *admin) handleAfishaSave(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "не удалось разобрать форму: "+err.Error(), http.StatusBadRequest)
		return
	}
	checked := trimList(r.Form["afisha"])
	home := trimList(r.Form["afisha-home"]) // вторая галочка: на главной
	homeSet := make(map[string]bool, len(home))
	for _, k := range home {
		homeSet[k] = true
	}
	raw := loadScoutedEvents(a.root)
	byKey := make(map[string]AfishaEntry, len(raw))
	for _, e := range raw {
		byKey[eventKey(e)] = toAfishaEntry(e)
	}
	// Старый отбор — запас для событий, ушедших из сырых файлов.
	stored := make(map[string]AfishaEntry, len(st.Afisha))
	for _, e := range st.Afisha {
		stored[afishaKey(e)] = e
	}
	// Свои события (manual) на сайте всегда — независимо от чекбоксов:
	// в пикере их нет, снятие галочек их не должно затирать. Они же всегда
	// и на главной.
	out := make([]AfishaEntry, 0, len(checked)+1)
	for _, e := range st.Afisha {
		if e.Manual {
			e.Home = true
			out = append(out, e)
		}
	}
	for _, k := range checked {
		var e AfishaEntry
		if v, ok := byKey[k]; ok {
			e = v
		} else if v, ok := stored[k]; ok {
			e = v
		} else {
			continue
		}
		e.Home = homeSet[k] // остальное отобранное — только на странице
		out = append(out, e)
	}
	st.Afisha = out
	if err := st.SaveAfisha(); err != nil {
		a.renderAfisha(w, st, []string{"Не удалось записать afisha.json: " + err.Error()}, "")
		return
	}
	http.Redirect(w, r, "/afisha?saved=1", http.StatusSeeOther)
}