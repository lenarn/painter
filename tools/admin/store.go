// store — чтение/запись JSON-файлов контента сайта (news.json, catalog.json).
//
// Те же файлы, что правит агент: конвейер «через агента» и админка работают
// с одними данными. Запись атомарная (temp+rename), перед перезаписью —
// <файл>.bak (как у tools/catalogimport). Формат: UTF-8, LF, отступ 2 пробела,
// без HTML-экранирования, завершающий перенос строки — как в текущих файлах.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Dimensions — размеры работы. float64: габариты бывают дробными
// (14.5×20.5 у графики); целые маршалятся без десятичной части.
type Dimensions struct {
	W    float64 `json:"w,omitempty"`
	H    float64 `json:"h,omitempty"`
	Unit string  `json:"unit,omitempty"`
}

// Artwork — запись каталога. Порядок полей = порядок в catalog.json
// (доминирующий порядок записей файла), чтобы diff при записи был минимальным.
type Artwork struct {
	Slug          string      `json:"slug"`
	File          string      `json:"file"`
	Categories    []string    `json:"categories"`
	NeedsMetadata bool        `json:"needs_metadata"`
	Hash          string      `json:"hash,omitempty"`
	Title         string      `json:"title,omitempty"`
	Year          int         `json:"year,omitempty"`
	Technique     string      `json:"technique,omitempty"`
	Dimensions    *Dimensions `json:"dimensions,omitempty"`
	Featured      bool        `json:"featured,omitempty"`
	Available     bool        `json:"available,omitempty"`
	Caption       string      `json:"caption,omitempty"`
	Alt           string      `json:"alt,omitempty"`
	Sort          map[string]int `json:"sort,omitempty"`
	FeaturedOrder *int        `json:"featuredOrder,omitempty"`
	Related       []string    `json:"related,omitempty"`
	SeriesOrder   *int        `json:"seriesOrder,omitempty"`
}

type Catalog struct {
	Artworks []Artwork `json:"artworks"`
}

// NewsItem — запись ленты. Порядок полей = как в news.json.
type NewsItem struct {
	Date     string   `json:"date"`
	Title    string   `json:"title"`
	Text     []string `json:"text"`
	Artworks []string `json:"artworks"`
	Photos   []string `json:"photos,omitempty"`
}

type News struct {
	News []NewsItem `json:"news"`
}

// Membership — творческий союз (about.json).
type Membership struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}

type About struct {
	Name        string       `json:"name"`
	Surname     string       `json:"surname"`
	Intro       []string     `json:"intro"`
	Memberships []Membership `json:"memberships"`
}

// EducationEntry — запись образования. Порядок полей = как в education.json;
// highlight с omitempty — ключ есть только у выделенных записей.
type EducationEntry struct {
	SortStart string `json:"sortStart"`
	Period    string `json:"period"`
	Title     string `json:"title"`
	Place     string `json:"place"`
	Highlight bool   `json:"highlight,omitempty"`
}

// ExhibitionEntry — запись выставки. Порядок полей = как в exhibitions.json;
// place/note записываются всегда (в файле ключи есть и пустые).
type ExhibitionEntry struct {
	Year      int      `json:"year"`
	SortStart string   `json:"sortStart"`
	Period    string   `json:"period"`
	Title     string   `json:"title"`
	Place     string   `json:"place"`
	Note      string   `json:"note"`
	Artworks  []string `json:"artworks,omitempty"` // slug работ из каталога (миниатюры на сайте)
}

// AfishaEntry — отобранное событие афиши («Куда пойти» на главной),
// web/src/content/afisha.json. Пишется из сырых подборок агента
// (exhibitions/*.json, lectures/*.json) целиком — в админке только отбор.
type AfishaEntry struct {
	Type        string `json:"type"` // exhibition | lecture
	Title       string `json:"title"`
	Venue       string `json:"venue"`
	Address     string `json:"address,omitempty"`
	URL         string `json:"url,omitempty"`
	DateStart   string `json:"dateStart,omitempty"` // выставка: начало, YYYY-MM-DD
	DateEnd     string `json:"dateEnd,omitempty"`   // выставка: окончание
	Date        string `json:"date,omitempty"`      // лекция: день, YYYY-MM-DD
	Time        string `json:"time,omitempty"`      // лекция: HH:MM
	Description string `json:"description,omitempty"`
	Manual      bool   `json:"manual,omitempty"` // своё событие (не из подборок агента)
	Home        bool   `json:"home"` // показывать в блоке на главной; false пишется явно: без omitempty оно стиралось и сайт считал отсутствие поля «показывать»
}

// ContactLink — ссылка контакта (label — подпись, href — адрес).
type ContactLink struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

// ContactSocial — соцсеть: публичный адрес + адрес для личных сообщений.
type ContactSocial struct {
	Name  string      `json:"name"`
	Main  ContactLink `json:"main"`
	Write ContactLink `json:"write"`
}

type Contacts struct {
	Email   string          `json:"email"`
	Socials []ContactSocial `json:"socials"`
}

