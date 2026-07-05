package scoreboard

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jollheef/henhouse/db"
)

const (
	teamTokenLen     = 16
	maxUploadBytes   = 64 << 20
	maxAttachNameLen = 80
)

func genTeamToken() (string, error) {
	buf := make([]byte, teamTokenLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

func adminRequireSession(w http.ResponseWriter, r *http.Request) *adminSession {
	s := adminGetSession(r)
	if s == nil {
		http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
	}
	return s
}

func adminRequirePostSession(w http.ResponseWriter, r *http.Request) *adminSession {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin.html", http.StatusSeeOther)
		return nil
	}

	s := adminGetSession(r)
	if s == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	if !adminCheckCSRF(r, s) {
		log.Printf("[admin] CSRF check failed from %s", adminClientIP(r))
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return nil
	}

	return s
}

func adminRenderPage(w http.ResponseWriter, title, content string) {
	tmpl, err := getTmpl("admin_page")
	if err != nil {
		log.Println("[admin] page tmpl:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, tmpl, html.EscapeString(title), getInfo(), content)
}

func adminErrorPage(w http.ResponseWriter, msg string) {
	content := `<div class="admin-section">` +
		`<div class="admin-form-error">` + html.EscapeString(msg) + `</div>` +
		`<a class="admin-link" href="/admin.html">&larr; Вернуться в панель</a>` +
		`</div>`
	w.WriteHeader(http.StatusBadRequest)
	adminRenderPage(w, "Ошибка", content)
}

func adminInputRow(id, name, label, value, hint string, required bool) string {
	req := ""
	if required {
		req = " required"
	}
	hintHTML := ""
	if hint != "" {
		hintHTML = `<div class="admin-form-hint">` +
			html.EscapeString(hint) + `</div>`
	}
	return `<div class="admin-form-row">` +
		`<label class="admin-label" for="` + id + `">` +
		html.EscapeString(label) + `</label>` +
		`<input class="admin-input" type="text" id="` + id +
		`" name="` + name + `" value="` + html.EscapeString(value) +
		`" autocomplete="off" spellcheck="false"` + req + `>` +
		hintHTML +
		`</div>`
}

func adminTextareaRow(id, name, label, value, hint string) string {
	hintHTML := ""
	if hint != "" {
		hintHTML = `<div class="admin-form-hint">` +
			html.EscapeString(hint) + `</div>`
	}
	return `<div class="admin-form-row admin-form-row-wide">` +
		`<label class="admin-label" for="` + id + `">` +
		html.EscapeString(label) + `</label>` +
		`<textarea class="admin-textarea" id="` + id + `" name="` + name +
		`" rows="4" spellcheck="false">` + html.EscapeString(value) +
		`</textarea>` +
		hintHTML +
		`</div>`
}

func adminHidden(name, value string) string {
	return `<input type="hidden" name="` + name + `" value="` +
		html.EscapeString(value) + `">`
}

func adminDeleteForm(formID, action, csrf string, id int) string {
	return `<form id="` + formID + `" method="post" action="` + action + `">` +
		adminHidden("csrf", csrf) +
		adminHidden("id", strconv.Itoa(id)) +
		`</form>`
}

func adminDeleteButton(formID, label string) string {
	return `<button class="admin-btn admin-btn-del" type="submit" form="` +
		formID + `">` + label + `</button>`
}

var attachNameRe = regexp.MustCompile(`[^0-9A-Za-zА-Яа-яЁё._-]+`)

func sanitizeAttachName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = attachNameRe.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._")
	if len(name) > maxAttachNameLen {
		name = name[len(name)-maxAttachNameLen:]
	}
	return name
}

func taskFilesDir(taskID int) string {
	return filepath.Join(filesPath, strconv.Itoa(taskID))
}

func saveTaskUploads(r *http.Request, taskID int) error {
	if r.MultipartForm == nil {
		return nil
	}
	for _, fh := range r.MultipartForm.File["files"] {
		name := sanitizeAttachName(fh.Filename)
		if name == "" {
			continue
		}

		if err := os.MkdirAll(taskFilesDir(taskID), 0755); err != nil {
			return err
		}

		src, err := fh.Open()
		if err != nil {
			return err
		}

		dst, err := os.Create(filepath.Join(taskFilesDir(taskID), name))
		if err != nil {
			src.Close()
			return err
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return err
		}

		log.Printf("[admin] task %d: file %q uploaded (%d bytes)",
			taskID, name, fh.Size)
	}
	return nil
}

func adminTaskFileDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор задания")
			return
		}

		name := sanitizeAttachName(r.FormValue("name"))
		if name == "" {
			adminErrorPage(w, "Некорректное имя файла")
			return
		}

		if err := os.Remove(filepath.Join(taskFilesDir(id), name)); err != nil {
			log.Println("[admin] remove file:", err)
			adminErrorPage(w, "Не удалось удалить файл")
			return
		}

		log.Printf("[admin] task %d: file %q deleted by %s",
			id, name, adminClientIP(r))

		http.Redirect(w, r, "/admin/task.html?id="+strconv.Itoa(id),
			http.StatusSeeOther)
	}
}

