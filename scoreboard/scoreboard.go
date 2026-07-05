package scoreboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fiam/gounidecode/unidecode"
	"github.com/jollheef/henhouse/db"
	"github.com/jollheef/henhouse/game"
	"golang.org/x/net/websocket"
)

const (
	contestStateNotAvailable = "state n/a"
	contestNotStarted        = "not started"
	contestRunning           = "running"
	contestCompleted         = "completed"
)

var (
	gameShim         *game.Game
	contestStatus    string
	scoreCache       []game.TeamScoreInfo
	solvedCountCache map[int]int
	filesPath        string
)

var (
	InfoTimeout = time.Second

	ScoreboardTimeout = time.Second

	TasksTimeout = time.Second

	FlagTimeout = time.Second

	ScoreboardRecalcTimeout = time.Second
)

func durationToHMS(d time.Duration) string {

	sec := int(d.Seconds())

	var h, m, s int

	h = sec / 60 / 60
	m = (sec / 60) % 60
	s = sec % 60

	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func getInfo() string {

	var left time.Duration
	var btnType string
	timerLabel := "до конца"

	now := time.Now()

	if now.Before(gameShim.Start) {

		contestStatus = contestNotStarted
		left = gameShim.Start.Sub(now)
		btnType = "stop"
		timerLabel = "до старта"

	} else if now.Before(gameShim.End) {

		contestStatus = contestRunning
		left = gameShim.End.Sub(now)
		btnType = "run"

	} else {
		contestStatus = contestCompleted
		left = 0
		btnType = "stop"
	}

	var info string
	if left != 0 {
		info = fmt.Sprintf(
			`<div class="timer-block">`+
				`<span class="timer-label">%s</span>`+
				`<span id="timer">%s</span>`+
				`</div>`,
			timerLabel, durationToHMS(left))
	} else {
		info = fmt.Sprintf(`<div class="timer-block"><span id="game_status-%s">contest %s</span></div>`,
			btnType, contestStatus)
	}

	return info
}

func infoHandler(ws *websocket.Conn) {

	teamID := getTeamID(ws.Request())
	defer ws.Close()
	for {
		content := l10n(ws.Request(), getInfo()) + profileWidgetHTML(teamID)
		if _, err := fmt.Fprint(ws, content); err != nil {
			return
		}

		time.Sleep(InfoTimeout)
	}
}

func scoreboardHandler(ws *websocket.Conn) {

	defer ws.Close()

	teamID := getTeamID(ws.Request())

	currentResult := scoreboardHTML(teamID)

	fmt.Fprint(ws, l10n(ws.Request(), currentResult))

	sendedResult := currentResult

	lastUpdate := time.Now()

	for {
		currentResult := scoreboardHTML(teamID)

		if sendedResult != currentResult ||
			time.Now().After(lastUpdate.Add(time.Minute)) {

			sendedResult = currentResult
			lastUpdate = time.Now()

			_, err := fmt.Fprint(ws, l10n(ws.Request(), currentResult))
			if err != nil {

				return
			}
		}

		time.Sleep(ScoreboardTimeout)
	}
}

var wreathSVG = [3]string{

	`<svg viewBox="0 0 54 52" width="54" height="52" style="position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);pointer-events:none"></svg>`,

	`<svg viewBox="0 0 54 52" width="54" height="52" style="position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);pointer-events:none"></svg>`,

	`<svg viewBox="0 0 54 52" width="54" height="52" style="position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);pointer-events:none"></svg>`,
}

func init() {
	wreathSVG[0] = buildWreathSVG("#f5a623")
	wreathSVG[1] = buildWreathSVG("#c9ced3")
	wreathSVG[2] = buildWreathSVG("#c0894a")
}

func buildWreathSVG(color string) string {
	cx, cy := 27.0, 26.0
	rx, ry := 13.0, 13.0
	a0, a1 := 117.0, 243.0
	N := 11
	leafLen, leafW := 7.4, 2.3
	lean, fan, taper := 40.0, 20.0, 0.7

	pt := func(deg, sgn float64) (float64, float64) {
		a := deg * math.Pi / 180
		return cx + sgn*rx*math.Cos(a), cy + ry*math.Sin(a)
	}

	leafPath := func(lx, ly, ang, length, w float64) string {
		a := ang * math.Pi / 180
		dx, dy := math.Cos(a), math.Sin(a)
		px2, py2 := -dy, dx
		tx, ty := lx+dx*length, ly+dy*length
		b1x := lx + dx*length*0.40 + px2*w*1.1
		b1y := ly + dy*length*0.40 + py2*w*1.1
		b2x := lx + dx*length*0.55 - px2*w*0.85
		b2y := ly + dy*length*0.55 - py2*w*0.85
		return fmt.Sprintf("M %.2f %.2f Q %.2f %.2f %.2f %.2f Q %.2f %.2f %.2f %.2f Z",
			lx, ly, b1x, b1y, tx, ty, b2x, b2y, lx, ly)
	}

	branch := func(sgn float64) string {
		var stem string
		for s := 0; s <= 40; s++ {
			deg := a0 + (a1-a0)*(float64(s)/40)
			x, y := pt(deg, sgn)
			if s == 0 {
				stem += fmt.Sprintf("M %.2f %.2f ", x, y)
			} else {
				stem += fmt.Sprintf("L %.2f %.2f ", x, y)
			}
		}
		out := fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>`, stem, color)
		for i := 0; i < N; i++ {
			t := float64(i) / float64(N-1)
			deg := a0 + (a1-a0)*t
			th := deg * math.Pi / 180
			lx, ly := pt(deg, sgn)
			tang := math.Atan2(ry*math.Cos(th), -sgn*rx*math.Sin(th)) * 180 / math.Pi
			outerDir := tang - sgn*lean
			innerDir := tang - sgn*(lean-fan)
			sizeF := 0.5 + 0.6*math.Sin(math.Pi*t)
			l := leafLen * sizeF
			w := leafW * sizeF
			out += fmt.Sprintf(`<path d="%s" fill="%s"/>`, leafPath(lx, ly, outerDir, l, w), color)
			out += fmt.Sprintf(`<path d="%s" fill="%s"/>`, leafPath(lx, ly, innerDir, l*taper+1, w*0.75), color)
		}
		return out
	}

	return `<svg viewBox="0 0 54 52" width="54" height="52" style="position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);pointer-events:none">` +
		branch(1) + branch(-1) + `</svg>`
}

func scoreboardHTML(teamID int) (result string) {

	result = "<thead><tr>" +
		`<th class="team_index">Место</th>` +
		`<th class="team_name">Команда</th>` +
		`<th class="team_school">Школа</th>` +
		`<th class="team_solved">Решено</th>` +
		`<th class="team_score">Очки</th>` +
		"</tr></thead>"

	result += "<tbody>"

	for n, teamScore := range scoreCache {
		var classes []string

		if teamScore.ID == teamID {
			classes = append(classes, "self_team")
		}

		var accentColor, rankColor string
		var wreathHTML string
		switch n + 1 {
		case 1:
			classes = append(classes, "rank-gold")
			accentColor = "#f5a623"
			rankColor = "#f5a623"
			wreathHTML = wreathSVG[0]
		case 2:
			classes = append(classes, "rank-silver")
			accentColor = "#c9ced3"
			rankColor = "#c9ced3"
			wreathHTML = wreathSVG[1]
		case 3:
			classes = append(classes, "rank-bronze")
			accentColor = "#c0894a"
			rankColor = "#c0894a"
			wreathHTML = wreathSVG[2]
		default:
			accentColor = "#2b3038"
			rankColor = "#5b626b"
		}

		if len(classes) > 0 {
			result += `<tr class="`
			for i, c := range classes {
				if i > 0 {
					result += " "
				}
				result += c
			}
			result += `">`
		} else {
			result += `<tr>`
		}

		solved := 0
		if solvedCountCache != nil {
			solved = solvedCountCache[teamScore.ID]
		}

		school := html.EscapeString(teamScore.Desc)
		name := html.EscapeString(teamScore.Name)

		result += fmt.Sprintf(
			`<td class="team_index">`+
				`<div class="rank-cell">`+
				`<span class="rank-bar" style="background:%s"></span>`+
				`<span class="rank-num-wrap" style="position:relative;display:inline-flex;align-items:center;justify-content:center;width:34px;height:34px">`+
				`%s`+
				`<span class="rank-num" style="position:relative;color:%s">%d</span>`+
				`</span>`+
				`</div></td>`+
				`<td class="team_name"><a class="team-link" href="/team.html?id=%d" title="Статистика команды">%s</a></td>`+
				`<td class="team_school">%s</td>`+
				`<td class="team_solved">%d</td>`+
				`<td class="team_score">%d</td></tr>`,
			accentColor, wreathHTML, rankColor,
			n+1, teamScore.ID, name, school, solved, teamScore.Score)
	}

	result += "</tbody>"

	return
}

