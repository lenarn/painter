// build — страница «Сборка» (шаг 4): запуск сборки сайта и публикационного
// архива прямо из админки. Наружу ничего не отправляется: «Архив» лишь
// создаёт site-*.zip в publish/, заливка на Beget — Елена через панель.
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// busy — 1, пока идёт сборка или архив: обе операции меняют web/dist,
// параллельно их запускать нельзя. GET показывает статус, POST отказывает.
var busy int32

// buildResult — итог операции для показа на странице.
type buildResult struct {
	OK      bool
	Title   string
	Seconds string
	Output  string
	Path    string // «Собрать архив»: путь к созданному zip (от корня проекта)
	Size    string // размер архива, человекочитаемый
	Hint    string // подсказка про заливку (по числу удалённых в отчёте)
}

// parseCount — число после метки в выводе архиватора («Новых: 0 …»).
// -1 — метка не найдена или число не распознано (подсказку лучше не
// показать, чем показать ложную).
func parseCount(out, label string) int {
	i := strings.Index(out, label+":")
	if i < 0 {
		return -1
	}
	s := strings.TrimLeft(out[i+len(label)+1:], " \t")
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return -1
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

// parseArchivePath — путь к zip из строки вывода «Архив:    <путь> (<размер>)».
func parseArchivePath(out string) string {
	i := strings.Index(out, "Архив:")
	if i < 0 {
		return ""
	}
	s := out[i+len("Архив:"):]
	if j := strings.IndexByte(s, '\n'); j >= 0 {
		s = s[:j]
	}
	s = strings.TrimSpace(s)
	// хвост « (300,8 МБ)»
	if k := strings.LastIndex(s, " ("); k > 0 && strings.HasSuffix(s, ")") {
		s = s[:k]
	}
	return s
}

// tailLines — последние n строк вывода (сборка печатает ~1500 строк
// прогресса; для страницы достаточно хвоста, ошибки всегда в конце).
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// runPS — запустить powershell с UTF-8 выводом (Write-Host в PS 5.1 иначе
// пишет в OEM-кодировке — кириллица приходит битой) и вернуть вывод + ошибку.
func runPS(timeout time.Duration, command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-Command", "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; "+command)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// handlePublishForm — /publish: кнопки + статус занятости.
func (a *admin) handlePublishForm(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	a.renderPublish(w, buildResult{}, atomic.LoadInt32(&busy) == 1)
}

// renderPublish — страница с результатом последней операции (если есть)
// и текущим статусом превью.
func (a *admin) renderPublish(w http.ResponseWriter, res buildResult, busyNow bool) {
	a.render(w, "publish", map[string]interface{}{
		"Page":       "publish",
		"Result":     res,
		"Busy":       busyNow,
		"Preview":    a.previewState(),
		"PreviewURL": previewURL,
	})
}

// handleBuildSite — POST /publish/build: npm run build в web/.
func (a *admin) handleBuildSite(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&busy, 0, 1) {
		// Отказ из-за занятости — не «ошибка»: показать статус на чистой
		// странице (баннер «сейчас идёт сборка или архив»).
		http.Redirect(w, r, "/publish", http.StatusSeeOther)
		return
	}
	defer atomic.StoreInt32(&busy, 0)

	web := filepath.Join(a.root, "web")
	start := time.Now()
	out, err := runPS(10*time.Minute,
		"Set-Location -LiteralPath '"+web+"'; npm run build; exit $LASTEXITCODE")
	res := buildResult{
		OK:      err == nil,
		Title:   "Сборка сайта",
		Seconds: time.Since(start).Round(time.Second).String(),
		Output:  tailLines(out, 80),
	}
	if err != nil {
		res.Output += "\n\n— ошибка: " + err.Error()
	}
	a.renderPublish(w, res, false)
}

// handleMakeArchive — POST /publish/archive: scripts/publish-pack.ps1 —
// сборка + diff-отчёт + site-*.zip в publish/ (заливка — Елена через панель).
func (a *admin) handleMakeArchive(w http.ResponseWriter, r *http.Request) {
	st := a.store(w)
	if st == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&busy, 0, 1) {
		// Отказ из-за занятости — не «ошибка»: показать статус на чистой
		// странице (баннер «сейчас идёт сборка или архив»).
		http.Redirect(w, r, "/publish", http.StatusSeeOther)
		return
	}
	defer atomic.StoreInt32(&busy, 0)

	script := filepath.Join(a.root, "scripts", "publish-pack.ps1")
	start := time.Now()
	out, err := runPS(15*time.Minute, "& '"+script+"'")
	res := buildResult{
		OK:      err == nil,
		Title:   "Архив для заливки",
		Seconds: time.Since(start).Round(time.Second).String(),
		Output:  tailLines(out, 80),
	}
	if err != nil {
		res.Output += "\n\n— ошибка: " + err.Error()
	} else {
		// Путь к архиву — из вывода скрипта; показывается только если
		// файл реально существует (страховка от смены формата вывода).
		if p := parseArchivePath(out); p != "" {
			if fi, e := os.Stat(p); e == nil {
				rel := p
				lowRoot := strings.ToLower(a.root) + string(filepath.Separator)
				if strings.HasPrefix(strings.ToLower(p), lowRoot) {
					rel = p[len(a.root)+1:] // от корня проекта
				}
				res.Path = rel
				// запятая — как в отчётах publish-pack.ps1
				res.Size = strings.Replace(fmt.Sprintf("%.1f МБ",
					float64(fi.Size())/1024/1024), ".", ",", 1)
			}
		}
		// Подсказка про заливку — по числу удалённых в отчёте.
		if n := parseCount(out, "Удалённых"); n >= 0 {
			if n == 0 {
				res.Hint = "Удалённых файлов нет — заливать можно поверх, без очистки public_html."
			} else {
				res.Hint = fmt.Sprintf("В архиве %d удалённых файлов — "+
					"перед распаковкой очисти public_html целиком.", n)
			}
		}
	}
	a.renderPublish(w, res, false)
}

