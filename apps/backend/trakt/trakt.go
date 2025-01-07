package trakt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odin-movieshow/backend/helpers"
	"github.com/odin-movieshow/backend/settings"
	"github.com/odin-movieshow/backend/tmdb"
	"github.com/odin-movieshow/backend/types"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	ptypes "github.com/pocketbase/pocketbase/tools/types"
	"github.com/thoas/go-funk"

	"github.com/go-resty/resty/v2"

	"github.com/charmbracelet/log"
)

const (
	TRAKT_URL = "https://api.trakt.tv"
)

type Trakt struct {
	app       *pocketbase.PocketBase
	tmdb      *tmdb.Tmdb
	settings  *settings.Settings
	helpers   *helpers.Helpers
	Headers   map[string]string
	FetchTMDB bool
}

func New(app *pocketbase.PocketBase, tmdb *tmdb.Tmdb, settings *settings.Settings, helpers *helpers.Helpers) *Trakt {
	return &Trakt{
		app:       app,
		tmdb:      tmdb,
		settings:  settings,
		helpers:   helpers,
		Headers:   map[string]string{},
		FetchTMDB: true,
	}
}

func (t *Trakt) removeDuplicates(objmap []types.TraktItem) []types.TraktItem {
	showsSeen := []uint{}
	toRemove := []int{}
	for i, o := range objmap {
		if o.Type != "episode" {
			continue
		}
		id := o.IDs.Trakt

		if o.Show != nil {
			id = o.Show.IDs.Trakt
		}
		if !funk.Contains(showsSeen, id) {
			showsSeen = append(showsSeen, id)
		} else {
			toRemove = append(toRemove, i)
		}
	}

	newmap := []types.TraktItem{}

	for i, o := range objmap {
		if !funk.ContainsInt(toRemove, i) {
			newmap = append(newmap, o)
		}
	}

	return newmap
}

func (t *Trakt) removeWatched(objmap []types.TraktItem) []types.TraktItem {
	return funk.Filter(objmap, func(o types.TraktItem) bool {
		return !o.Watched
	}).([]types.TraktItem)
}

func (t *Trakt) removeSeason0(objmap []types.TraktItem) []types.TraktItem {
	toKeep := []types.TraktItem{}
	for _, o := range objmap {
		if o.Number > 0 && o.Season > 0 {
			toKeep = append(toKeep, o)
		}
	}

	return toKeep
}

func (t *Trakt) SyncHistory() {
	users := []*models.Record{}
	t.app.Dao().RecordQuery("users").All(&users)
	t.FetchTMDB = false
	var wg sync.WaitGroup
	for _, u := range users {
		records, _ := t.app.Dao().FindRecordsByFilter("history", "user = {:user}", "-watched_at", 1, 0, dbx.Params{"user": u.Get("id")})
		last_watched := ptypes.DateTime{}
		if len(records) > 0 {
			last_watched = records[0].GetDateTime("watched_at")
		}

		token := make(map[string]any)
		if err := u.UnmarshalJSONField("trakt_token", &token); err != nil {
			continue
		}

		t.Headers["authorization"] = "Bearer " + token["access_token"].(string)

		wg.Add(1)
		go t.syncByType(&wg, "movies", last_watched, u.Get("id").(string))
		wg.Add(1)
		go t.syncByType(&wg, "episodes", last_watched, u.Get("id").(string))

		wg.Wait()
		log.Info("Done synching trakt history", "user", u.Get("id"))

	}
}

func (t *Trakt) syncByType(wg *sync.WaitGroup, typ string, last_history ptypes.DateTime, user string) {
	defer wg.Done()
	limit := 100
	url := "/sync/history/" + typ + "?limit=" + fmt.Sprint(limit)
	log.Debug(url)
	collection, _ := t.app.Dao().FindCollectionByNameOrId("history")
	if !last_history.IsZero() {
		url += "&start_at=" + last_history.Time().Add(time.Second*1).Format(time.RFC3339)
	}
	_, headers, _ := t.CallEndpoint(url, "GET", nil, false)
	pages, _ := strconv.Atoi(headers.Get("X-Pagination-Page-Count"))

	for i := 1; i <= pages; i++ {
		wg.Add(1)
		go func(i int, wg *sync.WaitGroup) {
			defer wg.Done()
			url += "&page=" + fmt.Sprint(i)

			data, _, _ := t.CallEndpoint(url, "GET", nil, false)

			for _, o := range data.([]types.TraktItem) {
				o.Original = nil
				o.Watched = true
				record := models.NewRecord(collection)
				record.Set("watched_at", o.WatchedAt)
				record.Set("user", user)
				record.Set("type", o.Type)
				record.Set("trakt_id", o.IDs.Trakt)
				record.Set("runtime", o.Runtime)
				switch typ {
				case "movies":
					record.Set("data", o)
				case "episodes":
					record.Set("show_id", o.Show.IDs.Trakt)
					record.Set("data", o.Show)
				}
				t.app.Dao().SaveRecord(record)

			}
		}(i, wg)
	}
}