func scoreboardUpdater(game *game.Game, updateTimeout time.Duration) {

	for {
		err := game.RecalcScoreboard()
		if err != nil {
			log.Println("Recalc scoreboard fail:", err)
			time.Sleep(updateTimeout)
			continue
		}

		scoreCache, err = game.Scoreboard()
		if err != nil {
			log.Println("Get scoreboard fail:", err)
			time.Sleep(updateTimeout)
			continue
		}

		cats, terr := game.Tasks()
		if terr == nil {
			sc := make(map[int]int)
			for _, cat := range cats {
				for _, t := range cat.TasksInfo {
					for _, tid := range t.SolvedBy {
						sc[tid]++
					}
				}
			}
			solvedCountCache = sc
		}

		time.Sleep(updateTimeout)
	}
}

func tasksHTML(teamID int, ru bool) (result string) {

	cats, err := gameShim.Tasks()
	if err != nil {
		log.Println("Get tasks fail:", err)
	}

	for _, cat := range cats {
		result += categoryToHTML(teamID, cat, ru)
	}

	return
}

func tasksHandler(ws *websocket.Conn) {

	defer ws.Close()

	teamID := getTeamID(ws.Request())

	currentTasks := tasksHTML(teamID, isAcceptRussian(ws.Request()))

	fmt.Fprint(ws, l10n(ws.Request(), currentTasks))

	sendedTasks := currentTasks

	lastUpdate := time.Now()

	for {
		currentTasks := tasksHTML(teamID, isAcceptRussian(ws.Request()))

		if sendedTasks != currentTasks ||
			time.Now().After(lastUpdate.Add(time.Minute)) {

			sendedTasks = currentTasks
			lastUpdate = time.Now()

			_, err := fmt.Fprint(ws, l10n(ws.Request(), currentTasks))
			if err != nil {

				return
			}
		}

		time.Sleep(TasksTimeout)
	}
}

