package scoreboard

import (
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jollheef/henhouse/db"
)

const adminSupportSummaryRunes = 70

func tgTokenHint(token string) string {
	if token == "" {
		return "Токен не задан. Создайте бота через @BotFather и вставьте токен сюда"
	}
	tail := token
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return "Токен задан (…" + tail + "). Оставьте поле пустым, чтобы не менять"
}

func tgConfigStateHTML(cfg tgConfig) string {
	var cls, txt string
	switch {
	case cfg.ready():
		cls, txt = "admin-status-open", "включено"
	case cfg.Enabled:
		cls, txt = "admin-status-forced", "включено, но не заполнены токен или chat_id"
	default:
		cls, txt = "admin-status-closed", "выключено"
	}
	return `<span class="admin-status ` + cls + `">` + txt + `</span>`
}

func adminSupportSettingsHTML(cfg tgConfig, csrf string) string {
	checked := ""
	if cfg.Enabled {
		checked = " checked"
	}

	out := `<div class="admin-note">Новые обращения приходят личным сообщением от вашего бота. ` +
		`Создайте бота через @BotFather, отправьте ему /start со своего аккаунта, ` +
		`узнайте свой chat_id (например, через @userinfobot) и заполните поля ниже.</div>` +
		`<form method="post" action="/admin/support/settings" class="admin-form">` +
		adminHidden("csrf", csrf) +
		`<div class="admin-form-grid">` +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="tg-token">Токен бота</label>` +
		`<input class="admin-input" type="password" id="tg-token" name="token" value=""` +
		` autocomplete="new-password" spellcheck="false" placeholder="123456789:AA...">` +
		`<div class="admin-form-hint">` + html.EscapeString(tgTokenHint(cfg.Token)) + `</div>` +
		`</div>` +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="tg-chat">Chat ID</label>` +
		`<input class="admin-input" type="text" id="tg-chat" name="chat_id" value="` +
		html.EscapeString(cfg.ChatID) + `" autocomplete="off" spellcheck="false"` +
		` placeholder="123456789">` +
		`<div class="admin-form-hint">Числовой идентификатор личного чата с ботом</div>` +
		`</div>` +
		`</div>` +
		`<label class="admin-check-row">` +
		`<input class="admin-checkbox" type="checkbox" name="enabled"` + checked + `>` +
		`<span>Отправлять новые обращения в Telegram</span>` +
		`</label>`

	if cfg.Token != "" {
		out += `<label class="admin-check-row">` +
			`<input class="admin-checkbox" type="checkbox" name="clear_token">` +
			`<span>Удалить сохранённый токен</span>` +
			`</label>`
	}

	out += `<div class="admin-form-actions">` +
		`<button class="admin-btn admin-btn-primary" type="submit">Сохранить</button>` +
		`</div>` +
		`</form>` +
		`<div class="admin-quick-actions">` +
		`<form method="post" action="/admin/support/test" class="admin-inline-form">` +
		adminHidden("csrf", csrf) +
		`<button class="admin-btn" type="submit">Отправить тестовое сообщение</button>` +
		`</form>` +
		`</div>`

	return out
}

func adminSupportTgStatusHTML(status string) string {
	if status == "" {
		return ""
	}

	cls := "admin-status-closed"
	switch {
	case strings.HasPrefix(status, "отправлено без вложения"):
		cls = "admin-status-forced"
	case strings.HasPrefix(status, "отправлено"):
		cls = "admin-status-open"
	case strings.HasPrefix(status, "ошибка"),
		strings.HasPrefix(status, "не отправлено"):
		cls = "admin-status-forced"
	}

	return `<span class="admin-status ` + cls + `">` +
		html.EscapeString(status) + `</span>`
}

func adminSupportTextHTML(text string) string {
	full := html.EscapeString(text)

	r := []rune(text)
	if len(r) <= adminSupportSummaryRunes {
		return `<div class="admin-sup-text">` + full + `</div>`
	}

	summary := html.EscapeString(string(r[:adminSupportSummaryRunes])) + "…"
	return `<details class="admin-sup-details">` +
		`<summary>` + summary + `</summary>` +
		`<div class="admin-sup-text">` + full + `</div>` +
		`</details>`
}

func adminSupportContactHTML(contact string) string {
	if contact == "" {
		return ""
	}

	esc := html.EscapeString(contact)
	if strings.HasPrefix(contact, "http://") ||
		strings.HasPrefix(contact, "https://") {
		return `<a class="admin-link" href="` + esc +
			`" target="_blank" rel="noopener noreferrer">` + esc + `</a>`
	}
	return esc
}