// Превью собранного сайта (шаг 4, решение Елены 2026-10-09). «Открыть сайт»
// запускает astro preview (порт 4321) скрытым фоновым процессом. Занятый
// порт сначала освобождается: гасится ЛЮБОЕ дерево процессов на 4321 —
// решение Елены «без разбора» (зависший dev-сервер, старый node).
// Своё живое превью не трогается. PID — в tmp/admin-preview.pid, переживает
// перезапуск админки; потерянный pidfile не страшен: «чужое» превью будет
// погашено и запущено заново.

const (
	previewPort = 4321
	previewURL  = "http://localhost:4321/"
	// createNoWindow — CREATE_NO_WINDOW: фоновый процесс без окна консоли.
	createNoWindow = 0x08000000
)

// previewState — статус превью для страницы «Сборка».
type previewState struct {
	Running bool // порт 4321 отвечает
	Ours    bool // слушатель — наш PID из tmp/admin-preview.pid
}

// pidFile — tmp/admin-preview.pid.
func (a *admin) pidFile() string {
	return filepath.Join(a.root, "tmp", "admin-preview.pid")
}

// readPreviewPid — PID из pid-файла (0, если файла нет или в нём не число).
func (a *admin) readPreviewPid() int {
	b, err := ioutil.ReadFile(a.pidFile())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// writePreviewPid — записать PID запущенного превью.
func (a *admin) writePreviewPid(pid int) error {
	if err := os.MkdirAll(filepath.Join(a.root, "tmp"), 0o755); err != nil {
		return err
	}
	return ioutil.WriteFile(a.pidFile(), []byte(strconv.Itoa(pid)), 0o644)
}

// portListenerPIDs — PID'ы процессов, слушающих порт (разбор netstat -ano;
// состояния в netstat не локализуются, локальный адрес кончается на «:порт»).
// Без «-p tcp»: с ним netstat не отдаёт IPv6-строки, а astro preview слушает
// на [::1]. Пустой срез — порт свободен. У одного слушателя бывают строки
// IPv4 и IPv6 — PID в них один и тот же, срез дедупится.
func portListenerPIDs(port int) []int {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return nil
	}
	suffix := fmt.Sprintf(":%d", port)
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		// TCP  [::1]:4321  [::]:0  LISTENING  1234
		if len(f) >= 5 && f[0] == "TCP" && f[3] == "LISTENING" &&
			strings.HasSuffix(f[1], suffix) {
			pid, err := strconv.Atoi(f[4])
			if err != nil {
				continue
			}
			dup := false
			for _, p := range pids {
				if p == pid {
					dup = true
				}
			}
			if !dup {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

// previewUp — отвечает ли порт превью. astro preview слушает только [::1]
// (localhost резолвится в IPv6 первым), поэтому проверяем оба адреса.
func previewUp() bool {
	for _, addr := range []string{
		fmt.Sprintf("127.0.0.1:%d", previewPort),
		fmt.Sprintf("[::1]:%d", previewPort),
	} {
		c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return true
		}
	}
	return false
}

// killTree — погасить дерево процессов (сам процесс и потомков).
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// waitPreviewUp — ждать готовности порта: превью стартует пару секунд.
func waitPreviewUp(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if previewUp() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// previewState — текущий статус превью для шаблона.
func (a *admin) previewState() previewState {
	st := previewState{Running: previewUp()}
	pid := a.readPreviewPid()
	if pid == 0 {
		return st
	}
	for _, p := range portListenerPIDs(previewPort) {
		if p == pid {
			st.Ours = true
		}
	}
	return st
}

// launchPreview — запустить astro preview скрытым фоновым процессом
// (порт должен быть свободен). Без npm-обёртки — node node_modules/astro/
// astro.js preview: PID запущенного процесса = PID слушателя, своё превью
// опознаётся по pid-файлу точно.
func (a *admin) launchPreview() error {
	cmd := exec.Command("node", filepath.Join("node_modules", "astro", "astro.js"),
		"preview")
	cmd.Dir = filepath.Join(a.root, "web")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// os/exec требует Wait — иначе копится мусор; превью живёт дольше
	// запроса и не зависит от админки, поэтому ждём в горутине.
	go func() { _ = cmd.Wait() }()
	return a.writePreviewPid(cmd.Process.Pid)
}

// handlePreviewStart — GET /publish/preview: превью уже работает — редирект
// на сайт; порт занят чужим — гасим дерево и запускаем своё («без разбора»).
func (a *admin) handlePreviewStart(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&busy) == 1 {
		// Занято — показать статус на чистой странице, без «Не получилось».
		http.Redirect(w, r, "/publish", http.StatusSeeOther)
		return
	}
	pid := a.readPreviewPid()
	ours := false
	for _, p := range portListenerPIDs(previewPort) {
		if p == pid && pid != 0 {
			ours = true
		}
	}
	if !ours {
		// Порт занят чужим (или pidfile устарел) — освободить и перезапустить.
		for _, p := range portListenerPIDs(previewPort) {
			killTree(p)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && len(portListenerPIDs(previewPort)) > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		_ = os.Remove(a.pidFile()) // запись была не про живое превью
		if err := a.launchPreview(); err != nil {
			a.renderPublish(w, buildResult{Title: "Запуск превью",
				Output: "Не удалось запустить превью: " + err.Error()}, false)
			return
		}
	}
	if !waitPreviewUp(10 * time.Second) {
		a.renderPublish(w, buildResult{Title: "Запуск превью",
			Output: "Превью не поднялось за 10 секунд — возможно, " +
				"сайт не собран (web/dist пуст). Собери сайт и повтори."}, false)
		return
	}
	http.Redirect(w, r, previewURL, http.StatusSeeOther)
}

// handlePreviewStop — POST /publish/preview-stop: погасить превью.
func (a *admin) handlePreviewStop(w http.ResponseWriter, r *http.Request) {
	if pid := a.readPreviewPid(); pid != 0 {
		killTree(pid)
		_ = os.Remove(a.pidFile())
	}
	http.Redirect(w, r, "/publish", http.StatusSeeOther)
}