func taskHandler(w http.ResponseWriter, r *http.Request) {

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		log.Println("Atoi fail:", err)
		http.Redirect(w, r, "/", 307)
		return
	}

	cats, err := gameShim.Tasks()
	if err != nil {
		log.Println("Get tasks fail:", err)
		http.Redirect(w, r, "/", 307)
		return
	}

	task := game.TaskInfo{ID: id, Opened: false}

	for _, c := range cats {
		for _, t := range c.TasksInfo {
			if t.ID == id {
				task = t
				break
			}
		}
	}

	if !task.Opened {

		http.Redirect(w, r, "/", 307)
		return
	}

	teamID := getTeamID(r)

	flagSubmitFormat :=
		`<div class="flag-form-label">Отправить флаг</div>` +
			`<form class="input-group" action="/flag?id=%d" method="post">` +
			`<input class="form-control float-left" name="flag" value="" placeholder="Flag">` +
			`<span class="input-group-btn">` +
			`<button class="btn btn-submit">Submit</button>` +
			`</span>` +
			`</form>`

	var submitForm string
	if taskSolvedBy(task, teamID) {

		submitForm = `<div class="task-solved-banner">` +
			`<span class="task-solved-check">` +
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
			`</span>` +
			`<div class="task-solved-title">Решено вашей командой</div>` +
			`</div>`
	} else {
		submitForm = fmt.Sprintf(flagSubmitFormat, task.ID)
	}

	tmpl, err := getTmpl("task")
	if err != nil {
		log.Println(err)
		return
	}

	var name, desc, author string
	if isAcceptRussian(r) {
		name = task.Name
		desc = task.Desc
		author = task.Author
	} else {
		name = task.NameEn
		desc = task.DescEn
		author = unidecode.Unidecode(task.Author)
	}

	fmt.Fprintf(w, l10n(r, tmpl), name, task.Price,
		desc+taskFilesHTML(task.ID),
		author, l10n(r, submitForm))
}

