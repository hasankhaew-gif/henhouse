package scoreboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/csv"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jollheef/henhouse/db"
)

const (
	adminCookieName     = "admin_session"
	adminSessionTTL     = 4 * time.Hour
	adminMinTokenLen    = 16
	adminMaxFails       = 5
	adminFailWindow     = 5 * time.Minute
	adminLoginBaseDelay = time.Second
)

var (
	adminTokenHash []byte

	adminSessionsMtx sync.Mutex
	adminSessions    = map[string]*adminSession{}

	adminFailsMtx sync.Mutex
	adminFails    = map[string]*adminFailCounter{}
)

type adminSession struct {
	Expires time.Time
	CSRF    string
}

type adminFailCounter struct {
	Count       int
	WindowStart time.Time
}

func EnableAdmin(token string) {
	if len(token) < adminMinTokenLen {
		log.Printf("[admin] token shorter than %d characters, admin panel DISABLED",
			adminMinTokenLen)
		return
	}
	sum := sha256.Sum256([]byte(token))
	adminTokenHash = sum[:]
	log.Println("[admin] web admin panel enabled")
}

func adminEnabled() bool { return adminTokenHash != nil }

func adminClientIP(r *http.Request) string {
	addr := getClientAddr(r)
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func adminSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy",
		"default-src 'none'; style-src 'self' https://fonts.googleapis.com; "+
			"font-src https://fonts.gstatic.com; img-src 'self'; "+
			"base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
}

func adminCheckToken(token string) bool {
	if !adminEnabled() {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], adminTokenHash) == 1
}

func adminRateLimited(ip string) bool {
	adminFailsMtx.Lock()
	defer adminFailsMtx.Unlock()
	fc, ok := adminFails[ip]
	if !ok {
		return false
	}
	if time.Since(fc.WindowStart) > adminFailWindow {
		delete(adminFails, ip)
		return false
	}
	return fc.Count >= adminMaxFails
}

func adminRegisterFail(ip string) {
	adminFailsMtx.Lock()
	defer adminFailsMtx.Unlock()
	fc, ok := adminFails[ip]
	if !ok || time.Since(fc.WindowStart) > adminFailWindow {
		adminFails[ip] = &adminFailCounter{Count: 1, WindowStart: time.Now()}
		return
	}
	fc.Count++
}

func adminNewSession() (id, csrf string, err error) {
	id, err = genSession()
	if err != nil {
		return
	}
	csrf, err = genSession()
	if err != nil {
		return
	}
	csrf = csrf[:64]

	adminSessionsMtx.Lock()
	defer adminSessionsMtx.Unlock()

	now := time.Now()
	for k, s := range adminSessions {
		if now.After(s.Expires) {
			delete(adminSessions, k)
		}
	}

	adminSessions[id] = &adminSession{
		Expires: now.Add(adminSessionTTL),
		CSRF:    csrf,
	}
	return
}

func adminGetSession(r *http.Request) *adminSession {
	c, err := r.Cookie(adminCookieName)
	if err != nil {
		return nil
	}

	adminSessionsMtx.Lock()
	defer adminSessionsMtx.Unlock()

	s, ok := adminSessions[c.Value]
	if !ok {
		return nil
	}
	if time.Now().After(s.Expires) {
		delete(adminSessions, c.Value)
		return nil
	}
	return s
}

func adminDropSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminCookieName); err == nil {
		adminSessionsMtx.Lock()
		delete(adminSessions, c.Value)
		adminSessionsMtx.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   underProxy,
		SameSite: http.SameSiteStrictMode,
	})
}

func adminCheckCSRF(r *http.Request, s *adminSession) bool {
	got := r.FormValue("csrf")
	return got != "" &&
		subtle.ConstantTimeCompare([]byte(got), []byte(s.CSRF)) == 1
}

func adminStatusRu() string {
	switch contestStatus {
	case contestNotStarted:
		return "не начато"
	case contestRunning:
		return "идёт"
	case contestCompleted:
		return "завершено"
	}
	return "неизвестно"
}

