// scout — запуск поиска событий агентом exhibition-scout прямо из админки:
// claude в headless-режиме (claude -p) делегирует работу субагенту-скауту,
// тот ищет свежие выставки/лекции в Москве и пишет свои JSON-подборки
// (exhibitions/*.json, lectures/*.json) — они же сырьё пикера «Афиши».
//
// Поиск долгий (минуты), поэтому запускается в фоне: запрос сразу
// отвечает redirect'ом, страница «Афиши» показывает статус. Состояние —
// в памяти процесса: перезапуск админки сбрасывает его (сам поиск при
// этом живёт самостоятельно и свои файлы допишет).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// searchPrompt — задание headless-клоду от имени админки.
func searchPrompt() string {
	return "Запусти субагента exhibition-scout (Agent tool, subagent_type " +
		"'exhibition-scout') с заданием: собрать свежую подборку художественных " +
		"выставок и лекций о живописи и искусстве в Москве на " +
		time.Now().Format("2006-01-02") + " и на три месяца вперёд. " +
		"Дождись окончания его работы и верни одним абзацем: сколько записей " +
		"он сохранил, в каких файлах, и с какой датой прогона."
}

// scoutView — статус для шаблона афиши.
type scoutView struct {
	Running bool   // поиск идёт
	Started string // «в 14:03»
	Done    bool   // последний прогон завершился
	OK      bool
	Seconds string
	Output  string // хвост вывода (только после завершения)
}

// scoutState — состояние поиска под мьютексом: пишет фоновая горутина,
// читают обработчики запросов.
type scoutState struct {
	mu      sync.Mutex
	running bool
	started time.Time
	ok      bool
	seconds string
	output  string
}

var scout = &scoutState{} // указатель: мьютекс один на всю жизнь процесса

func (a *admin) scoutView() scoutView {
	scout.mu.Lock()
	defer scout.mu.Unlock()
	v := scoutView{
		Running: scout.running,
		Done:    scout.seconds != "",
		OK:      scout.ok,
		Seconds: scout.seconds,
		Output:  scout.output,
		Started: fmt.Sprintf("в %s", scout.started.Format("15:04")),
	}
	return v
}

// handleScoutStart — GET /afisha/scout: запустить поиск в фоне, вернуться
// на страницу «Афиши» (там статус; пикер обновится после завершения).
func (a *admin) handleScoutStart(w http.ResponseWriter, r *http.Request) {
	scout.mu.Lock()
	if scout.running {
		scout.mu.Unlock()
		http.Redirect(w, r, "/afisha", http.StatusSeeOther)
		return
	}
	// Поля, не структуру: подмена scout целиком унесла бы мьютекс.
	scout.running = true
	scout.started = time.Now()
	scout.ok = false
	scout.seconds = ""
	scout.output = ""
	scout.mu.Unlock()

	go a.runScout()
	http.Redirect(w, r, "/afisha", http.StatusSeeOther)
}

// runScout — сам запуск. Права claude — белым списком инструментов
// (Agent/Task, WebSearch, WebFetch, Read, Write, Glob): скаут пишет только
// свои JSON-подборки, наружу ничего не отправляет; всё прочее отклоняется.
func (a *admin) runScout() {
	start := time.Now()
	status := func(ok bool, out string) {
		scout.mu.Lock()
		scout.running = false
		scout.ok = ok
		scout.seconds = time.Since(start).Round(time.Second).String()
		scout.output = out
		scout.mu.Unlock()
	}

	// claude.exe — запуск напрямую; .cmd — через оболочку (Windows не
	// исполняет .cmd без интерпретатора). Права — узким списком: только
	// то, что нужно скауту (поиск, чтение, свои JSON-подборки, передача
	// субагенту); всё прочее в headless отклоняется автоматически.
	var run func(ctx context.Context) *exec.Cmd
	allowed := []string{
		"-p", searchPrompt(),
		"--allowedTools",
		"Agent,Task,WebSearch,WebFetch,Read,Write,Glob",
	}
	if p, err := exec.LookPath("claude.exe"); err == nil {
		run = func(c context.Context) *exec.Cmd {
			return exec.CommandContext(c, p, allowed...)
		}
	} else if p, err := exec.LookPath("claude.cmd"); err == nil {
		all := append([]string{"/C", p}, allowed...)
		run = func(c context.Context) *exec.Cmd {
			return exec.CommandContext(c, "cmd", all...)
		}
	} else {
		status(false, "клод не найден в PATH — запусти поиск в Claude Code")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	cmd := run(ctx)
	cmd.Dir = a.root
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		status(false, tailLines(string(out), 14)+
			"\n— лимит 25 минут исчерпан, поиск прерван")
		return
	}
	status(err == nil, tailLines(string(out), 14))
}