type taskFileRef struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func taskFiles(taskID int) (files []taskFileRef) {
	entries, err := os.ReadDir(fmt.Sprintf("%s/%d", filesPath, taskID))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		files = append(files, taskFileRef{
			Name: e.Name(),
			URL:  fmt.Sprintf("/files/%d/%s", taskID, e.Name()),
		})
	}
	return
}

func taskFilesHTML(taskID int) string {
	files := taskFiles(taskID)
	if len(files) == 0 {
		return ""
	}
	out := `<div class="task-files"><div class="task-files-label">Файлы</div>`
	for _, f := range files {
		out += fmt.Sprintf(
			`<a class="task-file-link" href="%s" download>%s</a>`,
			f.URL, html.EscapeString(f.Name))
	}
	out += `</div>`
	return out
}

type taskAPIResp struct {
	ID     int           `json:"id"`
	Name   string        `json:"name"`
	Desc   string        `json:"desc"`
	Author string        `json:"author"`
	Price  int           `json:"price"`
	Cat    string        `json:"cat"`
	Solves int           `json:"solves"`
	Solved bool          `json:"solved"`
	Files  []taskFileRef `json:"files"`
}

func taskAPIHandler(w http.ResponseWriter, r *http.Request) {

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	cats, err := gameShim.Tasks()
	if err != nil {
		log.Println("Get tasks fail:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	task := game.TaskInfo{ID: id, Opened: false}
	catName := ""

	for _, c := range cats {
		for _, t := range c.TasksInfo {
			if t.ID == id {
				task = t
				catName = c.Name
				break
			}
		}
	}

	if !task.Opened {

		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	teamID := getTeamID(r)

	var name, desc, author string
	if isAcceptRussian(r) {
		name = task.Name
		desc = task.Desc
		author = task.Author
	} else {
		name = task.NameEn
		desc = task.DescEn
		author = unidecode.Unidecode(task.Author)
	}

	resp := taskAPIResp{
		ID:     task.ID,
		Name:   name,
		Desc:   desc,
		Author: author,
		Price:  task.Price,
		Cat:    catName,
		Solves: len(task.SolvedBy),
		Solved: taskSolvedBy(task, teamID),
		Files:  taskFiles(task.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Println("task api encode:", err)
	}
}

type flagAPIResp struct {
	OK  bool   `json:"ok"`
	Msg string `json:"msg"`
}

func flagAPIHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		taskID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		flag := r.FormValue("flag")

		teamID := getTeamID(r)

		w.Header().Set("Content-Type", "application/json")

		already, err := db.IsSolved(database, teamID, taskID)
		if err == nil && already {
			resp := flagAPIResp{OK: false,
				Msg: "Задание уже решено вашей командой."}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Println("flag api encode:", err)
			}
			return
		}

		solved, err := gameShim.Solve(teamID, taskID, flag)
		if err != nil {
			solved = false
		}

		log.Printf("Team ID: %d, Task ID: %d, Flag: %s, Solved: %v\n",
			teamID, taskID, flag, solved)

		time.Sleep(FlagTimeout)

		resp := flagAPIResp{OK: solved}
		if solved {
			resp.Msg = "Флаг принят. Очки начислены вашей команде."
		} else {
			resp.Msg = "Неверный флаг. Попробуйте ещё раз."
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Println("flag api encode:", err)
		}
	}
}

func flagHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if r.Method != "POST" {
			http.Redirect(w, r, "/", 307)
			return
		}

		taskID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			log.Println("Atoi fail:", err)
			http.Redirect(w, r, "/", 307)
			return
		}

		flag := r.FormValue("flag")

		teamID := getTeamID(r)

		var solvedMsg string

		already, err := db.IsSolved(database, teamID, taskID)
		if err == nil && already {
			solvedMsg = `<div class="flag_status invalid">Already solved</div>`
		} else {

			solved, serr := gameShim.Solve(teamID, taskID, flag)
			if serr != nil {
				solved = false
			}

			if solved {
				solvedMsg = `<div class="flag_status solved">Solved</div>`
			} else {
				solvedMsg = `<div class="flag_status invalid">Invalid flag</div>`
			}

			time.Sleep(FlagTimeout)
		}

		log.Printf("Team ID: %d, Task ID: %d, Flag: %s, Result: %s\n",
			teamID, taskID, flag, solvedMsg)

		tmpl, err := getTmpl("flag")
		if err != nil {
			log.Println(err)
			return
		}

		fmt.Fprintf(w, l10n(r, tmpl), l10n(r, solvedMsg))
	}
}