func (t *Trakt) RefreshTokens() {
	records := []*models.Record{}
	t.app.Dao().RecordQuery("users").All(&records)

	for _, r := range records {
		token := make(map[string]any)
		if err := r.UnmarshalJSONField("trakt_token", &t); err == nil {
			data, _, status := t.CallEndpoint("/oauth/token", "POST", map[string]any{"grant_type": "refresh_token", "client_id": os.Getenv("TRAKT_CLIENTID"), "client_secret": os.Getenv("TRAKT_SECRET"), "code": token["device_code"], "refresh_token": token["refresh_token"]}, false)
			if status < 300 && data != nil {
				data.(map[string]any)["device_code"] = token["device_code"]
				r.Set("trakt_token", data)
				t.app.Dao().Save(r)
				log.Info("trakt refresh token", "user", r.Get("id"))
			}
		}

	}
}

func (t *Trakt) normalize(objmap []types.TraktItem, isShow bool) []types.TraktItem {
	for i, o := range objmap {
		if o.Movie != nil || o.Episode != nil || o.Show != nil {
			m := types.TraktItem{}
			if o.Movie != nil {
				m = *o.Movie
				m.Movie = nil
				m.Type = "movie"
			} else {
				if o.Episode != nil && o.Show != nil {
					m = *o.Episode
					m.Episode = nil
					m.Show = o.Show
					m.Type = "episode"
				} else {
					m = *o.Show
					m.Show = nil
					m.Type = "show"
				}
			}
			orig := *o.Original
			if orig.(map[string]any)[m.Type] != nil {
				orig = (*o.Original).(map[string]any)[m.Type]
			}
			m.Original = &orig
			m.WatchedAt = o.WatchedAt
			objmap[i] = m
		} else {
			t := "movie"
			if isShow {
				t = "show"
			}
			if objmap[i].Episodes != nil {
				t = "season"
			}
			objmap[i].Type = t
		}
	}
	return objmap
}

func (t *Trakt) objToItems(objmap []any, isShow bool) []types.TraktItem {
	jm, err := json.Marshal(objmap)
	if err == nil {
		items := []types.TraktItem{}
		err = json.Unmarshal(jm, &items)

		if err == nil {
			if len(items) == 0 {
				return items
			}
			for i, item := range items {
				items[i].Original = &objmap[i]
				if item.Show != nil {
					sorig := objmap[i].(map[string]any)["show"]
					(*items[i].Show).Original = &sorig
				}
				if item.Episodes != nil && len(*item.Episodes) > 0 {
					for e := range *item.Episodes {
						(*items[i].Episodes)[e].Original = &objmap[i].(map[string]any)["episodes"].([]any)[e]
					}
				}
			}
			return t.normalize(items, isShow)
		}
	}
	return []types.TraktItem{}
}

func (t *Trakt) itemsToObj(items []types.TraktItem) []map[string]any {
	m, err := json.Marshal(items)
	o := []map[string]any{}
	if err != nil {
		return o
	}

	err = json.Unmarshal(m, &o)
	if err != nil {
		return o
	}

	for i := range o {
		orig := items[i].Original
		for k, v := range (*orig).(map[string]any) {
			if o[i][k] == nil && v != nil {
				o[i][k] = v
			}
		}
		o[i]["original"] = nil
		o[i]["movie"] = nil
		if o[i]["episode"] != nil {
			o[i]["episode"] = nil
		}
		if items[i].Show != nil {
			for k, v := range (*items[i].Show.Original).(map[string]any) {
				if o[i]["show"].(map[string]any)[k] == nil {
					o[i]["show"].(map[string]any)[k] = v
				}
			}
			o[i]["show"].(map[string]any)["original"] = nil
		}

		if items[i].Episodes != nil && len(*items[i].Episodes) > 0 {
			for e, ep := range *items[i].Episodes {
				for k, v := range (*ep.Original).(map[string]any) {
					if o[i]["episodes"].([]any)[e].(map[string]any)[k] == nil {
						o[i]["episodes"].([]any)[e].(map[string]any)[k] = v
					}
				}
				o[i]["episodes"].([]any)[e].(map[string]any)["original"] = nil
			}
		}
	}

	return o
}

