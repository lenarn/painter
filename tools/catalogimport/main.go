// catalogimport — разовый импорт существующих картин в каталог сайта.
//
// Обходит тематические папки, хеширует файлы (sha256), дедуплицирует
// (одна картина в нескольких папках = одна запись с несколькими категориями),
// копирует уникальные файлы в плоское хранилище artworks/ и генерирует
// catalog.json: для новых работ — скелет с needs_metadata=true.
//
// Метаданные известных работ сохраняются: перед генерацией читается
// существующий catalog.json, ключ записи — sha256 файла; все ручные поля
// (title/year/technique/dimensions/caption/alt/featured/needs_metadata/
// sort/featuredOrder/related/seriesOrder) переносятся, обновляются только
// генерируемые slug/file/categories.
//
// Запуск:
//
//	go run ./tools/catalogimport \
//	  -out-artworks web/src/assets/artworks \
//	  -out-catalog  web/src/content/catalog.json \
//	  "родные просторы" "портреты" "цветы" "жанровые" \
//	  "городской романс" "за морями за горами" "натюрморты"
//
// Идемпотентен: повторный запуск не дублирует записи (ключ — sha256).
// Перед перезаписью catalog.json сохраняет предыдущую версию в
// catalog.json.bak; испорченный существующий каталог — фатальная
// ошибка, а не тихая перезаливка скелетами.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// Artwork — запись каталога. Ручные поля (title/year/technique/dimensions/
// caption/alt/featured/sort/featuredOrder/related/seriesOrder) заполняются
// руками или через админку; при повторном импорте переносятся из старого
// catalog.json (ключ — hash). Импортёр генерирует только slug/file/categories.
// Размеры — float64: реальные габариты бывают дробными (14.5×20.5 у
// графики, 36.5×33.5); int ломал json.Unmarshal всего каталога.
// Целые значения маршалятся без десятичной части (50 → "50").
type Dimensions struct {
	W    float64 `json:"w,omitempty"`
	H    float64 `json:"h,omitempty"`
	Unit string  `json:"unit,omitempty"`
}

type Artwork struct {
	Slug          string         `json:"slug"`
	File          string         `json:"file"`
	Title         string         `json:"title,omitempty"`
	Year          int            `json:"year,omitempty"`
	Technique     string         `json:"technique,omitempty"`
	Dimensions    *Dimensions    `json:"dimensions,omitempty"`
	Categories    []string       `json:"categories"`
	Caption       string         `json:"caption,omitempty"`
	Alt           string         `json:"alt,omitempty"`
	Featured      bool           `json:"featured,omitempty"`
	// «В наличии» — работа готова к продаже (пометка на сайте).
	Available     bool           `json:"available,omitempty"`
	NeedsMetadata bool           `json:"needs_metadata"`
	// Ручной порядок внутри категории: ключ — имя категории.
	Sort map[string]int `json:"sort,omitempty"`
	// Ручной порядок в блоке «новая работа» (фолбэк по featured).
	FeaturedOrder *int `json:"featuredOrder,omitempty"`
	// Связанные работы (slug) для блока «Смотрите также».
	Related []string `json:"related,omitempty"`
	// Ручной порядок внутри серии (related-кластера): меньше — раньше.
	SeriesOrder *int `json:"seriesOrder,omitempty"`
	// Внутреннее: хеш для дедупа/verify (необязательное поле каталога).
	Hash string `json:"hash,omitempty"`
}

type Catalog struct {
	Artworks []Artwork `json:"artworks"`
}

type entry struct {
	hash       string
	shortHash  string
	ext        string
	source     string // путь первого найденного источника (для копирования)
	categories map[string]struct{}
}