const adminTimeFormat = "2006-01-02T15:04"

func adminGameHTML(csrf string) string {
	return `<form method="post" action="/admin/game" class="admin-game-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("action", "set") +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="g-start">Начало</label>` +
		`<input class="admin-input" type="datetime-local" id="g-start" name="start" value="` +
		gameShim.Start.Local().Format(adminTimeFormat) + `" required>` +
		`</div>` +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="g-end">Конец</label>` +
		`<input class="admin-input" type="datetime-local" id="g-end" name="end" value="` +
		gameShim.End.Local().Format(adminTimeFormat) + `" required>` +
		`</div>` +
		`<button class="admin-btn admin-btn-primary" type="submit">Сохранить</button>` +
		`</form>` +
		`<div class="admin-quick-actions">` +
		`<form method="post" action="/admin/game" class="admin-inline-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("action", "start_now") +
		`<button class="admin-btn" type="submit">Начать сейчас</button>` +
		`</form>` +
		`<form method="post" action="/admin/game" class="admin-inline-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("action", "end_now") +
		`<button class="admin-btn admin-btn-del" type="submit">Завершить сейчас</button>` +
		`</form>` +
		`</div>`
}

func adminGameHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		start := gameShim.Start
		end := gameShim.End

		switch r.FormValue("action") {
		case "set":
			var err error
			start, err = time.ParseInLocation(adminTimeFormat,
				r.FormValue("start"), time.Local)
			if err != nil {
				adminErrorPage(w, "Некорректная дата начала")
				return
			}
			end, err = time.ParseInLocation(adminTimeFormat,
				r.FormValue("end"), time.Local)
			if err != nil {
				adminErrorPage(w, "Некорректная дата конца")
				return
			}
		case "start_now":
			start = time.Now()
		case "end_now":
			end = time.Now()
		default:
			adminErrorPage(w, "Неизвестное действие")
			return
		}

		if !end.After(start) {
			adminErrorPage(w, "Конец мероприятия должен быть позже начала")
			return
		}

		if err := db.SetSetting(database, "game_start",
			start.Format(time.RFC3339)); err != nil {
			log.Println("[admin] save game_start:", err)
			adminErrorPage(w, "Не удалось сохранить время начала")
			return
		}
		if err := db.SetSetting(database, "game_end",
			end.Format(time.RFC3339)); err != nil {
			log.Println("[admin] save game_end:", err)
			adminErrorPage(w, "Не удалось сохранить время конца")
			return
		}

		gameShim.Start = start
		gameShim.End = end

		log.Printf("[admin] game time set to %s .. %s by %s",
			start.Format(time.RFC3339), end.Format(time.RFC3339),
			adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=game", http.StatusSeeOther)
	}
}

func adminCategoryAddHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			adminErrorPage(w, "Название категории не может быть пустым")
			return
		}

		cats, err := db.GetCategories(database)
		if err == nil {
			for _, c := range cats {
				if strings.EqualFold(c.Name, name) {
					adminErrorPage(w, "Такая категория уже существует")
					return
				}
			}
		}

		cat := db.Category{Name: name}
		if err := db.AddCategory(database, &cat); err != nil {
			log.Println("[admin] add category:", err)
			adminErrorPage(w, "Не удалось создать категорию")
			return
		}

		log.Printf("[admin] category %d (%s) created by %s",
			cat.ID, cat.Name, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=cats", http.StatusSeeOther)
	}
}

func adminCategoryDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор категории")
			return
		}

		tasks, err := db.GetTasks(database)
		if err != nil {
			adminErrorPage(w, "Не удалось проверить задания категории")
			return
		}
		for _, t := range tasks {
			if t.CategoryID == id {
				adminErrorPage(w, "В категории есть задания. Сначала "+
					"перенесите или удалите их")
				return
			}
		}

		if err := db.DeleteCategory(database, id); err != nil {
			log.Println("[admin] delete category:", err)
			adminErrorPage(w, "Не удалось удалить категорию")
			return
		}

		log.Printf("[admin] category %d deleted by %s", id, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=cats", http.StatusSeeOther)
	}
}

func adminTaskFormHTML(database *sql.DB, task *db.Task, csrf string) string {
	isNew := task == nil
	if isNew {
		task = &db.Task{Level: 1}
	}

	cats, err := db.GetCategories(database)
	if err != nil {
		log.Println("[admin] categories:", err)
	}

	catSelect := `<select class="admin-select" id="f-category" name="category_id">`
	for _, c := range cats {
		sel := ""
		if c.ID == task.CategoryID {
			sel = " selected"
		}
		catSelect += fmt.Sprintf(`<option value="%d"%s>%s</option>`,
			c.ID, sel, html.EscapeString(c.Name))
	}
	catSelect += `</select>`

	status := "auto"
	if task.Opened {
		status = "open"
	} else if task.ForceClosed {
		status = "closed"
	}
	statusOpt := func(val, label string) string {
		sel := ""
		if status == val {
			sel = " selected"
		}
		return `<option value="` + val + `"` + sel + `>` + label +
			`</option>`
	}
	statusSelect := `<select class="admin-select" id="f-status" name="status">` +
		statusOpt("auto", "Закрыто (откроется автоматически)") +
		statusOpt("open", "Открыто") +
		statusOpt("closed", "Закрыто принудительно") +
		`</select>`

	flagHint := "По умолчанию сравнивается как точная строка, например ETALON{m0y_fl4g}"
	flagRequired := true
	if !isNew {
		flagHint = "Оставьте пустым, чтобы не менять текущий флаг"
		flagRequired = false
	}
	flagReq := ""
	if flagRequired {
		flagReq = " required"
	}

	title := "Новое задание"
	if !isNew {
		title = task.Name
	}

	filesBlock := `<div class="admin-form-row admin-form-row-wide">` +
		`<label class="admin-label" for="f-files">Файлы задания</label>`
	if !isNew {
		files := taskFiles(task.ID)
		if len(files) > 0 {
			filesBlock += `<div class="admin-attach-list">`
			for _, f := range files {
				delFormID := "del-file-" + html.EscapeString(f.Name)
				filesBlock += `<div class="admin-attach">` +
					`<a class="admin-attach-name" href="` + f.URL + `" download>` +
					html.EscapeString(f.Name) + `</a>` +
					`<button class="admin-attach-del" type="submit" form="` +
					delFormID + `" title="Удалить файл">&times;</button>` +
					`</div>`
			}
			filesBlock += `</div>`
		}
	}
	filesBlock += `<input class="admin-file" type="file" id="f-files" name="files" multiple>`
	if isNew {
		filesBlock += `<div class="admin-form-hint">Файлы видны участникам ` +
			`в карточке задания. Можно выбрать сразу несколько</div>`
	}
	filesBlock += `</div>`

	out := `<div class="admin-section">` +
		`<a class="admin-link admin-backlink" href="/admin.html?tab=tasks">&larr; Все задания</a>` +
		`<h1 class="admin-page-title">` + html.EscapeString(title) + `</h1>` +
		`<form method="post" action="/admin/task" enctype="multipart/form-data" class="admin-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("id", strconv.Itoa(task.ID)) +
		`<div class="admin-form-grid">` +
		adminInputRow("f-name", "name", "Название (рус)", task.Name, "", true) +
		adminInputRow("f-name-en", "name_en", "Название (англ)", task.NameEn,
			"", false) +
		adminTextareaRow("f-desc", "desc", "Описание (рус)", task.Desc,
			"Разрешён HTML: ссылки, форматирование") +
		adminTextareaRow("f-desc-en", "desc_en", "Описание (англ)", task.DescEn, "") +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="f-category">Категория</label>` +
		catSelect +
		`</div>` +
		adminInputRow("f-level", "level", "Уровень", strconv.Itoa(task.Level),
			"Порядок открытия внутри категории", true) +
		adminInputRow("f-tags", "tags", "Теги", task.Tags, "", false) +
		adminInputRow("f-author", "author", "Автор", task.Author, "", false) +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="f-flag">Флаг</label>` +
		`<input class="admin-input" type="text" id="f-flag" name="flag" value=""` +
		` autocomplete="off" spellcheck="false" placeholder="ETALON{...}"` +
		flagReq + `>` +
		`<div class="admin-form-hint">` + html.EscapeString(flagHint) + `</div>` +
		`<label class="admin-check-row admin-check-row-tight">` +
		`<input class="admin-checkbox" type="checkbox" name="flag_regex">` +
		`<span>Флаг задан регулярным выражением</span>` +
		`</label>` +
		`</div>` +
		`<div class="admin-form-row">` +
		`<label class="admin-label" for="f-status">Статус</label>` +
		statusSelect +
		`</div>` +
		filesBlock +
		`</div>` +
		`<div class="admin-form-actions">` +
		`<button class="admin-btn admin-btn-primary" type="submit">Сохранить</button>` +
		`<a class="admin-link" href="/admin.html?tab=tasks">Отмена</a>`

	if !isNew {
		out += adminDeleteButton("task-del", "Удалить задание")
	}

	out += `</div></form>`

	if !isNew {
		out += adminDeleteForm("task-del", "/admin/task/delete", csrf, task.ID)

		for _, f := range taskFiles(task.ID) {
			out += `<form id="del-file-` + html.EscapeString(f.Name) +
				`" method="post" action="/admin/task/file/delete">` +
				adminHidden("csrf", csrf) +
				adminHidden("id", strconv.Itoa(task.ID)) +
				adminHidden("name", f.Name) +
				`</form>`
		}
	}

	out += `</div>`
	return out
}

func adminTaskFormHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequireSession(w, r)
		if s == nil {
			return
		}

		var task *db.Task
		title := "Новое задание"
		if idStr := r.URL.Query().Get("id"); idStr != "" {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				adminErrorPage(w, "Некорректный идентификатор задания")
				return
			}
			t, err := db.GetTask(database, id)
			if err != nil {
				adminErrorPage(w, "Задание не найдено")
				return
			}
			task = &t
			title = t.Name
		}

		adminRenderPage(w, title, adminTaskFormHTML(database, task, s.CSRF))
	}
}

func resolveCategory(database *sql.DB, r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.FormValue("category_id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("category not selected")
	}
	return id, nil
}

func adminTaskSaveHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			adminErrorPage(w, "Не удалось обработать форму (слишком большой файл?)")
			return
		}

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор задания")
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			adminErrorPage(w, "Название задания не может быть пустым")
			return
		}

		level, err := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
		if err != nil || level < 1 {
			adminErrorPage(w, "Уровень должен быть целым числом от 1")
			return
		}

		catID, err := resolveCategory(database, r)
		if err != nil {
			adminErrorPage(w, "Не удалось определить категорию задания")
			return
		}

		var task db.Task
		isNew := id == 0
		if isNew {
			task = db.Task{
				Price:         500,
				Shared:        true,
				MaxSharePrice: 500,
				MinSharePrice: 100,
			}
		} else {
			task, err = db.GetTask(database, id)
			if err != nil {
				adminErrorPage(w, "Задание не найдено")
				return
			}
		}

		task.Name = name
		task.NameEn = strings.TrimSpace(r.FormValue("name_en"))
		if task.NameEn == "" {
			task.NameEn = name
		}
		task.Desc = r.FormValue("desc")
		task.DescEn = r.FormValue("desc_en")
		if task.DescEn == "" {
			task.DescEn = task.Desc
		}
		task.Tags = strings.TrimSpace(r.FormValue("tags"))
		task.Author = strings.TrimSpace(r.FormValue("author"))
		task.CategoryID = catID
		task.Level = level

		flag := r.FormValue("flag")
		if flag != "" {
			if r.FormValue("flag_regex") != "" {
				if _, rerr := regexp.Compile(flag); rerr != nil {
					adminErrorPage(w,
						"Флаг задан как регулярное выражение, но оно не компилируется")
					return
				}
				task.Flag = flag
			} else {

				task.Flag = regexp.QuoteMeta(flag)
			}
		}
		if task.Flag == "" {
			adminErrorPage(w, "Флаг не может быть пустым")
			return
		}

		wasOpened := task.Opened
		switch r.FormValue("status") {
		case "open":
			task.Opened = true
			task.ForceClosed = false
			if !wasOpened {
				task.OpenedTime = time.Now()
			}
		case "closed":
			task.Opened = false
			task.ForceClosed = true
		default:
			task.Opened = false
			task.ForceClosed = false
		}

		if isNew {
			err = db.AddTask(database, &task)
		} else {
			err = db.UpdateTask(database, &task)
		}
		if err != nil {
			log.Println("[admin] save task:", err)
			adminErrorPage(w, "Не удалось сохранить задание")
			return
		}

		if err := saveTaskUploads(r, task.ID); err != nil {
			log.Println("[admin] save uploads:", err)
			adminErrorPage(w, "Задание сохранено, но файл загрузить не удалось")
			return
		}

		action := "updated"
		if isNew {
			action = "created"
		}
		log.Printf("[admin] task %d (%s) %s by %s",
			task.ID, task.Name, action, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=tasks", http.StatusSeeOther)
	}
}

func adminTaskDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор задания")
			return
		}

		task, err := db.GetTask(database, id)
		if err != nil {
			adminErrorPage(w, "Задание не найдено")
			return
		}

		if err := db.DeleteFlagsByTask(database, id); err != nil {
			log.Println("[admin] delete task flags:", err)
			adminErrorPage(w, "Не удалось удалить флаги задания")
			return
		}

		if err := db.DeleteTask(database, id); err != nil {
			log.Println("[admin] delete task:", err)
			adminErrorPage(w, "Не удалось удалить задание")
			return
		}

		if err := os.RemoveAll(taskFilesDir(id)); err != nil {
			log.Println("[admin] delete task files:", err)
		}

		log.Printf("[admin] task %d (%s) DELETED by %s",
			task.ID, task.Name, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=tasks", http.StatusSeeOther)
	}
}

func adminTeamFormHTML(team *db.Team, csrf string) string {
	isNew := team == nil
	if isNew {
		team = &db.Team{}
	}

	title := "Новая команда"
	if !isNew {
		title = team.Name
	}

	testChecked := ""
	if team.Test {
		testChecked = " checked"
	}

	out := `<div class="admin-section">` +
		`<a class="admin-link admin-backlink" href="/admin.html?tab=teams">&larr; Все команды</a>` +
		`<h1 class="admin-page-title">` + html.EscapeString(title) + `</h1>` +
		`<form method="post" action="/admin/team" class="admin-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("id", strconv.Itoa(team.ID)) +
		`<div class="admin-form-grid">` +
		adminInputRow("f-name", "name", "Название команды", team.Name, "", true) +
		adminInputRow("f-desc", "desc", "Школа", team.Desc, "", false) +
		adminInputRow("f-email", "email", "Email", team.Email, "", false) +
		`</div>` +
		`<label class="admin-check-row">` +
		`<input class="admin-checkbox" type="checkbox" name="test"` + testChecked + `>` +
		`<span>Тестовая команда (не участвует в зачёте)</span>` +
		`</label>`

	if isNew {
		out += `<div class="admin-form-hint">Токен доступа будет сгенерирован ` +
			`автоматически и показан один раз после сохранения.</div>`
	} else {
		out += `<label class="admin-check-row">` +
			`<input class="admin-checkbox" type="checkbox" name="regen_token">` +
			`<span>Сгенерировать новый токен доступа (старый перестанет действовать)</span>` +
			`</label>`
	}

	out += `<div class="admin-form-actions">` +
		`<button class="admin-btn admin-btn-primary" type="submit">Сохранить</button>` +
		`<a class="admin-link" href="/admin.html?tab=teams">Отмена</a>`

	if !isNew {
		out += adminDeleteButton("team-del", "Удалить команду")
	}

	out += `</div></form>`

	if !isNew {
		out += adminDeleteForm("team-del", "/admin/team/delete", csrf, team.ID)
	}

	out += `</div>`
	return out
}

func adminTokenPageHTML(teamName, token string) string {
	return `<div class="admin-section">` +
		`<h1 class="admin-page-title">Токен доступа</h1>` +
		`<div class="admin-token-box">` +
		`<div class="admin-token-team">Команда &laquo;` +
		html.EscapeString(teamName) + `&raquo;</div>` +
		`<div class="admin-token-value">` + html.EscapeString(token) + `</div>` +
		`<div class="admin-token-note">Токен показывается только один раз. ` +
		`Сохраните его и передайте команде: по нему команда входит на платформу.</div>` +
		`</div>` +
		`<div class="admin-form-actions">` +
		`<a class="admin-btn admin-btn-primary" href="/admin.html?tab=teams">Вернуться в панель</a>` +
		`</div></div>`
}

func adminTeamFormHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequireSession(w, r)
		if s == nil {
			return
		}

		var team *db.Team
		title := "Новая команда"
		if idStr := r.URL.Query().Get("id"); idStr != "" {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				adminErrorPage(w, "Некорректный идентификатор команды")
				return
			}
			t, err := db.GetTeam(database, id)
			if err != nil {
				adminErrorPage(w, "Команда не найдена")
				return
			}
			team = &t
			title = t.Name
		}

		adminRenderPage(w, title, adminTeamFormHTML(team, s.CSRF))
	}
}

func adminTeamSaveHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор команды")
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			adminErrorPage(w, "Название команды не может быть пустым")
			return
		}

		isNew := id == 0

		var team db.Team
		if !isNew {
			team, err = db.GetTeam(database, id)
			if err != nil {
				adminErrorPage(w, "Команда не найдена")
				return
			}
		}

		team.Name = name
		team.Desc = strings.TrimSpace(r.FormValue("desc"))
		team.Email = strings.TrimSpace(r.FormValue("email"))
		team.Test = r.FormValue("test") != ""

		newToken := ""
		if isNew || r.FormValue("regen_token") != "" {
			newToken, err = genTeamToken()
			if err != nil {
				log.Println("[admin] gen token:", err)
				adminErrorPage(w, "Не удалось сгенерировать токен")
				return
			}
			team.Token = newToken
		}

		if isNew {
			err = db.AddTeam(database, &team)
		} else {
			err = db.UpdateTeam(database, &team)
		}
		if err != nil {
			log.Println("[admin] save team:", err)
			adminErrorPage(w, "Не удалось сохранить команду")
			return
		}

		if !isNew && newToken != "" {
			if err := db.DeleteSessionsByTeam(database, team.ID); err != nil {
				log.Println("[admin] drop team sessions:", err)
			}
		}

		action := "updated"
		if isNew {
			action = "created"
		}
		if newToken != "" {
			action += " (new token)"
		}
		log.Printf("[admin] team %d (%s) %s by %s",
			team.ID, team.Name, action, adminClientIP(r))

		if newToken != "" {
			adminRenderPage(w, "Токен доступа",
				adminTokenPageHTML(team.Name, newToken))
			return
		}

		http.Redirect(w, r, "/admin.html?tab=teams", http.StatusSeeOther)
	}
}

func adminTeamDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор команды")
			return
		}

		team, err := db.GetTeam(database, id)
		if err != nil {
			adminErrorPage(w, "Команда не найдена")
			return
		}

		steps := []struct {
			what string
			fn   func(*sql.DB, int) error
		}{
			{"flags", db.DeleteFlagsByTeam},
			{"sessions", db.DeleteSessionsByTeam},
			{"scores", db.DeleteScoresByTeam},
			{"team", db.DeleteTeam},
		}
		for _, step := range steps {
			if err := step.fn(database, id); err != nil {
				log.Printf("[admin] delete team %s: %v", step.what, err)
				adminErrorPage(w, "Не удалось удалить команду")
				return
			}
		}

		log.Printf("[admin] team %d (%s) DELETED by %s",
			team.ID, team.Name, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=teams", http.StatusSeeOther)
	}
}

func adminNewsFormHTML(n *db.News, csrf string) string {
	isNew := n == nil
	if isNew {
		n = &db.News{}
	}

	title := "Новая новость"
	if !isNew {
		title = n.Title
	}

	out := `<div class="admin-section">` +
		`<a class="admin-link admin-backlink" href="/admin.html?tab=news">&larr; Все новости</a>` +
		`<h1 class="admin-page-title">` + html.EscapeString(title) + `</h1>` +
		`<form method="post" action="/admin/news" class="admin-form">` +
		adminHidden("csrf", csrf) +
		adminHidden("id", strconv.Itoa(n.ID)) +
		`<div class="admin-form-grid">` +
		adminInputRow("f-title", "title", "Заголовок", n.Title, "", true) +
		adminInputRow("f-tag", "tag", "Тег", n.Tag,
			"Короткая метка: важно, система, инфраструктура...", false) +
		adminTextareaRow("f-body", "body", "Текст новости", n.Body,
			"Новость появится у всех участников в профиле") +
		`</div>` +
		`<div class="admin-form-actions">` +
		`<button class="admin-btn admin-btn-primary" type="submit">Опубликовать</button>` +
		`<a class="admin-link" href="/admin.html?tab=news">Отмена</a>`

	if !isNew {
		out += adminDeleteButton("news-del", "Удалить новость")
	}

	out += `</div></form>`

	if !isNew {
		out += adminDeleteForm("news-del", "/admin/news/delete", csrf, n.ID)
	}

	out += `</div>`
	return out
}

func adminNewsFormHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequireSession(w, r)
		if s == nil {
			return
		}

		var item *db.News
		title := "Новая новость"
		if idStr := r.URL.Query().Get("id"); idStr != "" {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				adminErrorPage(w, "Некорректный идентификатор новости")
				return
			}
			n, err := db.GetNewsItem(database, id)
			if err != nil {
				adminErrorPage(w, "Новость не найдена")
				return
			}
			item = &n
			title = n.Title
		}

		adminRenderPage(w, title, adminNewsFormHTML(item, s.CSRF))
	}
}

func adminNewsSaveHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор новости")
			return
		}

		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			adminErrorPage(w, "Заголовок новости не может быть пустым")
			return
		}

		item := db.News{
			ID:    id,
			Title: title,
			Body:  strings.TrimSpace(r.FormValue("body")),
			Tag:   strings.TrimSpace(r.FormValue("tag")),
		}

		isNew := id == 0
		if isNew {
			err = db.AddNews(database, &item)
		} else {
			if _, gerr := db.GetNewsItem(database, id); gerr != nil {
				adminErrorPage(w, "Новость не найдена")
				return
			}
			err = db.UpdateNews(database, &item)
		}
		if err != nil {
			log.Println("[admin] save news:", err)
			adminErrorPage(w, "Не удалось сохранить новость")
			return
		}

		action := "updated"
		if isNew {
			action = "published"
		}
		log.Printf("[admin] news %d (%s) %s by %s",
			item.ID, item.Title, action, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=news", http.StatusSeeOther)
	}
}

func adminNewsDeleteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminSecurityHeaders(w)

		s := adminRequirePostSession(w, r)
		if s == nil {
			return
		}

		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			adminErrorPage(w, "Некорректный идентификатор новости")
			return
		}

		item, err := db.GetNewsItem(database, id)
		if err != nil {
			adminErrorPage(w, "Новость не найдена")
			return
		}

		if err := db.DeleteNews(database, id); err != nil {
			log.Println("[admin] delete news:", err)
			adminErrorPage(w, "Не удалось удалить новость")
			return
		}

		log.Printf("[admin] news %d (%s) DELETED by %s",
			item.ID, item.Title, adminClientIP(r))

		http.Redirect(w, r, "/admin.html?tab=news", http.StatusSeeOther)
	}
}