func filesHandler() http.Handler {
	fs := http.StripPrefix("/files/", http.FileServer(http.Dir(filesPath)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func handleStaticFile(pattern, file string) {
	http.HandleFunc(pattern,
		func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, file)
		})
}

func handleStaticFileSimple(file, wwwPath string) {
	handleStaticFile(file, wwwPath+file)
}

func signinHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := getTmpl("auth")
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprint(w, l10n(r, tmpl))
}

func supportHandler(w http.ResponseWriter, r *http.Request) {
	teamID := getTeamID(r)

	tmpl, err := getTmpl("support")
	if err != nil {
		log.Println(err)
		return
	}

	var content string
	if r.URL.Query().Get("sent") == "1" {
		content = `<div class="support-success">` +
			`<span class="support-success-icon">` +
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.27"></polyline></svg>` +
			`</span>` +
			`<div class="support-success-title">Обращение отправлено</div>` +
			`<div class="support-success-text">Мы свяжемся с вами в ближайшее время. ` +
			`Ответ придёт на почту, указанную в профиле.</div>` +
			`<a class="support-again" href="/support.html">Написать ещё</a>` +
			`</div>`
	} else {
		content, err = getTmpl("support_form")
		if err != nil {
			log.Println("[support] form tmpl:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	fmt.Fprintf(w, l10n(r, tmpl),
		l10n(r, getInfo())+profileWidgetHTML(teamID),
		content)
}

const (
	supportMaxUpload     = 10 << 20
	supportMaxTextRunes  = 2000
	supportMaxFieldRunes = 256
)

var (
	SupportLog   string
	SupportFiles string
)

func appendSupportRequest(teamID int, ptype, contact, attach, text string) {
	if SupportLog == "" {
		return
	}
	f, err := os.OpenFile(SupportLog,
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Println("[support] open log:", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\tteam=%d\ttype=%q\tcontact=%q\tattach=%q\ttext=%q\n",
		time.Now().Format(time.RFC3339), teamID, ptype, contact, attach, text)
}

func clampRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

func supportAttachPath(id int, name string) string {
	if SupportFiles == "" || name == "" {
		return ""
	}
	return filepath.Join(SupportFiles, strconv.Itoa(id), name)
}

func supportSaveAttach(id int, fh *multipart.FileHeader, name string) error {
	dir := filepath.Join(SupportFiles, strconv.Itoa(id))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func supportSubmitHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/support.html", http.StatusSeeOther)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, supportMaxUpload+(1<<20))

		if err := r.ParseMultipartForm(supportMaxUpload); err != nil {
			http.Redirect(w, r, "/support.html", http.StatusSeeOther)
			return
		}

		text := strings.TrimSpace(r.FormValue("text"))
		if text == "" {
			http.Redirect(w, r, "/support.html", http.StatusSeeOther)
			return
		}
		text = clampRunes(text, supportMaxTextRunes)

		var fh *multipart.FileHeader
		attach := ""
		if r.MultipartForm != nil && len(r.MultipartForm.File["file"]) > 0 {
			fh = r.MultipartForm.File["file"][0]
			if fh.Size > 0 && fh.Size <= supportMaxUpload &&
				SupportFiles != "" {
				attach = sanitizeAttachName(fh.Filename)
			}
			if attach == "" {
				fh = nil
			}
		}

		req := db.SupportRequest{
			TeamID:  getTeamID(r),
			Type:    clampRunes(strings.TrimSpace(r.FormValue("ptype")), 64),
			Contact: clampRunes(strings.TrimSpace(r.FormValue("contact")), supportMaxFieldRunes),
			Attach:  attach,
			Text:    text,
		}

		if err := db.AddSupportRequest(database, &req); err != nil {
			log.Println("[support] save request:", err)
			http.Redirect(w, r, "/support.html", http.StatusSeeOther)
			return
		}

		if fh != nil {
			if err := supportSaveAttach(req.ID, fh, attach); err != nil {
				log.Println("[support] save attach:", err)
				req.Attach = ""
				if err := db.SetSupportAttach(database,
					req.ID, ""); err != nil {
					log.Println("[support] reset attach:", err)
				}
			}
		}

		appendSupportRequest(req.TeamID, req.Type, req.Contact,
			req.Attach, req.Text)

		log.Printf("[support] request %d from team %d: type=%q attach=%q text_len=%d",
			req.ID, req.TeamID, req.Type, req.Attach, len([]rune(req.Text)))

		tgEnqueue(database, req.ID)

		http.Redirect(w, r, "/support.html?sent=1", http.StatusSeeOther)
	}
}

func sponsorsHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var tmpl string
	if isAcceptRussian(r) {
		tmpl, err = getTmpl("sponsors.ru")
	} else {
		tmpl, err = getTmpl("sponsors.en")
	}
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprint(w, l10n(r, tmpl))
}

func Scoreboard(database *sql.DB, game *game.Game,
	wwwPath, tmpltsPath, addr string, proxy bool) (err error) {

	contestStatus = contestStateNotAvailable
	gameShim = game
	templatePath = tmpltsPath
	underProxy = proxy

	filesPath = wwwPath + "/files"
	if err = os.MkdirAll(filesPath, 0755); err != nil {
		log.Println("Create files dir fail:", err)
		return
	}

	scoreCache, err = gameShim.Scoreboard()
	if err != nil {
		log.Println("Get scoreboard fail:", err)
		return
	}

	go scoreboardUpdater(game, ScoreboardRecalcTimeout)

	tgStart(database)

	handleStaticFileSimple("/css/style.css", wwwPath)
	handleStaticFileSimple("/js/scoreboard.js", wwwPath)
	handleStaticFileSimple("/js/tasks.js", wwwPath)
	handleStaticFileSimple("/js/chart.js", wwwPath)
	handleStaticFileSimple("/js/profile.js", wwwPath)
	handleStaticFileSimple("/js/support.js", wwwPath)
	handleStaticFileSimple("/images/pgu-logo.png", wwwPath)
	handleStaticFileSimple("/images/bg.jpg", wwwPath)
	handleStaticFileSimple("/images/favicon.ico", wwwPath)
	handleStaticFileSimple("/images/favicon.png", wwwPath)
	handleStaticFileSimple("/images/401.jpg", wwwPath)
	handleStaticFileSimple("/images/juniors_ctf_txt.png", wwwPath)
	handleStaticFileSimple("/images/etalon-owl.png", wwwPath)

	http.HandleFunc("/auth.html", signinHandler)
	http.HandleFunc("/outer-scoreboard", outerScoreboard)

	http.Handle("/", authorized(database, http.HandlerFunc(innerScoreboard)))
	http.Handle("/index.html", authorized(database, http.HandlerFunc(innerScoreboard)))
	http.Handle("/tasks.html", authorized(database, http.HandlerFunc(staticTasks)))
	http.Handle("/profile.html", authorized(database, http.HandlerFunc(profileHandler(database))))
	http.Handle("/team.html", authorized(database, http.HandlerFunc(teamPageHandler(database))))
	http.Handle("/logout", authorized(database, logoutHandler(database)))
	http.Handle("/sponsors.html", authorized(database, http.HandlerFunc(sponsorsHandler)))
	http.Handle("/support.html", authorized(database, http.HandlerFunc(supportHandler)))
	http.Handle("/support", authorized(database, supportSubmitHandler(database)))
	http.Handle("/api/chart", authorized(database, chartHandler(database)))
	http.Handle("/files/", authorized(database, filesHandler()))
	http.Handle("/api/task", authorized(database, http.HandlerFunc(taskAPIHandler)))
	http.Handle("/api/flag", authorized(database, flagAPIHandler(database)))

	http.Handle("/scoreboard", authorized(database, websocket.Handler(scoreboardHandler)))
	http.Handle("/info", authorized(database, websocket.Handler(infoHandler)))
	http.Handle("/tasks", authorized(database, websocket.Handler(tasksHandler)))
	http.Handle("/tasks-summary", authorized(database, websocket.Handler(tasksSummaryWSHandler)))

	http.Handle("/task", authorized(database, http.HandlerFunc(taskHandler)))
	http.Handle("/flag", authorized(database, flagHandler(database)))

	http.HandleFunc("/auth.php", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			authHandler(database, w, r)
		}))

	if adminEnabled() {
		http.HandleFunc("/admin.html", adminPageHandler(database))
		http.HandleFunc("/admin/auth", adminAuthHandler)
		http.HandleFunc("/admin/logout", adminLogoutHandler)
		http.HandleFunc("/admin/action", adminActionHandler(database))
		http.HandleFunc("/admin/game", adminGameHandler(database))
		http.HandleFunc("/admin/export.csv", adminExportHandler(database))

		http.HandleFunc("/admin/task.html", adminTaskFormHandler(database))
		http.HandleFunc("/admin/task", adminTaskSaveHandler(database))
		http.HandleFunc("/admin/task/delete", adminTaskDeleteHandler(database))
		http.HandleFunc("/admin/task/file/delete", adminTaskFileDeleteHandler(database))

		http.HandleFunc("/admin/category", adminCategoryAddHandler(database))
		http.HandleFunc("/admin/category/delete", adminCategoryDeleteHandler(database))

		http.HandleFunc("/admin/team.html", adminTeamFormHandler(database))
		http.HandleFunc("/admin/team", adminTeamSaveHandler(database))
		http.HandleFunc("/admin/team/delete", adminTeamDeleteHandler(database))

		http.HandleFunc("/admin/news.html", adminNewsFormHandler(database))
		http.HandleFunc("/admin/news", adminNewsSaveHandler(database))
		http.HandleFunc("/admin/news/delete", adminNewsDeleteHandler(database))

		http.HandleFunc("/admin/support/settings", adminSupportSettingsHandler(database))
		http.HandleFunc("/admin/support/test", adminSupportTestHandler(database))
		http.HandleFunc("/admin/support/resend", adminSupportResendHandler(database))
		http.HandleFunc("/admin/support/done", adminSupportDoneHandler(database))
		http.HandleFunc("/admin/support/delete", adminSupportDeleteHandler(database))
		http.HandleFunc("/admin/support/file", adminSupportFileHandler(database))
	}

	log.Println("Launching scoreboard at", addr)

	return http.ListenAndServe(addr, nil)
}
