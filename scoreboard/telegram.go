package scoreboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jollheef/henhouse/db"
)

const (
	tgSettingEnabled = "tg_enabled"
	tgSettingToken   = "tg_bot_token"
	tgSettingChatID  = "tg_chat_id"

	tgMaxMessageRunes = 4000
	tgQueueSize       = 256
	tgSendAttempts    = 3
)

var (
	tgAPIBase = "https://api.telegram.org"

	tgClient = &http.Client{Timeout: 15 * time.Second}

	tgQueue chan int

	tgRetryDelays = [...]time.Duration{3 * time.Second, 10 * time.Second}
)

type tgConfig struct {
	Enabled bool
	Token   string
	ChatID  string
}

func (c tgConfig) ready() bool {
	return c.Enabled && c.Token != "" && c.ChatID != ""
}

func tgLoadConfig(database *sql.DB) (c tgConfig) {
	enabled, err := db.GetSetting(database, tgSettingEnabled)
	if err != nil {
		log.Println("[tg] read enabled:", err)
	}
	c.Enabled = enabled == "1"

	c.Token, err = db.GetSetting(database, tgSettingToken)
	if err != nil {
		log.Println("[tg] read token:", err)
	}
	c.Token = strings.TrimSpace(c.Token)

	c.ChatID, err = db.GetSetting(database, tgSettingChatID)
	if err != nil {
		log.Println("[tg] read chat id:", err)
	}
	c.ChatID = strings.TrimSpace(c.ChatID)

	return
}

type tgAPIError struct {
	Code int
	Desc string
}

func (e *tgAPIError) Error() string {
	if e.Desc != "" {
		return e.Desc
	}
	return fmt.Sprintf("telegram api error %d", e.Code)
}

func tgPermanent(err error) bool {
	_, ok := err.(*tgAPIError)
	return ok
}

func tgRedact(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), token, "***")
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}

type tgAPIResp struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func tgParseResp(resp *http.Response) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}

	var r tgAPIResp
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("telegram: bad response (http %d)",
			resp.StatusCode)
	}

	if !r.OK {
		return &tgAPIError{Code: r.ErrorCode, Desc: r.Description}
	}

	return nil
}

func tgSendMessage(token, chatID, text string) error {
	if r := []rune(text); len(r) > tgMaxMessageRunes {
		text = string(r[:tgMaxMessageRunes]) + "…"
	}

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", text)
	params.Set("disable_web_page_preview", "true")

	resp, err := tgClient.PostForm(
		tgAPIBase+"/bot"+token+"/sendMessage", params)
	if err != nil {
		return tgRedact(err, token)
	}

	return tgParseResp(resp)
}

func tgSendDocument(token, chatID, path, caption string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		var werr error
		defer func() { pw.CloseWithError(werr) }()

		if werr = mw.WriteField("chat_id", chatID); werr != nil {
			return
		}
		if caption != "" {
			if werr = mw.WriteField("caption", caption); werr != nil {
				return
			}
		}

		var part io.Writer
		part, werr = mw.CreateFormFile("document", filepath.Base(path))
		if werr != nil {
			return
		}
		if _, werr = io.Copy(part, f); werr != nil {
			return
		}

		werr = mw.Close()
	}()

	resp, err := tgClient.Post(tgAPIBase+"/bot"+token+"/sendDocument",
		mw.FormDataContentType(), pr)
	if err != nil {
		return tgRedact(err, token)
	}

	return tgParseResp(resp)
}

func tgSendWithRetry(send func() error) (err error) {
	for attempt := 0; attempt < tgSendAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(tgRetryDelays[attempt-1])
		}

		err = send()
		if err == nil || tgPermanent(err) {
			return
		}

		log.Printf("[tg] attempt %d failed: %v", attempt+1, err)
	}
	return
}

func tgSupportMessage(req db.SupportRequest, teamName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Новое обращение #%d\n", req.ID)
	fmt.Fprintf(&b, "Команда: %s (id %d)\n", teamName, req.TeamID)

	ptype := req.Type
	if ptype == "" {
		ptype = "не указан"
	}
	fmt.Fprintf(&b, "Тип: %s\n", ptype)

	if req.Contact != "" {
		fmt.Fprintf(&b, "Контакт: %s\n", req.Contact)
	}
	if req.Attach != "" {
		fmt.Fprintf(&b, "Вложение: %s\n", req.Attach)
	}

	fmt.Fprintf(&b, "Время: %s\n\n",
		req.Timestamp.Local().Format("02.01.2006 15:04"))

	b.WriteString(req.Text)

	return b.String()
}

func tgStatusStamp(prefix string) string {
	return prefix + " " + time.Now().Format("02.01 15:04")
}

func tgEnqueue(database *sql.DB, id int) {
	select {
	case tgQueue <- id:
		if err := db.SetSupportTgStatus(database, id,
			"в очереди"); err != nil {
			log.Println("[tg] set status:", err)
		}
	default:
		log.Printf("[tg] queue full, request %d not sent", id)
		if err := db.SetSupportTgStatus(database, id,
			"ошибка: очередь отправки переполнена"); err != nil {
			log.Println("[tg] set status:", err)
		}
	}
}

func tgSetStatus(database *sql.DB, id int, status string) {
	if err := db.SetSupportTgStatus(database, id, status); err != nil {
		log.Println("[tg] set status:", err)
	}
}

func tgDeliver(database *sql.DB, id int) {
	req, err := db.GetSupportRequest(database, id)
	if err != nil {
		log.Printf("[tg] request %d not found: %v", id, err)
		return
	}

	cfg := tgLoadConfig(database)
	if !cfg.ready() {
		if cfg.Token == "" || cfg.ChatID == "" {
			tgSetStatus(database, id,
				"не отправлено: не заполнены токен или chat_id")
		} else {
			tgSetStatus(database, id,
				"не отправлено: уведомления выключены в настройках")
		}
		return
	}

	teamName := fmt.Sprintf("команда #%d", req.TeamID)
	if team, terr := db.GetTeam(database, req.TeamID); terr == nil {
		teamName = team.Name
	}

	text := tgSupportMessage(req, teamName)

	err = tgSendWithRetry(func() error {
		return tgSendMessage(cfg.Token, cfg.ChatID, text)
	})
	if err != nil {
		log.Printf("[tg] request %d send failed: %v", id, err)
		tgSetStatus(database, id, "ошибка: "+err.Error())
		return
	}

	if req.Attach != "" {
		path := supportAttachPath(req.ID, req.Attach)
		if path != "" {
			if _, serr := os.Stat(path); serr == nil {
				err = tgSendWithRetry(func() error {
					return tgSendDocument(cfg.Token, cfg.ChatID,
						path, "Вложение к обращению #"+
							strconv.Itoa(req.ID))
				})
			} else {
				err = serr
			}
			if err != nil {
				log.Printf("[tg] request %d attach failed: %v",
					id, err)
				tgSetStatus(database, id, tgStatusStamp(
					"отправлено без вложения"))
				return
			}
		}
	}

	log.Printf("[tg] request %d delivered", id)
	tgSetStatus(database, id, tgStatusStamp("отправлено"))
}

func tgWorker(database *sql.DB) {
	for id := range tgQueue {
		tgDeliver(database, id)
	}
}

func tgStart(database *sql.DB) {
	if v := os.Getenv("HENHOUSE_TG_API"); v != "" {
		tgAPIBase = strings.TrimRight(v, "/")
		log.Println("[tg] api base overridden:", tgAPIBase)
	}

	tgQueue = make(chan int, tgQueueSize)
	go tgWorker(database)
}