func adminSupportListHTML(database *sql.DB, csrf string) string {
	reqs, err := db.GetSupportRequests(database)
	if err != nil {
		return `<div class="admin-empty">Не удалось получить обращения</div>`
	}

	if len(reqs) == 0 {
		return `<div class="admin-empty">Обращений пока нет</div>`
	}

	teamName := map[int]string{}
	if teams, terr := db.GetTeams(database); terr == nil {
		for _, t := range teams {
			teamName[t.ID] = t.Name
		}
	}

	out := `<table class="admin-table admin-sup-table">` +
		`<thead><tr>` +
		`<th class="admin-th-id">#</th><th>Команда</th>` +
		`<th>Обращение</th><th>Статус</th><th class="admin-th-act"></th>` +
		`</tr></thead><tbody>`

	for _, req := range reqs {
		name := teamName[req.TeamID]
		if name == "" {
			name = fmt.Sprintf("команда #%d", req.TeamID)
		}

		who := `<div class="admin-td-name">` + html.EscapeString(name) + `</div>` +
			`<div class="admin-sup-when">` +
			req.Timestamp.Local().Format("02.01 15:04") + `</div>`
		if req.Type != "" {
			who += `<div class="admin-sup-type">` +
				html.EscapeString(req.Type) + `</div>`
		}

		body := adminSupportTextHTML(req.Text)
		var meta string
		if c := adminSupportContactHTML(req.Contact); c != "" {
			meta += `<span class="admin-sup-meta">контакт: ` + c + `</span>`
		}
		if req.Attach != "" {
			meta += `<span class="admin-sup-meta">вложение: ` +
				`<a class="admin-link" href="/admin/support/file?id=` +
				strconv.Itoa(req.ID) + `">` +
				html.EscapeString(req.Attach) + `</a></span>`
		}
		if meta != "" {
			body += `<div class="admin-sup-links">` + meta + `</div>`
		}

		var stateBadge, doneLabel, doneVal string
		if req.Done {
			stateBadge = `<span class="admin-status admin-status-closed">обработано</span>`
			doneLabel, doneVal = "вернуть", "0"
		} else {
			stateBadge = `<span class="admin-status admin-status-open">новое</span>`
			doneLabel, doneVal = "обработано", "1"
		}

		state := stateBadge
		if tg := adminSupportTgStatusHTML(req.TgStatus); tg != "" {
			state += `<div class="admin-sup-tg">` + tg + `</div>`
		}

		rowClass := ""
		if !req.Done {
			rowClass = ` class="admin-sup-new"`
		}

		actions := `<form method="post" action="/admin/support/done" class="admin-inline-form">` +
			adminHidden("csrf", csrf) +
			adminHidden("id", strconv.Itoa(req.ID)) +
			adminHidden("done", doneVal) +
			`<button class="admin-link" type="submit">` + doneLabel + `</button>` +
			`</form>` +
			`<form method="post" action="/admin/support/resend" class="admin-inline-form">` +
			adminHidden("csrf", csrf) +
			adminHidden("id", strconv.Itoa(req.ID)) +
			`<button class="admin-link" type="submit">в telegram</button>` +
			`</form>` +
			`<form method="post" action="/admin/support/delete" class="admin-inline-form">` +
			adminHidden("csrf", csrf) +
			adminHidden("id", strconv.Itoa(req.ID)) +
			`<button class="admin-link admin-link-del" type="submit">удалить</button>` +
			`</form>`

		out += `<tr` + rowClass + `>` +
			`<td class="admin-td-id">` + strconv.Itoa(req.ID) + `</td>` +
			`<td class="admin-sup-who">` + who + `</td>` +
			`<td class="admin-td-sup">` + body + `</td>` +
			`<td class="admin-sup-state">` + state + `</td>` +
			`<td class="admin-td-act">` + actions + `</td>` +
			`</tr>`
	}

	out += `</tbody></table>`
	return out
}

func adminSupportHTML(database *sql.DB, csrf string) string {
	cfg := tgLoadConfig(database)
	head := `<span class="admin-panel-title">Уведомления в Telegram</span>` +
		tgConfigStateHTML(cfg)
	return adminPanelPad(head, adminSupportSettingsHTML(cfg, csrf)) +
		adminTablePanel(adminPanelTitle("Обращения", "", ""),
			adminSupportListHTML(database, csrf))
}

func adminSupportSettingsHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		enabled := ""
		if r.FormValue("enabled") != "" {
			enabled = "1"
		}

		chatID := clampRunes(strings.TrimSpace(r.FormValue("chat_id")), 64)

		token := strings.TrimSpace(r.FormValue("token"))
		clearToken := r.FormValue("clear_token") != ""

		if token != "" && (len(token) < 20 || !strings.Contains(token, ":")) {
			adminErrorPage(w, "Токен бота выглядит некорректно. "+
				"Он должен иметь вид 123456789:AA...")
			return
		}

		if err := db.SetSetting(database, tgSettingEnabled, enabled); err != nil {
			log.Println("[admin] save tg enabled:", err)
			adminErrorPage(w, "Не удалось сохранить настройки")
			return
		}

		if err := db.SetSetting(database, tgSettingChatID, chatID); err != nil {
			log.Println("[admin] save tg chat id:", err)
			adminErrorPage(w, "Не удалось сохранить настройки")
			return
		}

		if clearToken {
			token = ""
		}
		if token != "" || clearToken {
			if err := db.SetSetting(database, tgSettingToken,
				token); err != nil {
				log.Println("[admin] save tg token:", err)
				adminErrorPage(w, "Не удалось сохранить настройки")
				return
			}
		}

		log.Printf("[admin] telegram settings updated by %s "+
			"(enabled=%q, chat_id set=%v, token changed=%v)",
			adminClientIP(r), enabled, chatID != "",
			token != "" || clearToken)

		http.Redirect(w, r, "/admin.html?tab=support", http.StatusSeeOther)
	}
}

func adminSupportTestHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		cfg := tgLoadConfig(database)
		if cfg.Token == "" || cfg.ChatID == "" {
			adminErrorPage(w, "Сначала сохраните токен бота и chat_id")
			return
		}

		err := tgSendMessage(cfg.Token, cfg.ChatID,
			"Тестовое сообщение платформы ЭТАЛОН CTF. "+
				"Уведомления об обращениях настроены.")
		if err != nil {
			log.Printf("[admin] tg test send failed: %v", err)
			adminErrorPage(w, "Не удалось отправить сообщение: "+
				err.Error())
			return
		}

		log.Printf("[admin] tg test message sent by %s", adminClientIP(r))

		content := `<div class="admin-section">` +
			`<h1 class="admin-page-title">Telegram настроен</h1>` +
			`<div class="admin-note">Тестовое сообщение отправлено. ` +
			`Проверьте личные сообщения от бота.</div>` +
			`<div class="admin-form-actions">` +
			`<a class="admin-btn admin-btn-primary" href="/admin.html?tab=support">Вернуться в панель</a>` +
			`</div></div>`
		adminRenderPage(w, "Telegram настроен", content)
	}
}

func adminSupportResendHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор обращения")
			return
		}

		if _, err := db.GetSupportRequest(database, id); err != nil {
			adminErrorPage(w, "Обращение не найдено")
			return
		}

		tgEnqueue(database, id)

		log.Printf("[admin] support %d re-enqueued to telegram by %s",
			id, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=support", http.StatusSeeOther)
	}
}

func adminSupportDoneHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор обращения")
			return
		}

		done := r.FormValue("done") == "1"

		if err := db.SetSupportDone(database, id, done); err != nil {
			log.Println("[admin] support done:", err)
			adminErrorPage(w, "Не удалось обновить статус обращения")
			return
		}

		log.Printf("[admin] support %d done=%v by %s",
			id, done, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=support", http.StatusSeeOther)
	}
}

func adminSupportDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор обращения")
			return
		}

		if _, err := db.GetSupportRequest(database, id); err != nil {
			adminErrorPage(w, "Обращение не найдено")
			return
		}

		if err := db.DeleteSupportRequest(database, id); err != nil {
			log.Println("[admin] delete support:", err)
			adminErrorPage(w, "Не удалось удалить обращение")
			return
		}

		if SupportFiles != "" {
			dir := filepath.Join(SupportFiles, strconv.Itoa(id))
			if err := os.RemoveAll(dir); err != nil {
				log.Println("[admin] delete support files:", err)
			}
		}

		log.Printf("[admin] support %d DELETED by %s", id, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=support", http.StatusSeeOther)
	}
}

func adminSupportFileHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := adminGetSession(r)
		if s == nil {
			http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
			return
		}

		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		req, err := db.GetSupportRequest(database, id)
		if err != nil || req.Attach == "" {
			http.NotFound(w, r)
			return
		}

		name := sanitizeAttachName(req.Attach)
		path := supportAttachPath(req.ID, name)
		if name == "" || path == "" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Disposition",
			`attachment; filename="`+name+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, path)
	}
}
