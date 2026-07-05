package scoreboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jollheef/henhouse/db"
	"golang.org/x/net/websocket"
)

func profileWidgetHTML(teamID int) string {
	name := "-"
	rank := 0
	for i, t := range scoreCache {
		if t.ID == teamID {
			name = html.EscapeString(t.Name)
			rank = i + 1
			break
		}
	}
	return fmt.Sprintf(
		`<a href="/profile.html" class="profile-widget">`+
			`<span class="profile-terminal">&gt;_</span>`+
			`<div class="profile-meta">`+
			`<span class="profile-name">%s</span>`+
			`<span class="profile-rank">#%d</span>`+
			`</div>`+
			`</a>`,
		name, rank)
}

func tasksSummaryHTML(teamID int) string {
	cats, err := gameShim.Tasks()
	if err != nil {
		return ""
	}

	solved, total := 0, 0
	for _, cat := range cats {
		for _, t := range cat.TasksInfo {
			if t.Opened {
				total++
			}
			if taskSolvedBy(t, teamID) {
				solved++
			}
		}
	}

	pts := 0
	for _, s := range scoreCache {
		if s.ID == teamID {
			pts = s.Score
			break
		}
	}

	return fmt.Sprintf(
		`<div class="tasks-page-header">`+
			`<div class="tasks-page-title-block">`+
			`<div class="tasks-page-title">Задания</div>`+
			`</div>`+
			`<div class="tasks-page-stats">`+
			`<div class="tasks-page-stat">`+
			`<div class="tasks-page-stat-val">%d<span class="tasks-page-stat-total">/%d</span></div>`+
			`<div class="tasks-page-stat-lbl">решено вами</div>`+
			`</div>`+
			`<div class="tasks-page-stat-div"></div>`+
			`<div class="tasks-page-stat">`+
			`<div class="tasks-page-stat-val">%d</div>`+
			`<div class="tasks-page-stat-lbl">очков набрано</div>`+
			`</div>`+
			`</div>`+
			`</div>`,
		solved, total, pts)
}

func tasksSummaryWSHandler(ws *websocket.Conn) {
	teamID := getTeamID(ws.Request())
	defer ws.Close()

	last := ""
	for {
		html := tasksSummaryHTML(teamID)
		if html != last {
			last = html
			if _, err := fmt.Fprint(ws, html); err != nil {
				return
			}
		}
		time.Sleep(TasksTimeout)
	}
}

type chartPoint struct {
	T int64 `json:"t"`
	S int   `json:"s"`
}

type chartTeam struct {
	ID   int          `json:"id"`
	Name string       `json:"name"`
	Mine bool         `json:"mine"`
	Pts  []chartPoint `json:"pts"`
}

type chartResp struct {
	Start int64       `json:"start"`
	End   int64       `json:"end"`
	Teams []chartTeam `json:"teams"`
}

func chartHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := getTeamID(r)

		solves, err := db.GetSolvedFlags(database)
		if err != nil {
			log.Println("GetSolvedFlags:", err)
			http.Error(w, "db error", 500)
			return
		}

		teams, err := db.GetTeams(database)
		if err != nil {
			log.Println("GetTeams:", err)
			http.Error(w, "db error", 500)
			return
		}

		cats, err := gameShim.Tasks()
		if err != nil {
			log.Println("Tasks:", err)
			http.Error(w, "db error", 500)
			return
		}

		priceMap := make(map[int]int)
		for _, cat := range cats {
			for _, t := range cat.TasksInfo {
				priceMap[t.ID] = t.Price
			}
		}

		nameMap := make(map[int]string)
		testMap := make(map[int]bool)
		for _, t := range teams {
			nameMap[t.ID] = t.Name
			testMap[t.ID] = t.Test
		}

		ptsMap := make(map[int][]chartPoint)
		cum := make(map[int]int)
		for _, f := range solves {
			if testMap[f.TeamID] {
				continue
			}
			price, ok := priceMap[f.TaskID]
			if !ok {
				continue
			}
			cum[f.TeamID] += price
			ptsMap[f.TeamID] = append(ptsMap[f.TeamID],
				chartPoint{T: f.Timestamp.Unix(), S: cum[f.TeamID]})
		}

		scorePos := make(map[int]int)
		for i, t := range scoreCache {
			scorePos[t.ID] = i
		}

		contestStart := gameShim.Start.Unix()
		contestEnd := gameShim.End.Unix()
		nowUnix := time.Now().Unix()
		chartNow := contestEnd
		if nowUnix < contestEnd {
			chartNow = nowUnix
		}

		currentScore := make(map[int]int)
		for _, sc := range scoreCache {
			currentScore[sc.ID] = sc.Score
		}

		resp := chartResp{
			Start: contestStart,
			End:   contestEnd,
		}

		for _, team := range teams {
			if team.Test {
				continue
			}
			tid := team.ID
			rawPts := ptsMap[tid]

			out := make([]chartPoint, 0, len(rawPts)+2)
			out = append(out, chartPoint{T: contestStart, S: 0})

			out = append(out, rawPts...)

			cur := currentScore[tid]
			if out[len(out)-1].T < chartNow {
				out = append(out, chartPoint{T: chartNow, S: cur})
			} else {
				out[len(out)-1].S = cur
			}

			resp.Teams = append(resp.Teams, chartTeam{
				ID:   tid,
				Name: nameMap[tid],
				Mine: tid == teamID,
				Pts:  out,
			})
		}
		sort.Slice(resp.Teams, func(i, j int) bool {
			pi, iok := scorePos[resp.Teams[i].ID]
			pj, jok := scorePos[resp.Teams[j].ID]
			if !iok {
				pi = len(scorePos) + resp.Teams[i].ID
			}
			if !jok {
				pj = len(scorePos) + resp.Teams[j].ID
			}
			return pi < pj
		})

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Println("chart encode:", err)
		}
	}
}

func newsBlockHTML(database *sql.DB) string {
	news, err := db.GetNews(database)
	if err != nil {
		log.Println("newsBlockHTML: GetNews:", err)
		return ""
	}

	out := `<div class="profile-news">` +
		`<div class="profile-news-header">` +
		`<span class="profile-news-bar"></span>` +
		`<span class="profile-news-title">Новости</span>` +
		`</div>`

	if len(news) == 0 {
		out += `<div class="profile-news-empty">Пока нет новостей</div>` +
			`</div>`
		return out
	}

	for _, item := range news {
		out += fmt.Sprintf(
			`<div class="news-acc-item" data-news-id="%d">`+
				`<button class="news-acc-trigger" type="button">`+
				`<div class="news-acc-meta">`+
				`<span class="news-acc-time">%s</span>`+
				`<span class="news-acc-tag">%s</span>`+
				`</div>`+
				`<div class="news-acc-title">%s</div>`+
				`<span class="news-acc-unread" aria-label="Непрочитано"></span>`+
				`<svg class="news-acc-chevron" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">`+
				`<polyline points="6 9 12 15 18 9"></polyline>`+
				`</svg>`+
				`</button>`+
				`<div class="news-acc-body" role="region">`+
				`<p class="news-acc-text">%s</p>`+
				`</div>`+
				`</div>`,
			item.ID,
			item.Timestamp.Local().Format("15:04"),
			html.EscapeString(item.Tag),
			html.EscapeString(item.Title),
			html.EscapeString(item.Body),
		)
	}

	out += `</div>`
	return out
}

func scoringRulesHTML() string {
	row := func(mark, title, body string) string {
		return `<div class="scoring-row">` +
			`<span class="scoring-mark">` + mark + `</span>` +
			`<div class="scoring-text">` +
			`<div class="scoring-title">` + title + `</div>` +
			`<div class="scoring-body">` + body + `</div>` +
			`</div></div>`
	}

	return `<div class="profile-news profile-scoring">` +
		`<div class="profile-news-header">` +
		`<span class="profile-news-bar"></span>` +
		`<span class="profile-news-title">Как начисляются баллы</span>` +
		`</div>` +
		row("1", "Динамическая оценка заданий",
			"Каждое задание стартует с максимальной оценки. Чем больше команд "+
				"решило задачу, тем ниже её оценка, но не ниже минимальной.") +
		row("2", "Итог фиксируется в конце игры",
			"Если оценка задачи снижается, баллы пересчитываются у всех "+
				"решивших её команд, поэтому счёт может уменьшаться по ходу игры.") +
		row("3", "Всё обновляется само",
			"Таблица результатов и список заданий обновляются в реальном "+
				"времени, страницу перезагружать не нужно.") +
		`</div>`
}

