package game

import (
	"database/sql"
	"log"
	"math"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/jollheef/henhouse/db"
)

type Game struct {
	db              *sql.DB
	Start           time.Time
	End             time.Time
	OpenTimeout     time.Duration
	AutoOpen        bool
	AutoOpenTimeout time.Duration
	scoreboardLock  *sync.Mutex
	TaskPrice       struct {
		TeamsBase              float64
		P500, P400, P300, P200 float64
	}
}

type TaskInfo struct {
	ID          int
	Name        string
	Desc        string
	NameEn      string
	DescEn      string
	Tags        string
	Author      string
	Price       int
	Opened      bool
	Level       int
	ForceClosed bool
	SolvedBy    []int
	OpenedTime  time.Time
}

type CategoryInfo struct {
	Name      string
	TasksInfo []TaskInfo
}

type TeamScoreInfo struct {
	ID         int
	Name       string
	Desc       string
	Score      int
	LastAccept int64
}

type ByScoreAndLastAccept []TeamScoreInfo

func (tr ByScoreAndLastAccept) Len() int      { return len(tr) }
func (tr ByScoreAndLastAccept) Swap(i, j int) { tr[i], tr[j] = tr[j], tr[i] }
func (tr ByScoreAndLastAccept) Less(i, j int) bool {
	if tr[i].Score == tr[j].Score {
		return tr[i].LastAccept < tr[j].LastAccept
	}
	return tr[i].Score > tr[j].Score
}

type byLevel []TaskInfo

func (ti byLevel) Len() int           { return len(ti) }
func (ti byLevel) Swap(i, j int)      { ti[i], ti[j] = ti[j], ti[i] }
func (ti byLevel) Less(i, j int) bool { return ti[i].Level < ti[j].Level }

func CalcTeamsBase(database *sql.DB) (z float64, err error) {

	teams, err := db.GetTeams(database)
	if err != nil {
		return
	}

	flags, err := db.GetFlags(database)
	if err != nil {
		return
	}

	k := []int{}
	for _, t := range teams {
		solvedCount := 0
		for _, f := range flags {

			if f.TeamID == t.ID && f.Solved {
				solvedCount++
			}
		}
		k = append(k, solvedCount)
	}

	max := 1
	for _, ki := range k {
		if ki > max {
			max = ki
		}
	}

	for _, ki := range k {
		z += math.Sqrt(float64(ki) / float64(max))
	}

	if z < 21 {
		z = 21
	}

	return
}

func NewGame(database *sql.DB, start, end time.Time,
	teamBase float64) (g Game, err error) {

	g.db = database
	g.Start = start
	g.End = end

	g.TaskPrice.P200 = 0.50
	g.TaskPrice.P300 = 0.30
	g.TaskPrice.P400 = 0.15
	g.TaskPrice.P500 = 0.10

	g.scoreboardLock = &sync.Mutex{}

	g.TaskPrice.TeamsBase = teamBase

	_, err = g.Scoreboard()
	if err != nil {
		err = g.RecalcScoreboard()
		if err != nil {
			return
		}
	}

	return
}

func (g *Game) SetTaskPrice(p500, p400, p300, p200 int) {
	g.TaskPrice.P200 = float64(p200) / 100
	g.TaskPrice.P300 = float64(p300) / 100
	g.TaskPrice.P400 = float64(p400) / 100
	g.TaskPrice.P500 = float64(p500) / 100
}

func (g *Game) SetTeamsBase(teams int) {
	g.TaskPrice.TeamsBase = float64(teams)
}

func (g *Game) TeamsBaseUpdater(database *sql.DB, updateTimeout time.Duration) {
	for {
		z, err := CalcTeamsBase(database)
		if err != nil {
			return
		}

		log.Println("Set teams base to", z)
		g.TaskPrice.TeamsBase = z

		time.Sleep(updateTimeout)
	}
}

func (g Game) Run() (err error) {

	for time.Now().Before(g.Start) {
		time.Sleep(time.Second)
	}

	cats, err := g.Tasks()
	if err != nil {
		return
	}

	for _, c := range cats {
		for _, t := range c.TasksInfo {
			if t.ForceClosed {
				continue
			}

			log.Println("Open task", t.Name, t.Level)
			err = db.SetOpened(g.db, t.ID, true)
			if err != nil {
				return
			}

			break
		}
	}

	if !g.AutoOpen {
		return
	}

	go func() {
		for {
			time.Sleep(time.Second)
			err = g.autoOpenTasks()
			if err != nil {
				log.Println("Auto open tasks fail:", err)
			}
		}
	}()

	return
}

func (g Game) autoOpenTasks() (err error) {

	now := time.Now()

	cats, err := g.Tasks()
	if err != nil {
		return
	}

	for _, c := range cats {
		prev := TaskInfo{Opened: true}
		for i, t := range c.TasksInfo {
			if i == 0 || t.Opened || !prev.Opened || t.ForceClosed {
				prev = t
				continue
			}

			if now.After(prev.OpenedTime.Add(g.AutoOpenTimeout)) {
				log.Println("Open task", t.Name, t.Level)
				err = db.SetOpened(g.db, t.ID, true)
				if err != nil {
					return
				}
			}

			prev = t
		}

	}

	return
}

