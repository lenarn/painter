// speller — проверка орфографии через бесплатный HTTPS-API Яндекс.Спеллера
// (решение Елены 2026-10-09, reports/admin-plan.md).
//
// POST https://speller.yandex.net/services/spellservice.json/checkText
// с формой text=<текст>&lang=ru — без ключа и регистрации; лимиты
// (10 000 запросов / 10 млн символов в сутки) заведомо с запасом.
//
// Мягкая деградация: API недоступен → 503 + понятное сообщение,
// сохранение форм от проверки не зависит. Текст черновика уходит на
// сервер Яндекса — Елена предупреждена.
package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const spellerURL = "https://speller.yandex.net/services/spellservice.json/checkText"

var spellerClient = &http.Client{Timeout: 10 * time.Second}

// spellFinding — одно найденное слово с подсказками (форма ответа Спеллера
// урезана до нужного админке).
type spellFinding struct {
	Word       string   `json:"word"`
	Len        int      `json:"len"`
	Pos        int      `json:"pos"`
	Suggests   []string `json:"s"`
}

// checkSpeller — проверить один текст. Ошибка сети/недоступность — err
// (вызывающий показывает «проверка недоступна», сохранение не блокируется).
func checkSpeller(text string) ([]spellFinding, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	resp, err := spellerClient.PostForm(spellerURL, url.Values{
		"text": {text},
		"lang": {"ru"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errAPINotAvailable
	}
	var raw []struct {
		Word string   `json:"word"`
		Len  int      `json:"len"`
		Pos  int      `json:"pos"`
		S    []string `json:"s"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]spellFinding, 0, len(raw))
	for _, r := range raw {
		if len(r.S) == 0 {
			continue // без подсказок нечего показать/исправить
		}
		out = append(out, spellFinding{Word: r.Word, Len: r.Len, Pos: r.Pos, Suggests: r.S})
	}
	return out, nil
}

type spellerError string

func (e spellerError) Error() string { return string(e) }

const errAPINotAvailable = spellerError("API недоступен")

// handleSpell — POST {field: text, ...} → {field: [находки], ...}.
// Каждый текстовый поля формы проверяется отдельным запросом Спеллера —
// позиции находок остаются в координатах своего поля.
func (a *admin) handleSpell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var fields map[string]string
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		http.Error(w, "битый запрос", http.StatusBadRequest)
		return
	}
	out := map[string][]spellFinding{}
	var firstErr string
	for name, text := range fields {
		findings, err := checkSpeller(text)
		if err != nil {
			// Недоступность не валит весь запрос: поля без ответа молча
			// пропускаются, фронт получит available:false.
			if firstErr == "" {
				firstErr = "Проверка недоступна (нет сети или API Яндекса)"
			}
			continue
		}
		if len(findings) > 0 {
			out[name] = findings
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": firstErr == "",
		"error":     firstErr,
		"fields":    out,
	})
}