// Store — загруженные файлы контента и пути к ним.
type Store struct {
	NewsPath    string
	CatalogPath string

	News    News
	Catalog Catalog

	AboutPath       string
	EducationPath   string
	ExhibitionsPath string
	ContactsPath    string

	About       About
	Education   []EducationEntry
	Exhibitions []ExhibitionEntry
	Contacts    Contacts

	AfishaPath string
	Afisha     []AfishaEntry // отбор для «Куда пойти» на главной

	// Карта slug → работа (обновляется вместе с Catalog).
	bySlug map[string]*Artwork
}

func LoadStore(root string) (*Store, error) {
	s := &Store{
		NewsPath:        filepath.Join(root, "web", "src", "content", "news.json"),
		CatalogPath:     filepath.Join(root, "web", "src", "content", "catalog.json"),
		AboutPath:       filepath.Join(root, "web", "src", "content", "about.json"),
		EducationPath:   filepath.Join(root, "web", "src", "content", "education.json"),
		ExhibitionsPath: filepath.Join(root, "web", "src", "content", "exhibitions.json"),
		ContactsPath:    filepath.Join(root, "web", "src", "content", "contacts.json"),
		AfishaPath:      filepath.Join(root, "web", "src", "content", "afisha.json"),
	}
	if err := readJSON(s.NewsPath, &s.News); err != nil {
		return nil, fmt.Errorf("news.json: %v", err)
	}
	if err := readJSON(s.CatalogPath, &s.Catalog); err != nil {
		return nil, fmt.Errorf("catalog.json: %v", err)
	}
	if err := readJSON(s.AboutPath, &s.About); err != nil {
		return nil, fmt.Errorf("about.json: %v", err)
	}
	if err := readJSON(s.EducationPath, &s.Education); err != nil {
		return nil, fmt.Errorf("education.json: %v", err)
	}
	if err := readJSON(s.ExhibitionsPath, &s.Exhibitions); err != nil {
		return nil, fmt.Errorf("exhibitions.json: %v", err)
	}
	// contacts.json — новый файл (шаг 3); отсутствие — не ошибка, форма пустая.
	if err := readJSON(s.ContactsPath, &s.Contacts); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("contacts.json: %v", err)
	}
	// afisha.json — тоже может отсутствовать: афиша ещё не отбиралась.
	if err := readJSON(s.AfishaPath, &s.Afisha); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("afisha.json: %v", err)
	}
	s.reindex()
	return s, nil
}

// reindex — пересобрать карту slug → работа (после изменения Catalog).
func (s *Store) reindex() {
	s.bySlug = make(map[string]*Artwork, len(s.Catalog.Artworks))
	for i := range s.Catalog.Artworks {
		s.bySlug[s.Catalog.Artworks[i].Slug] = &s.Catalog.Artworks[i]
	}
}

func (s *Store) ArtworkBySlug(slug string) (*Artwork, bool) {
	a, ok := s.bySlug[slug]
	return a, ok
}

// ArtworkByHash — существующая работа по sha256 файла (дедуп при загрузке).
func (s *Store) ArtworkByHash(hash string) (*Artwork, bool) {
	for i := range s.Catalog.Artworks {
		if s.Catalog.Artworks[i].Hash == hash {
			return &s.Catalog.Artworks[i], true
		}
	}
	return nil, false
}

// SaveNews — бэкап + атомарная запись news.json.
func (s *Store) SaveNews() error {
	return writeJSON(s.NewsPath, s.News)
}

// SaveCatalog — бэкап + атомарная запись catalog.json.
func (s *Store) SaveCatalog() error {
	return writeJSON(s.CatalogPath, s.Catalog)
}

// SaveAbout / SaveEducation / SaveExhibitions / SaveContacts — то же для
// контентных разделов (шаг 3).
func (s *Store) SaveAbout() error       { return writeJSON(s.AboutPath, s.About) }
func (s *Store) SaveEducation() error  { return writeJSON(s.EducationPath, s.Education) }
func (s *Store) SaveExhibitions() error {
	return writeJSON(s.ExhibitionsPath, s.Exhibitions)
}
func (s *Store) SaveContacts() error { return writeJSON(s.ContactsPath, s.Contacts) }

// SaveAfisha — бэкап + атомарная запись afisha.json.
func (s *Store) SaveAfisha() error { return writeJSON(s.AfishaPath, s.Afisha) }

// Categories — все уникальные категории каталога (полные пути, включая
// вложенные «раздел/подраздел»), отсортированные — как на сайте.
func (s *Store) Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range s.Catalog.Artworks {
		for _, c := range a.Categories {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

func readJSON(path string, v interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON — бэкап старого файла, затем атомарная запись нового
// (temp+rename). Отступ 2 пробела, LF, без HTML-экранирования,
// завершающий перенос строки.
func writeJSON(path string, v interface{}) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		if err := copyFileBytes(path, path+".bak"); err != nil {
			return fmt.Errorf("бэкап: %v", err)
		}
	}

	tmp := path + ".tmp"
	if err := ioutil.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func copyFileBytes(src, dst string) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, data, 0o644)
}