func (t *Trakt) CallEndpoint(endpoint string, method string, body map[string]any, donorm bool) (any, http.Header, int) {
	var objmap any

	hs := t.Headers

	request := resty.New().SetRetryCount(3).SetRetryWaitTime(time.Second * 3).R()
	request.SetHeader("trakt-api-version", "2").SetHeader("content-type", "application/json").SetHeader("trakt-api-key", os.Getenv("TRAKT_CLIENTID")).AddRetryCondition(func(r *resty.Response, err error) bool {
		return r.StatusCode() == 401
	}).SetHeaders(hs)

	var respHeaders http.Header
	status := 200
	if body != nil {
		request.SetBody(body)
	}
	request.Attempt = 3
	var r func(url string) (*resty.Response, error)
	switch method {
	case "POST":
		r = request.Post
	case "PATCH":
		r = request.Patch
	case "PUT":
		r = request.Put
	case "DELETE":
		r = request.Delete
	default:
		r = request.Get

	}

	if !strings.Contains(endpoint, "oauth") {
		if !strings.Contains(endpoint, "extended=") {
			if strings.Contains(endpoint, "?") {
				endpoint += "&"
			} else {
				endpoint += "?"
			}
			endpoint += "extended=full&limit=30"
			if !strings.Contains(endpoint, "limit=") {
				endpoint += "&limit=30"
			}
		}
	}

	if resp, err := r(fmt.Sprintf("%s%s", TRAKT_URL, endpoint)); err == nil {
		respHeaders = resp.Header()
		status = resp.StatusCode()
		log.Info("trakt fetch", "url", endpoint, "method", method, "status", status)
		if status > 299 {
			log.Error("trakt", "fetch", endpoint, "status", status)
			// log.Debug("trakt", "fetch", endpoint, "status", status, "res", string(resp.Body()), "body", body, "headers", respHeaders)
		}
		err := json.Unmarshal(resp.Body(), &objmap)
		if err != nil {
			log.Error("trakt", "unmarshal", err)
		}

		switch objmap := objmap.(type) {

		case []any:
			items := t.objToItems(objmap, strings.Contains(endpoint, "/shows"))
			var wg sync.WaitGroup
			var mux sync.Mutex

			if len(items) == 0 || strings.Contains(endpoint, "sync/history") {
				if t.FetchTMDB {
					t.getTMDB(&wg, &mux, items)
				}
				wg.Wait()
				return items, respHeaders, status
			}

			t.GetWatched(items)

			if strings.Contains(endpoint, "calendars") {
				items = t.removeSeason0(items)
				items = t.removeWatched(items)
				items = t.removeDuplicates(items)
			}

			if t.FetchTMDB {
				t.getTMDB(&wg, &mux, items)
			}

			wg.Wait()

			return t.itemsToObj(items), respHeaders, status

		default:

		}

	} else {
		log.Error("trakt", "endpoint", endpoint, "body", body, "err", err)
	}

	return objmap, respHeaders, status
}

func (t *Trakt) getTMDB(wg *sync.WaitGroup, _ *sync.Mutex, objmap []types.TraktItem) {
	for k := range objmap {
		wg.Add(1)
		go func() {
			t.tmdb.PopulateTMDB(k, objmap)
			wg.Done()
		}()
	}
	wg.Wait()
}

type Watched struct {
	LastWatchedAt time.Time `json:"last_watched_at"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
	Seasons       []struct {
		Episodes []struct {
			LastWatchedAt time.Time `json:"last_watched_at"`
			Number        int       `json:"number"`
			Plays         int       `json:"plays"`
		} `json:"episodes"`
		Number int `json:"number"`
	} `json:"seasons"`
	Movie types.TraktItem `json:"movie"`
	Show  types.TraktItem `json:"show"`
	Plays int             `json:"plays"`
}

func (t *Trakt) GetWatched(objmap []types.TraktItem) []types.TraktItem {
	if len(objmap) == 0 {
		return objmap
	}
	return t.AssignWatched(objmap, objmap[0].Type)
}

func (t *Trakt) getHistory(htype string) []any {
	records, _ := t.app.Dao().FindRecordsByFilter("history", "type = {:htype}", "-watched_at", -1, 0, dbx.Params{"htype": htype})
	data := make([]any, 0)
	for _, r := range records {
		item := make(map[string]any)
		item["type"] = r.Get("type")
		item["trakt_id"] = r.Get("trakt_id")
		data = append(data, item)
	}
	return data
}

func (t *Trakt) AssignWatched(objmap []types.TraktItem, typ string) []types.TraktItem {
	if typ == "season" {
		typ = "episode"
	}
	history := t.getHistory(typ)
	for i, o := range objmap {
		if o.Episodes != nil {
			for j, e := range *o.Episodes {
				oid := e.IDs.Trakt
				(*objmap[i].Episodes)[j].Watched = false
				for _, h := range history {
					hid := uint(h.(map[string]any)["trakt_id"].(float64))
					if hid == oid {
						(*objmap[i].Episodes)[j].Watched = true
						log.Debug(oid, "watched", (*objmap[i].Episodes)[j].Watched)
						break
					}
				}
			}
		} else {

			oid := o.IDs.Trakt
			objmap[i].Watched = false
			for _, h := range history {
				hid := uint(h.(map[string]any)["trakt_id"].(float64))
				if hid == oid {
					objmap[i].Watched = true
					break
				}
			}
		}
	}
	newmap := make([]types.TraktItem, 0)

	for _, o := range objmap {
		newmap = append(newmap, o)
	}

	return newmap
}

func (t *Trakt) GetSeasons(id int) any {
	// cache := t.helpers.ReadTraktSeasonCache(uint(id))
	// if cache != nil {
	// 	return cache
	// }
	endpoint := fmt.Sprintf("/shows/%d/seasons?extended=full,episodes", id)
	result, _, _ := t.CallEndpoint(endpoint, "GET", nil, false)
	t.helpers.WriteTraktSeasonCache(uint(id), &result)
	return result
}