func sponsorsBlockHTML() string {
	vkCard := func(url, label, name string) string {
		return fmt.Sprintf(
			`<a href="%s" target="_blank" rel="noopener" class="support-vk-card">`+
				`<span class="support-vk-badge">VK</span>`+
				`<span class="support-vk-meta">`+
				`<span class="support-vk-label">%s</span>`+
				`<span class="support-vk-name">%s</span>`+
				`</span>`+
				`</a>`,
			url, label, name)
	}

	return `<div class="support-block">` +
		`<div class="support-text-wrap">` +
		`<div class="support-label">При поддержке</div>` +
		`<div class="support-text">Мероприятие проводится при поддержке ` +
		`<span class="support-name">ПГУ</span>, кафедра ` +
		`<span class="support-name">ИБСТ</span>.</div>` +
		`</div>` +
		`<div class="support-links">` +
		vkCard("https://vk.ru/repetitor_penza_etalon", "Группа", "Эталон") +
		vkCard("https://vk.ru/ibst_pgu", "Группа", "ПГУ") +
		`</div>` +
		`</div>`
}

func teamStatsHTML(database *sql.DB, teamID int, ru bool) string {

	teamName := "Unknown"
	teamDesc := ""
	teamScore := 0
	teamRank := 0
	for i, t := range scoreCache {
		if t.ID == teamID {
			teamName = html.EscapeString(t.Name)
			teamDesc = html.EscapeString(t.Desc)
			teamScore = t.Score
			teamRank = i + 1
			break
		}
	}
	if teamName == "Unknown" {
		if t, terr := db.GetTeam(database, teamID); terr == nil {
			teamName = html.EscapeString(t.Name)
			teamDesc = html.EscapeString(t.Desc)
		}
	}

	cats, err := gameShim.Tasks()
	if err != nil {
		log.Println("teamStatsHTML: Tasks:", err)
	}

	type taskMeta struct {
		Name    string
		CatName string
		Price   int
	}
	taskMap := make(map[int]taskMeta)
	catTotalMap := make(map[string]int)
	for _, cat := range cats {
		for _, t := range cat.TasksInfo {
			name := t.Name
			if !ru {
				name = t.NameEn
			}
			catKey := html.EscapeString(cat.Name)
			taskMap[t.ID] = taskMeta{Name: html.EscapeString(name), CatName: catKey, Price: t.Price}
			catTotalMap[catKey]++
		}
	}

	flags, err := db.GetFlags(database)
	if err != nil {
		log.Println("teamStatsHTML: GetFlags:", err)
	}

	type solveEvent struct {
		TaskID    int
		Timestamp time.Time
	}
	var solvedEvents []solveEvent
	catSolvedMap := make(map[string]int)
	for _, f := range flags {
		if f.TeamID == teamID && f.Solved {
			solvedEvents = append(solvedEvents, solveEvent{f.TaskID, f.Timestamp})
			if meta, ok := taskMap[f.TaskID]; ok {
				catSolvedMap[meta.CatName]++
			}
		}
	}
	sort.Slice(solvedEvents, func(i, j int) bool {
		return solvedEvents[i].Timestamp.After(solvedEvents[j].Timestamp)
	})

	catProgressHTML := ""
	catNames := make([]string, 0, len(catTotalMap))
	for n := range catTotalMap {
		catNames = append(catNames, n)
	}
	sort.Strings(catNames)
	for _, cn := range catNames {
		total := catTotalMap[cn]
		solved := catSolvedMap[cn]
		pct := 0
		if total > 0 {
			pct = (solved * 100) / total
		}
		catProgressHTML += fmt.Sprintf(
			`<div class="cat-prog-row">`+
				`<div class="cat-prog-head">`+
				`<span class="cat-prog-name">%s</span>`+
				`<span class="cat-prog-count">%d/%d</span>`+
				`</div>`+
				`<div class="cat-prog-track">`+
				`<div class="cat-prog-fill" style="width:%d%%"></div>`+
				`</div>`+
				`</div>`,
			cn, solved, total, pct)
	}
	if catProgressHTML == "" {
		catProgressHTML = `<div class="profile-empty">Нет данных</div>`
	}

	solvedLogHTML := ""
	if len(solvedEvents) == 0 {
		solvedLogHTML = `<div class="profile-empty">Ещё нет решённых заданий</div>`
	} else {
		for _, ev := range solvedEvents {
			meta := taskMap[ev.TaskID]
			ts := ev.Timestamp.Local().Format("15:04")
			solvedLogHTML += fmt.Sprintf(
				`<div class="solved-log-row">`+
					`<span class="solved-log-time">%s</span>`+
					`<div class="solved-log-task">`+
					`<span class="solved-log-name">%s</span>`+
					` <span class="solved-log-cat">· %s</span>`+
					`</div>`+
					`<span class="solved-log-pts">+%d</span>`+
					`</div>`,
				ts, meta.Name, meta.CatName, meta.Price)
		}
	}

	rankClass := ""
	rankWreath := ""
	switch teamRank {
	case 1:
		rankClass = "rank-gold"
		rankWreath = wreathSVG[0]
	case 2:
		rankClass = "rank-silver"
		rankWreath = wreathSVG[1]
	case 3:
		rankClass = "rank-bronze"
		rankWreath = wreathSVG[2]
	}

	rankText := fmt.Sprintf("#%d", teamRank)
	if rankWreath != "" {
		rankText = fmt.Sprintf("%d", teamRank)
	}

	rankHTML := rankText
	if rankWreath != "" {
		rankHTML = `<span class="profile-rank-wrap">` + rankWreath +
			`<span class="profile-rank-num">` + rankText + `</span></span>`
	}

	descHTML := ""
	if teamDesc != "" {
		descHTML = fmt.Sprintf(`<div class="profile-card-school">%s</div>`, teamDesc)
	}

	return fmt.Sprintf(
		`<div class="profile-card">`+
			`<span class="profile-card-avatar">&gt;_</span>`+
			`<div class="profile-card-info">`+
			`<div class="profile-card-name">%s</div>`+
			`%s`+
			`</div>`+
			`<div class="profile-card-stats">`+
			`<div class="profile-card-stat">`+
			`<span class="profile-card-stat-val %s">%s</span>`+
			`<span class="profile-card-stat-lbl">место</span>`+
			`</div>`+
			`<div class="profile-card-stat">`+
			`<span class="profile-card-stat-val">%d</span>`+
			`<span class="profile-card-stat-lbl">очков</span>`+
			`</div>`+
			`<div class="profile-card-stat">`+
			`<span class="profile-card-stat-val">%d</span>`+
			`<span class="profile-card-stat-lbl">тасков</span>`+
			`</div>`+
			`</div>`+
			`</div>`+
			`<div class="profile-grid">`+
			`<div class="profile-panel">`+
			`<div class="profile-panel-title">Прогресс по категориям</div>`+
			`%s`+
			`</div>`+
			`<div class="profile-panel">`+
			`<div class="profile-panel-title">Решённые задания</div>`+
			`%s`+
			`</div>`+
			`</div>`,
		teamName, descHTML, rankClass, rankHTML,
		teamScore, len(solvedEvents),
		catProgressHTML, solvedLogHTML)
}

func profilePageHTML(database *sql.DB, teamID int, ru bool) string {
	return teamStatsHTML(database, teamID, ru) +
		scoringRulesHTML() +
		newsBlockHTML(database) +
		sponsorsBlockHTML()
}

func profileHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := getTeamID(r)

		tmpl, err := getTmpl("profile")
		if err != nil {
			log.Println("profile tmpl:", err)
			return
		}

		content := profilePageHTML(database, teamID, isAcceptRussian(r))
		fmt.Fprintf(w, tmpl,
			l10n(r, getInfo())+profileWidgetHTML(teamID),
			content)
	}
}

func teamPageHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		team, err := db.GetTeam(database, id)
		if err != nil || team.Test {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		tmpl, err := getTmpl("team")
		if err != nil {
			log.Println("team tmpl:", err)
			return
		}

		viewerID := getTeamID(r)
		fmt.Fprintf(w, tmpl,
			html.EscapeString(team.Name),
			l10n(r, getInfo())+profileWidgetHTML(viewerID),
			teamStatsHTML(database, id, isAcceptRussian(r)))
	}
}