var adminTabDefs = []struct {
	ID    string
	Label string
}{
	{"overview", "Обзор"},
	{"game", "Игра"},
	{"tasks", "Задания"},
	{"teams", "Команды"},
	{"cats", "Категории"},
	{"news", "Новости"},
	{"support", "Поддержка"},
	{"events", "События"},
}

func adminTabsHTML(active string, newSupport int) string {
	var b strings.Builder
	for _, t := range adminTabDefs {
		cls := "admin-tab"
		if t.ID == active {
			cls += " admin-tab-active"
		}
		b.WriteString(`<a class="` + cls + `" href="/admin.html?tab=` +
			t.ID + `">` + t.Label)
		if t.ID == "support" && newSupport > 0 {
			b.WriteString(`<span class="admin-tab-badge">` +
				strconv.Itoa(newSupport) + `</span>`)
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`<a class="admin-tab admin-tab-site" href="/index.html">на сайт &rarr;</a>`)
	return b.String()
}

func adminPageHead(title, actions string) string {
	out := `<div class="admin-page-head"><h1 class="admin-page-h">` +
		title + `</h1>`
	if actions != "" {
		out += `<div class="admin-page-actions">` + actions + `</div>`
	}
	return out + `</div>`
}

func adminPanel(head, body string) string {
	out := `<div class="admin-panel">`
	if head != "" {
		out += `<div class="admin-panel-head">` + head + `</div>`
	}
	return out + body + `</div>`
}

func adminPanelPad(head, body string) string {
	return adminPanel(head, `<div class="admin-panel-pad">`+body+`</div>`)
}

func adminTablePanel(head, table string) string {
	return adminPanel(head, `<div class="table-scroll-wrap">`+table+`</div>`)
}

func adminPanelTitle(title, href, label string) string {
	out := `<span class="admin-panel-title">` + title + `</span>`
	if href != "" {
		out += `<a class="admin-link admin-panel-more" href="` + href +
			`">` + label + ` &rarr;</a>`
	}
	return out
}

func adminProgressRowHTML() string {
	now := time.Now()
	pct := 0
	if now.After(gameShim.Start) {
		span := gameShim.End.Sub(gameShim.Start)
		if span > 0 {
			pct = int(float64(now.Sub(gameShim.Start)) / float64(span) * 100)
		}
		if pct > 100 {
			pct = 100
		}
	}
	return fmt.Sprintf(
		`<div class="admin-progress-row">`+
			`<progress class="admin-progress" value="%d" max="100"></progress>`+
			`<span class="admin-progress-lbl">%d%% времени</span>`+
			`</div>`,
		pct, pct)
}

func adminTopTeamsHTML() string {
	if len(scoreCache) == 0 {
		return `<div class="admin-empty">Команд пока нет</div>`
	}

	out := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th class="admin-th-id">#</th><th>Команда</th>` +
		`<th class="admin-th-num">Решено</th><th class="admin-th-num">Очки</th>` +
		`</tr></thead><tbody>`

	for i, ts := range scoreCache {
		if i >= 5 {
			break
		}
		solved := 0
		if solvedCountCache != nil {
			solved = solvedCountCache[ts.ID]
		}
		out += fmt.Sprintf(
			`<tr>`+
				`<td class="admin-td-id">%d</td>`+
				`<td class="admin-td-name">%s</td>`+
				`<td class="admin-td-num">%d</td>`+
				`<td class="admin-td-num admin-td-score">%d</td>`+
				`</tr>`,
			i+1, html.EscapeString(ts.Name), solved, ts.Score)
	}

	return out + `</tbody></table>`
}

func adminOverviewHTML(database *sql.DB) string {
	teams, _ := db.GetTeams(database)
	realTeams := 0
	for _, t := range teams {
		if !t.Test {
			realTeams++
		}
	}

	cats, err := gameShim.Tasks()
	if err != nil {
		log.Println("[admin] tasks:", err)
	}
	total, opened, solves := 0, 0, 0
	for _, c := range cats {
		for _, t := range c.TasksInfo {
			total++
			if t.Opened {
				opened++
			}
			solves += len(t.SolvedBy)
		}
	}

	var statusClass string
	switch contestStatus {
	case contestRunning:
		statusClass = "admin-st-run"
	case contestCompleted:
		statusClass = "admin-st-done"
	default:
		statusClass = "admin-st-wait"
	}

	newSupport, err := db.CountNewSupportRequests(database)
	if err != nil {
		log.Println("[admin] count support:", err)
	}

	card := func(href, label, val, valClass string) string {
		if href == "" {
			return `<div class="admin-card">` +
				`<span class="admin-card-k">` + label + `</span>` +
				`<span class="admin-card-v ` + valClass + `">` + val + `</span>` +
				`</div>`
		}
		return `<a class="admin-card" href="` + href + `">` +
			`<span class="admin-card-k">` + label + `</span>` +
			`<span class="admin-card-v ` + valClass + `">` + val + `</span>` +
			`</a>`
	}

	supportClass := ""
	if newSupport > 0 {
		supportClass = "admin-card-hot"
	}

	out := `<div class="admin-cards">` +
		card("", "статус", adminStatusRu(), statusClass) +
		card("/admin.html?tab=teams", "команд",
			fmt.Sprintf("%d", realTeams), "") +
		card("/admin.html?tab=tasks", "заданий открыто",
			fmt.Sprintf("%d&thinsp;/&thinsp;%d", opened, total), "") +
		card("/admin.html?tab=events", "решений",
			fmt.Sprintf("%d", solves), "") +
		card("/admin.html?tab=support", "новых обращений",
			fmt.Sprintf("%d", newSupport), supportClass) +
		`</div>`

	out += `<div class="admin-panel admin-ov-time">` +
		`<div class="admin-ov-when">` +
		`<span class="admin-card-k">старт</span>` +
		`<span class="admin-ov-date">` +
		gameShim.Start.Local().Format("02.01 15:04") + `</span>` +
		`</div>` +
		adminProgressRowHTML() +
		`<div class="admin-ov-when admin-ov-when-end">` +
		`<span class="admin-card-k">финиш</span>` +
		`<span class="admin-ov-date">` +
		gameShim.End.Local().Format("02.01 15:04") + `</span>` +
		`</div>` +
		`</div>`

	out += `<div class="admin-ov-grid">` +
		adminTablePanel(
			adminPanelTitle("Лучшие команды",
				"/admin.html?tab=teams", "все команды"),
			adminTopTeamsHTML()) +
		adminTablePanel(
			adminPanelTitle("Последние события",
				"/admin.html?tab=events", "вся лента"),
			adminEventsTableHTML(database, 8, false)) +
		`</div>`

	return out
}

func adminTasksHTML(csrf string) string {
	cats, err := gameShim.Tasks()
	if err != nil {
		return `<div class="admin-empty">Не удалось получить задания</div>`
	}

	out := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th class="admin-th-id">#</th><th>Задание</th><th>Категория</th>` +
		`<th class="admin-th-num">Уровень</th>` +
		`<th class="admin-th-num">Цена</th><th class="admin-th-num">Решили</th>` +
		`<th>Статус</th><th class="admin-th-act"></th>` +
		`</tr></thead><tbody>`

	rows := 0
	for _, cat := range cats {
		for _, t := range cat.TasksInfo {
			rows++
			var status, statusClass, action, actionLabel string
			if t.Opened {
				status, statusClass = "открыто", "admin-status-open"
				action, actionLabel = "close", "закрыть"
			} else if t.ForceClosed {
				status, statusClass = "закрыто принуд.", "admin-status-forced"
				action, actionLabel = "open", "открыть"
			} else {
				status, statusClass = "закрыто", "admin-status-closed"
				action, actionLabel = "open", "открыть"
			}

			out += fmt.Sprintf(
				`<tr>`+
					`<td class="admin-td-id">%d</td>`+
					`<td class="admin-td-name"><a class="admin-rowlink" href="/admin/task.html?id=%d">%s</a></td>`+
					`<td class="admin-td-dim">%s</td>`+
					`<td class="admin-td-num">%d</td>`+
					`<td class="admin-td-num">%d</td>`+
					`<td class="admin-td-num">%d</td>`+
					`<td><span class="admin-status %s">%s</span></td>`+
					`<td class="admin-td-act">`+
					`<form method="post" action="/admin/action" class="admin-inline-form">`+
					`<input type="hidden" name="csrf" value="%s">`+
					`<input type="hidden" name="id" value="%d">`+
					`<input type="hidden" name="do" value="%s">`+
					`<button class="admin-link" type="submit">%s</button>`+
					`</form>`+
					`<a class="admin-link" href="/admin/task.html?id=%d">изменить</a>`+
					`</td>`+
					`</tr>`,
				t.ID, t.ID, html.EscapeString(t.Name),
				html.EscapeString(cat.Name),
				t.Level, t.Price, len(t.SolvedBy),
				statusClass, status,
				html.EscapeString(csrf), t.ID, action, actionLabel,
				t.ID)
		}
	}

	out += `</tbody></table>`
	if rows == 0 {
		out += `<div class="admin-empty">Заданий пока нет</div>`
	}
	return out
}

func adminTeamsHTML(database *sql.DB) string {
	teams, err := db.GetTeams(database)
	if err != nil {
		return `<div class="admin-empty">Не удалось получить команды</div>`
	}

	rank := map[int]int{}
	score := map[int]int{}
	for i, t := range scoreCache {
		rank[t.ID] = i + 1
		score[t.ID] = t.Score
	}

	sort.SliceStable(teams, func(i, j int) bool {
		ri, iok := rank[teams[i].ID]
		rj, jok := rank[teams[j].ID]
		if iok && jok {
			return ri < rj
		}
		if iok != jok {
			return iok
		}
		return teams[i].ID < teams[j].ID
	})

	out := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th class="admin-th-id">#</th><th>Команда</th><th>Школа</th><th>Email</th>` +
		`<th class="admin-th-num">Место</th><th class="admin-th-num">Решено</th>` +
		`<th class="admin-th-num">Очки</th><th class="admin-th-act"></th>` +
		`</tr></thead><tbody>`

	for _, t := range teams {
		solved := 0
		if solvedCountCache != nil {
			solved = solvedCountCache[t.ID]
		}

		name := html.EscapeString(t.Name)
		if t.Test {
			name += ` <span class="admin-test-badge">тест</span>`
		}

		place := "&ndash;"
		if r, ok := rank[t.ID]; ok {
			place = fmt.Sprintf("%d", r)
		}

		out += fmt.Sprintf(
			`<tr>`+
				`<td class="admin-td-id">%d</td>`+
				`<td class="admin-td-name"><a class="admin-rowlink" href="/admin/team.html?id=%d">%s</a></td>`+
				`<td class="admin-td-dim">%s</td>`+
				`<td class="admin-td-dim">%s</td>`+
				`<td class="admin-td-num">%s</td>`+
				`<td class="admin-td-num">%d</td>`+
				`<td class="admin-td-num admin-td-score">%d</td>`+
				`<td class="admin-td-act">`+
				`<a class="admin-link" href="/admin/team.html?id=%d">изменить</a>`+
				`</td>`+
				`</tr>`,
			t.ID, t.ID, name, html.EscapeString(t.Desc),
			html.EscapeString(t.Email), place, solved, score[t.ID],
			t.ID)
	}

	out += `</tbody></table>`
	return out
}

func adminCategoriesHTML(database *sql.DB, csrf string) string {
	cats, err := db.GetCategories(database)
	if err != nil {
		return `<div class="admin-empty">Не удалось получить категории</div>`
	}

	tasks, _ := db.GetTasks(database)
	countByCat := map[int]int{}
	for _, t := range tasks {
		countByCat[t.CategoryID]++
	}

	form := `<form method="post" action="/admin/category" class="admin-addcat">` +
		`<input type="hidden" name="csrf" value="` + html.EscapeString(csrf) + `">` +
		`<input class="admin-input" type="text" name="name" placeholder="Название новой категории"` +
		` autocomplete="off" spellcheck="false" required>` +
		`<button class="admin-btn admin-btn-primary" type="submit">Добавить</button>` +
		`</form>`

	out := adminPanelPad(
		adminPanelTitle("Новая категория", "", ""), form)

	if len(cats) == 0 {
		out += adminPanel("",
			`<div class="admin-empty">Категорий пока нет</div>`)
		return out
	}

	table := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th class="admin-th-id">#</th><th>Категория</th>` +
		`<th class="admin-th-num">Заданий</th><th class="admin-th-act"></th>` +
		`</tr></thead><tbody>`

	for _, c := range cats {
		cnt := countByCat[c.ID]
		act := `<span class="admin-td-dim">есть задания</span>`
		if cnt == 0 {
			act = fmt.Sprintf(
				`<form method="post" action="/admin/category/delete" class="admin-inline-form">`+
					`<input type="hidden" name="csrf" value="%s">`+
					`<input type="hidden" name="id" value="%d">`+
					`<button class="admin-link admin-link-del" type="submit">удалить</button>`+
					`</form>`,
				html.EscapeString(csrf), c.ID)
		}

		table += fmt.Sprintf(
			`<tr>`+
				`<td class="admin-td-id">%d</td>`+
				`<td class="admin-td-name">%s</td>`+
				`<td class="admin-td-num">%d</td>`+
				`<td class="admin-td-act">%s</td>`+
				`</tr>`,
			c.ID, html.EscapeString(c.Name), cnt, act)
	}

	table += `</tbody></table>`
	return out + adminTablePanel("", table)
}

const adminEventsLimit = 50

func adminEventsTableHTML(database *sql.DB, limit int, withFlag bool) string {
	flags, err := db.GetLastFlags(database, limit)
	if err != nil {
		return `<div class="admin-empty">Не удалось получить события</div>`
	}

	if len(flags) == 0 {
		return `<div class="admin-empty">Попыток сдачи флагов пока не было</div>`
	}

	teamName := map[int]string{}
	if teams, terr := db.GetTeams(database); terr == nil {
		for _, t := range teams {
			teamName[t.ID] = t.Name
		}
	}

	taskName := map[int]string{}
	if tasks, terr := db.GetTasks(database); terr == nil {
		for _, t := range tasks {
			taskName[t.ID] = t.Name
		}
	}

	nameOr := func(m map[int]string, id int, kind string) string {
		if n, ok := m[id]; ok {
			return html.EscapeString(n)
		}
		return fmt.Sprintf("%s #%d", kind, id)
	}

	out := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th>Время</th><th>Команда</th><th>Задание</th>`
	if withFlag {
		out += `<th>Сдано</th>`
	}
	out += `<th>Результат</th></tr></thead><tbody>`

	for _, f := range flags {
		verdict := `<span class="admin-status admin-status-forced">неверно</span>`
		if f.Solved {
			verdict = `<span class="admin-status admin-status-open">принят</span>`
		}

		out += fmt.Sprintf(
			`<tr>`+
				`<td class="admin-td-mono">%s</td>`+
				`<td class="admin-td-name">%s</td>`+
				`<td class="admin-td-dim">%s</td>`,
			f.Timestamp.Local().Format("02.01 15:04:05"),
			nameOr(teamName, f.TeamID, "команда"),
			nameOr(taskName, f.TaskID, "задание"))

		if withFlag {
			sub := f.Flag
			if len([]rune(sub)) > 48 {
				sub = string([]rune(sub)[:48]) + "…"
			}
			out += `<td class="admin-td-mono admin-td-flag">` +
				html.EscapeString(sub) + `</td>`
		}

		out += `<td>` + verdict + `</td></tr>`
	}

	return out + `</tbody></table>`
}

func adminEventsTabHTML(database *sql.DB) string {
	out := adminTablePanel("",
		adminEventsTableHTML(database, adminEventsLimit, true))

	if total, cerr := db.CountFlags(database); cerr == nil && total > adminEventsLimit {
		out += fmt.Sprintf(
			`<div class="admin-note admin-events-note">Показаны последние %d из %d попыток</div>`,
			adminEventsLimit, total)
	}

	return out
}

func adminExportHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := adminGetSession(r)
		if s == nil {
			http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
			return
		}

		teams, err := db.GetTeams(database)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		teamByID := map[int]db.Team{}
		for _, t := range teams {
			teamByID[t.ID] = t
		}

		solvedNames := map[int][]string{}
		if cats, terr := gameShim.Tasks(); terr == nil {
			for _, c := range cats {
				for _, t := range c.TasksInfo {
					for _, tid := range t.SolvedBy {
						solvedNames[tid] = append(solvedNames[tid], t.Name)
					}
				}
			}
		}

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition",
			`attachment; filename="etalon-ctf-results.csv"`)
		w.Header().Set("Cache-Control", "no-store")

		w.Write([]byte{0xEF, 0xBB, 0xBF})

		cw := csv.NewWriter(w)
		cw.Comma = ';'

		cw.Write([]string{"Место", "Команда", "Школа", "Email",
			"Очки", "Решено задач", "Решённые задания"})

		for i, ts := range scoreCache {
			t := teamByID[ts.ID]
			names := solvedNames[ts.ID]
			cw.Write([]string{
				strconv.Itoa(i + 1),
				ts.Name,
				ts.Desc,
				t.Email,
				strconv.Itoa(ts.Score),
				strconv.Itoa(len(names)),
				strings.Join(names, ", "),
			})
		}

		cw.Flush()

		log.Printf("[admin] results exported as CSV by %s", adminClientIP(r))
	}
}

func adminNewsListHTML(database *sql.DB) string {
	news, err := db.GetNews(database)
	if err != nil {
		return `<div class="admin-empty">Не удалось получить новости</div>`
	}

	if len(news) == 0 {
		return `<div class="admin-empty">Новостей пока нет. Участники видят новости в своём профиле</div>`
	}

	out := `<table class="admin-table">` +
		`<thead><tr>` +
		`<th>Дата</th><th>Тег</th><th>Заголовок</th><th class="admin-th-act"></th>` +
		`</tr></thead><tbody>`

	for _, n := range news {
		out += fmt.Sprintf(
			`<tr>`+
				`<td class="admin-td-mono">%s</td>`+
				`<td class="admin-td-dim">%s</td>`+
				`<td class="admin-td-name"><a class="admin-rowlink" href="/admin/news.html?id=%d">%s</a></td>`+
				`<td class="admin-td-act">`+
				`<a class="admin-link" href="/admin/news.html?id=%d">изменить</a>`+
				`</td>`+
				`</tr>`,
			n.Timestamp.Local().Format("02.01 15:04"),
			html.EscapeString(n.Tag),
			n.ID, html.EscapeString(n.Title),
			n.ID)
	}

	out += `</tbody></table>`
	return out
}

func adminPageHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminGetSession(r)
		if s == nil {
			tmpl, err := getTmpl("admin_login")
			if err != nil {
				log.Println("[admin] login tmpl:", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, tmpl, "")
			return
		}

		tmpl, err := getTmpl("admin")
		if err != nil {
			log.Println("[admin] tmpl:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		tab := r.URL.Query().Get("tab")
		known := false
		for _, t := range adminTabDefs {
			if t.ID == tab {
				known = true
				break
			}
		}
		if !known {
			tab = "overview"
		}

		newSupport, err := db.CountNewSupportRequests(database)
		if err != nil {
			log.Println("[admin] count support:", err)
		}

		var content string
		switch tab {
		case "game":
			content = adminPageHead("Игра", "") +
				adminPanelPad(adminPanelTitle("Время игры", "", ""),
					adminGameHTML(s.CSRF))
		case "tasks":
			content = adminPageHead("Задания",
				`<a class="admin-btn admin-btn-primary" href="/admin/task.html">+ Задание</a>`) +
				adminTablePanel("", adminTasksHTML(s.CSRF))
		case "teams":
			content = adminPageHead("Команды",
				`<a class="admin-btn" href="/admin/export.csv">Экспорт CSV</a>`+
					`<a class="admin-btn admin-btn-primary" href="/admin/team.html">+ Команда</a>`) +
				adminTablePanel("", adminTeamsHTML(database))
		case "cats":
			content = adminPageHead("Категории", "") +
				adminCategoriesHTML(database, s.CSRF)
		case "news":
			content = adminPageHead("Новости",
				`<a class="admin-btn admin-btn-primary" href="/admin/news.html">+ Новость</a>`) +
				adminTablePanel("", adminNewsListHTML(database))
		case "support":
			content = adminPageHead("Поддержка", "") +
				adminSupportHTML(database, s.CSRF)
		case "events":
			content = adminPageHead("События", "") +
				adminEventsTabHTML(database)
		default:
			content = adminOverviewHTML(database)
		}

		fmt.Fprintf(w, tmpl,
			getInfo(),
			html.EscapeString(s.CSRF),
			adminTabsHTML(tab, newSupport),
			content)
	}
}

func adminAuthHandler(w http.ResponseWriter, r *http.Request) {
	adminSecurityHeaders(w)

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
		return
	}

	ip := adminClientIP(r)

	if adminRateLimited(ip) {
		log.Printf("[admin] rate-limited login attempt from %s", ip)
		http.Error(w, "too many attempts, try later",
			http.StatusTooManyRequests)
		return
	}

	time.Sleep(adminLoginBaseDelay)

	if !adminCheckToken(r.FormValue("token")) {
		adminRegisterFail(ip)
		log.Printf("[admin] FAILED login from %s", ip)

		tmpl, err := getTmpl("admin_login")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, tmpl,
			`<div class="admin-login-error">Неверный токен</div>`)
		return
	}

	id, _, err := adminNewSession()
	if err != nil {
		log.Println("[admin] session:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   underProxy,
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("[admin] successful login from %s", ip)
	http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
}

func adminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	adminSecurityHeaders(w)

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
		return
	}

	s := adminGetSession(r)
	if s == nil || !adminCheckCSRF(r, s) {
		http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
		return
	}

	adminDropSession(w, r)
	log.Printf("[admin] logout from %s", adminClientIP(r))
	http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
}

func adminActionHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
			return
		}

		s := adminGetSession(r)
		if s == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !adminCheckCSRF(r, s) {
			log.Printf("[admin] CSRF check failed from %s", adminClientIP(r))
			http.Error(w, "bad csrf token", http.StatusForbidden)
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		do := r.FormValue("do")
		if do != "open" && do != "close" {
			http.Error(w, "bad action", http.StatusBadRequest)
			return
		}

		task, err := db.GetTask(database, id)
		if err != nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}

		if do == "open" {
			task.Opened = true
			task.ForceClosed = false
			task.OpenedTime = time.Now()
		} else {
			task.Opened = false

			task.ForceClosed = true
		}

		if err := db.UpdateTask(database, &task); err != nil {
			log.Println("[admin] update task:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Printf("[admin] task %d (%s) %s by %s",
			task.ID, task.Name, do, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=tasks", http.StatusSeeOther)
	}
}
