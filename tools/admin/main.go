// admin — локальная админка сайта-визитки (Фаза 3).
//
// Go stdlib only, работает на localhost: правит те же файлы контента
// (web/src/content/*.json, web/src/assets/*), что и агент. Публичного
// сервера нет: заливка на Beget — через панель, как раньше.
//
// Запуск из корня проекта:
//
//	go run ./tools/admin            # http://localhost:8150
//	go run ./tools/admin -port 8200
//
// Состав (по шагам плана, reports/admin-plan.md):
//   шаг 1 — новости: список, форма со спеллером, «+ новая картина», фото;
//   шаг 2 — картины отдельной страницей; шаг 3 — об авторе/выставки/
//   образование/контакты; шаг 4 — «Собрать сайт»/«Архив».
package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// admin — состояние приложения. Данные НЕ кэшируются: стор перечитывается
// с диска на каждый запрос (контент правят параллельно и админка, и агент —
// оба должны видеть свежие файлы; два небольших JSON читаются мгновенно).
type admin struct {
	root      string
	templates map[string]*template.Template
}

var funcMap = template.FuncMap{
	// has — есть ли значение в срезе строк (отмеченные галочки).
	"has": func(list []string, v string) bool {
		for _, s := range list {
			if s == v {
				return true
			}
		}
		return false
	},
}

func main() {
	port := flag.Int("port", 8150, "порт админки на localhost")
	root := flag.String("root", ".", "корень проекта (рабочая директория)")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalf("корень проекта: %v", err)
	}

	store, err := LoadStore(absRoot)
	if err != nil {
		log.Fatalf("чтение контента: %v", err)
	}
	_ = store // проверка на старте; в запросах стор читается заново

	a := &admin{root: absRoot}
	a.loadTemplates()

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleDashboard)
	// Миниатюры в формах: картинки из web/src/assets/ (только localhost).
	mux.Handle("/assets/", http.StripPrefix("/assets/",
		http.FileServer(http.Dir(a.assetsDir()))))
	mux.HandleFunc("/news", a.handleNewsList)
	mux.HandleFunc("/news/edit", a.handleNewsForm) // ?i= — форма записи
	mux.HandleFunc("/news/new", a.handleNewsForm)  // пустая форма
	mux.HandleFunc("/news/save", a.handleNewsSave)
	mux.HandleFunc("/artworks", a.handleArtworksList)
	mux.HandleFunc("/artworks/new", a.handleArtworkForm)    // пустая форма
	mux.HandleFunc("/artworks/edit", a.handleArtworkForm)   // ?slug= — правка
	mux.HandleFunc("/artworks/save", a.handleArtworkSave)   // новая/правка полей
	mux.HandleFunc("/artworks/replace", a.handleArtworkReplace) // замена файла
	mux.HandleFunc("/about", a.handleAboutForm)
	mux.HandleFunc("/about/save", a.handleAboutSave)
	mux.HandleFunc("/exhibitions", a.handleExhibitionsList)
	mux.HandleFunc("/exhibitions/new", a.handleExhibitionForm)
	mux.HandleFunc("/exhibitions/edit", a.handleExhibitionForm) // ?i= — правка
	mux.HandleFunc("/exhibitions/save", a.handleExhibitionSave)
	mux.HandleFunc("/education", a.handleEducationList)
	mux.HandleFunc("/education/new", a.handleEducationForm)
	mux.HandleFunc("/education/edit", a.handleEducationForm) // ?i= — правка
	mux.HandleFunc("/education/save", a.handleEducationSave)
	mux.HandleFunc("/contacts", a.handleContactsForm)
	mux.HandleFunc("/contacts/save", a.handleContactsSave)
	mux.HandleFunc("/afisha", a.handleAfishaForm)
	mux.HandleFunc("/afisha/save", a.handleAfishaSave)
	mux.HandleFunc("/afisha/add", a.handleAfishaAdd)
	mux.HandleFunc("/afisha/remove", a.handleAfishaRemove)
	mux.HandleFunc("/afisha/scout", a.handleScoutStart)
	mux.HandleFunc("/publish", a.handlePublishForm)
	mux.HandleFunc("/publish/build", a.handleBuildSite)
	mux.HandleFunc("/publish/archive", a.handleMakeArchive)
	mux.HandleFunc("/publish/preview", a.handlePreviewStart)     // запуск + редирект
	mux.HandleFunc("/publish/preview-stop", a.handlePreviewStop) // остановка
	mux.HandleFunc("/spell", a.handleSpell)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("админка: http://localhost:%d (корень проекта: %s)", *port, absRoot)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// loadTemplates — layout + страница, каждый маршрут отдельным набором.
// Go 1.14: без embed, шаблоны читаются с диска (tools/admin/templates).
func (a *admin) loadTemplates() {
	pages := map[string]string{
		"dashboard":        "dashboard.html",
		"news-list":        "news-list.html",
		"news-form":        "news-form.html",
		"artworks-list":    "artworks-list.html",
		"artwork-form":     "artwork-form.html",
		"about-form":       "about-form.html",
		"exhibitions-list": "exhibitions-list.html",
		"exhibition-form":  "exhibition-form.html",
		"education-list":   "education-list.html",
		"education-form":   "education-form.html",
		"contacts-form":    "contacts-form.html",
		"afisha":            "afisha.html",
		"publish":           "publish.html",
	}
	a.templates = make(map[string]*template.Template, len(pages))
	for name, file := range pages {
		t := template.New("layout.html").Funcs(funcMap)
		t, err := t.ParseFiles(
			filepath.Join(a.root, "tools", "admin", "templates", "layout.html"),
			filepath.Join(a.root, "tools", "admin", "templates", file),
		)
		if err != nil {
			log.Fatalf("шаблон %s: %v", file, err)
		}
		a.templates[name] = t
	}
}

func (a *admin) render(w http.ResponseWriter, name string, data interface{}) {
	t, ok := a.templates[name]
	if !ok {
		http.Error(w, "нет шаблона "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("рендер %s: %v", name, err)
	}
}

// store — свежий стор с диска для запроса (общий для обработчиков).
func (a *admin) store(w http.ResponseWriter) *Store {
	st, err := LoadStore(a.root)
	if err != nil {
		http.Error(w, "не удалось прочитать контент: "+err.Error(), http.StatusInternalServerError)
		return nil
	}
	return st
}

// handleDashboard — счётчики и заголовки разделов.
func (a *admin) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	st := a.store(w)
	if st == nil {
		return
	}
	drafts := 0
	for _, aw := range st.Catalog.Artworks {
		if aw.NeedsMetadata {
			drafts++
		}
	}
	a.render(w, "dashboard", map[string]interface{}{
		"Page":     "dashboard",
		"Artworks": len(st.Catalog.Artworks),
		"News":     len(st.News.News),
		"Drafts":   drafts,
	})
}

// assetsDir — абсолютный путь к папке ассетов (web/src/assets/...).
func (a *admin) assetsDir(parts ...string) string {
	return filepath.Join(append([]string{a.root, "web", "src", "assets"}, parts...)...)
}

// ensureDir — создать папку, если нет.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}