func main() {
	outArtworks := flag.String("out-artworks", "web/src/assets/artworks", "каталог для канонических оригиналов")
	outCatalog := flag.String("out-catalog", "web/src/content/catalog.json", "путь к catalog.json")
	flag.Parse()

	srcDirs := flag.Args()
	if len(srcDirs) == 0 {
		fmt.Fprintln(os.Stderr, "ошибка: укажи хотя бы одну тематическую папку")
		os.Exit(2)
	}

	if err := os.MkdirAll(*outArtworks, 0o755); err != nil {
		fatalf("не удалось создать %s: %v", *outArtworks, err)
	}

	entries := map[string]*entry{} // hash -> entry

	for _, src := range srcDirs {
		top := filepath.Base(strings.TrimRight(src, `/\`))
		if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if !imageExts[ext] {
				return nil
			}
			h, err := fileHash(path)
			if err != nil {
				return fmt.Errorf("хеш %s: %w", path, err)
			}
			// Категория — путь от верхней папки: вложенность = подраздел
			// («иллюстрации/Сказка о…»). Файл в корне папки — просто категория.
			category := top
			if rel, relErr := filepath.Rel(src, path); relErr == nil {
				if dir := filepath.Dir(rel); dir != "." {
					category = filepath.ToSlash(filepath.Join(top, dir))
				}
			}
			e, ok := entries[h]
			if !ok {
				e = &entry{
					hash:       h,
					shortHash:  h[:10],
					ext:        ext,
					source:     path,
					categories: map[string]struct{}{},
				}
				entries[h] = e
			}
			e.categories[category] = struct{}{}
			return nil
		}); err != nil {
			fatalf("обход %s: %v", src, err)
		}
	}

	// Копируем уникальные файлы и строим каталог.
	// У известных работ (hash из старого catalog.json) переносим ручные
	// метаданные, обновляя только генерируемые поля.
	existing := loadExisting(*outCatalog)
	artworks := make([]Artwork, 0, len(entries))
	for _, e := range entries {
		fileName := "artwork-" + e.shortHash + e.ext
		dst := filepath.Join(*outArtworks, fileName)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := copyFile(e.source, dst); err != nil {
				fatalf("копирование %s: %v", fileName, err)
			}
		}
		a := Artwork{
			Slug:          "artwork-" + e.shortHash,
			File:          fileName,
			Categories:    sortedKeys(e.categories),
			NeedsMetadata: true,
			Hash:          e.hash,
		}
		if old, ok := existing[e.hash]; ok {
			a = old
			a.Slug = "artwork-" + e.shortHash
			a.File = fileName
			a.Categories = sortedKeys(e.categories)
			a.Hash = e.hash
		}
		artworks = append(artworks, a)
	}

	sort.Slice(artworks, func(i, j int) bool { return artworks[i].Slug < artworks[j].Slug })

	cat := Catalog{Artworks: artworks}
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		fatalf("marshal catalog: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(*outCatalog), 0o755); err != nil {
		fatalf("не удалось создать каталог для %s: %v", *outCatalog, err)
	}
	// Бэкап перед перезаписью — страховка ручных подписей каталога.
	if _, err := os.Stat(*outCatalog); err == nil {
		if err := copyFile(*outCatalog, *outCatalog+".bak"); err != nil {
			fatalf("бэкап %s: %v", *outCatalog+".bak", err)
		}
	}
	if err := writeFile(*outCatalog, data); err != nil {
		fatalf("запись %s: %v", *outCatalog, err)
	}

	fmt.Printf("импорт завершён: %d уникальных работ → %s\n", len(artworks), *outCatalog)
}

// loadExisting — записи существующего catalog.json по hash.
// Нет файла → пустая карта (поведение первой заливки). Файл есть, но
// не читается, не парсится или не содержит ни одного hash → фатальная
// ошибка: молчаливый пустой мерж затирает все ручные подписи
// (так 2026-09-26 слетел весь каталог).
func loadExisting(path string) map[string]Artwork {
	out := map[string]Artwork{}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out // первая заливка — старого каталога нет
		}
		fatalf("не удалось прочитать %s (мерж невозможен): %v", path, err)
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		fatalf("не удалось разобрать %s (мерж невозможен): %v", path, err)
	}
	for _, a := range cat.Artworks {
		if a.Hash != "" {
			out[a.Hash] = a
		}
	}
	if len(cat.Artworks) > 0 && len(out) == 0 {
		fatalf("%s: %d записей без поля hash — мерж по hash невозможен", path, len(cat.Artworks))
	}
	return out
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// writeFile — атомарная запись (temp+rename), замена os.WriteFile для Go 1.14.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := out.Write(data); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ошибка: "+format+"\n", args...)
	os.Exit(1)
}