func (g *Game) taskPrice(database *sql.DB, taskID int) (price int, err error) {

	count, err := db.GetSolvedCount(database, taskID)

	fprice := float64(count) / g.TaskPrice.TeamsBase

	if fprice <= g.TaskPrice.P500 {
		price = 500
	} else if fprice <= g.TaskPrice.P400 {
		price = 400
	} else if fprice <= g.TaskPrice.P300 {
		price = 300
	} else if fprice <= g.TaskPrice.P200 {
		price = 200
	} else {
		price = 100
	}

	return
}

func (g Game) Tasks() (cats []CategoryInfo, err error) {

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	categories, err := db.GetCategories(g.db)
	if err != nil {
		return
	}

	for _, category := range categories {

		cat := CategoryInfo{Name: category.Name}

		for _, task := range tasks {

			if task.CategoryID == category.ID {

				var price int
				price, err = g.taskPrice(g.db, task.ID)
				if err != nil {
					return
				}

				var solvedBy []int
				solvedBy, err = db.GetSolvedBy(g.db, task.ID)
				if err != nil {
					return
				}

				if !task.Opened {
					task.Desc = ""
				}

				tInfo := TaskInfo{
					ID:          task.ID,
					Name:        task.Name,
					Desc:        task.Desc,
					NameEn:      task.NameEn,
					DescEn:      task.DescEn,
					Tags:        task.Tags,
					Price:       price,
					Opened:      task.Opened,
					SolvedBy:    solvedBy,
					Author:      task.Author,
					Level:       task.Level,
					ForceClosed: task.ForceClosed,
					OpenedTime:  task.OpenedTime,
				}

				cat.TasksInfo = append(cat.TasksInfo, tInfo)
			}
		}

		sort.Sort(byLevel(cat.TasksInfo))

		cats = append(cats, cat)
	}

	return
}

func LastAccept(teamID int, flags []db.Flag) int64 {
	timestamp := time.Unix(0, 0)
	for _, f := range flags {

		if f.TeamID == teamID && f.Solved && f.Timestamp.After(timestamp) {
			timestamp = f.Timestamp
		}
	}
	return timestamp.Unix()
}

func (g Game) Scoreboard() (scores []TeamScoreInfo, err error) {

	g.scoreboardLock.Lock()
	defer g.scoreboardLock.Unlock()

	teams, err := db.GetTeams(g.db)
	if err != nil {
		return
	}

	flags, err := db.GetFlags(g.db)
	if err != nil {
		return
	}

	for _, team := range teams {

		if team.Test {
			continue
		}

		var s db.Score
		s, err = db.GetLastScore(g.db, team.ID)
		if err == sql.ErrNoRows {

			s.Score = 0
			err = nil
		} else if err != nil {
			return
		}

		scores = append(scores, TeamScoreInfo{
			ID:         team.ID,
			Name:       team.Name,
			Desc:       team.Desc,
			Score:      s.Score,
			LastAccept: LastAccept(team.ID, flags),
		})
	}

	sort.Sort(ByScoreAndLastAccept(scores))

	return
}

func (g Game) RecalcScoreboard() (err error) {

	g.scoreboardLock.Lock()
	defer g.scoreboardLock.Unlock()

	teams, err := db.GetTeams(g.db)
	if err != nil {
		return
	}

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, team := range teams {

		if team.Test {
			continue
		}

		score := 0

		for _, task := range tasks {

			var price int
			price, err = g.taskPrice(g.db, task.ID)
			if err != nil {
				return
			}

			var solved bool
			solved, err = db.IsSolved(g.db, team.ID, task.ID)
			if err != nil {
				return
			}

			if solved {
				score += price
			}
		}

		err = db.AddScore(g.db, &db.Score{TeamID: team.ID, Score: score})
		if err != nil {
			return
		}
	}

	return
}

func (g Game) OpenNextTask(t db.Task) (err error) {

	time.Sleep(g.OpenTimeout)

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, task := range tasks {

		if t.CategoryID == task.CategoryID && t.Level+1 == task.Level {

			if !task.Opened && !task.ForceClosed {

				log.Println("Open task", task.Name, task.Level)
				err = db.SetOpened(g.db, task.ID, true)
				if err != nil {
					return
				}
			}
		}
	}

	return
}

func (g Game) isTestTeam(teamID int) bool {

	teams, err := db.GetTeams(g.db)
	if err != nil {
		log.Println("Get teams fail:", err)
		return true
	}

	for _, team := range teams {
		if team.ID == teamID {
			return team.Test
		}
	}

	return false
}

func (g Game) Solve(teamID, taskID int, flag string) (solved bool, err error) {

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, task := range tasks {
		if task.ID == taskID {

			solved, err = regexp.MatchString("^("+task.Flag+")$", flag)
			if err != nil {
				log.Println("Match regex fail:", err)
				return
			}

			if g.isTestTeam(teamID) {
				return
			}

			now := time.Now()
			inGame := now.After(g.Start) && now.Before(g.End)

			if solved {

				var isSolv bool
				isSolv, err = db.IsSolved(g.db, teamID, taskID)
				if isSolv {
					return
				}

				if inGame {
					err = db.AddFlag(g.db, &db.Flag{
						TeamID: teamID,
						TaskID: taskID,
						Flag:   flag,
						Solved: solved,
					})
					if err != nil {
						return
					}

					go g.OpenNextTask(task)
				}
			} else if inGame {

				ferr := db.AddFlag(g.db, &db.Flag{
					TeamID: teamID,
					TaskID: taskID,
					Flag:   flag,
					Solved: false,
				})
				if ferr != nil {
					log.Println("Add failed attempt:", ferr)
				}
			}

			break
		}
	}

	return
}
