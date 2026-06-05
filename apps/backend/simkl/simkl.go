package simkl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odin-movieshow/backend/cache"
	"github.com/odin-movieshow/backend/common"
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
	URL = "https://api.simkl.com"
)

type Simkl struct {
	app      *pocketbase.PocketBase
	tmdb     *tmdb.Tmdb
	settings *settings.Settings
	cache    *cache.Cache
}

func New(app *pocketbase.PocketBase, tmdb *tmdb.Tmdb, settings *settings.Settings, cache *cache.Cache) *Simkl {
	return &Simkl{
		app:      app,
		tmdb:     tmdb,
		settings: settings,
		cache:    cache,
	}
}

func (t *Simkl) RemoveDuplicates(objmap []types.SimklItem) []types.SimklItem {
	showsSeen := []uint{}
	toRemove := []int{}
	for i, o := range objmap {
		if o.Type != "episode" {
			continue
		}
		id := o.IDs.SimklID

		if o.Show != nil {
			id = o.Show.IDs.SimklID
		}
		if !funk.Contains(showsSeen, id) {
			showsSeen = append(showsSeen, id)
		} else {
			toRemove = append(toRemove, i)
		}
	}

	newmap := []types.SimklItem{}

	for i, o := range objmap {
		if !funk.ContainsInt(toRemove, i) {
			newmap = append(newmap, o)
		}
	}

	return newmap
}

func (t *Simkl) removeWatched(objmap []types.SimklItem) []types.SimklItem {
	return funk.Filter(objmap, func(o types.SimklItem) bool {
		return !o.Watched
	}).([]types.SimklItem)
}

func (t *Simkl) removeSeason0(objmap []types.SimklItem) []types.SimklItem {
	toKeep := []types.SimklItem{}
	for _, o := range objmap {
		if o.Number > 0 && o.Season > 0 {
			toKeep = append(toKeep, o)
		}
	}

	return toKeep
}

func (t *Simkl) SyncHistory(id string) {
	users := []*models.Record{}
	t.app.Dao().RecordQuery("users").All(&users)
	var wg sync.WaitGroup
	for _, u := range users {
		if id != "" && u.Id != id {
			continue
		}
		records, _ := t.app.Dao().FindRecordsByFilter("history", "user = {:user}", "-watched_at", 1, 0, dbx.Params{"user": u.Get("id")})
		last_watched := ptypes.DateTime{}
		if len(records) > 0 {
			last_watched = records[0].GetDateTime("watched_at")
		}

		token := make(map[string]any)
		if err := u.UnmarshalJSONField("simkl_token", &token); err != nil {
			continue
		}

		wg.Add(1)
		go t.syncByType(&wg, "movies", last_watched, u.Get("id").(string), "Bearer "+token["access_token"].(string))
		wg.Add(1)
		go t.syncByType(&wg, "episodes", last_watched, u.Get("id").(string), "Bearer "+token["access_token"].(string))

		wg.Wait()
		log.Info("Done synching simkl history", "user", u.Get("id"))

	}
}

func (t *Simkl) syncByType(wg *sync.WaitGroup, typ string, last_history ptypes.DateTime, user string, accesToken string) {
	defer wg.Done()
	limit := 200
	url := "/sync/history/" + typ + "?limit=" + fmt.Sprint(limit)
	collection, _ := t.app.Dao().FindCollectionByNameOrId("history")
	if !last_history.IsZero() {
		url += "&start_at=" + last_history.Time().Add(time.Second*1).Format(time.RFC3339)
	}
	_, headers, _ := t.CallEndpoint(url, "GET", types.SimklParams{Headers: map[string]string{"authorization": accesToken}})
	pages, _ := strconv.Atoi(headers.Get("X-Pagination-Page-Count"))

	for i := 1; i <= pages; i++ {
		// wg.Add(1)
		// go func(i int, wg *sync.WaitGroup) {
		// defer wg.Done()
		pageurl := url + "&page=" + fmt.Sprint(i)

		data, _, _ := t.CallEndpoint(pageurl, "GET", types.SimklParams{Headers: map[string]string{"authorization": accesToken}})
		if data == nil {
			continue
		}
		for _, o := range data.([]types.SimklItem) {
			o.Original = nil
			o.Watched = true
			record := models.NewRecord(collection)
			record.Set("watched_at", o.WatchedAt)
			record.Set("user", user)
			record.Set("type", o.Type)
			record.Set("simkl_id", o.IDs.SimklID)
			record.Set("runtime", o.RuntimeStr)
			switch typ {
			case "movies":
				record.Set("data", o)
			case "episodes":
				record.Set("show_id", o.Show.IDs.SimklID)
				record.Set("data", o.Show)
			}
			t.app.Dao().SaveRecord(record)

		}
		// }(i, wg)
	}
}

func (t *Simkl) RefreshTokens() {
	records := []*models.Record{}
	t.app.Dao().RecordQuery("users").All(&records)

	for _, r := range records {
		token := make(map[string]any)
		if err := r.UnmarshalJSONField("simkl_token", &token); err == nil {
			data, _, status := t.CallEndpoint("/oauth/token", "POST", types.SimklParams{Body: map[string]any{"grant_type": "refresh_token", "client_id": os.Getenv("simkl_CLIENTID"), "client_secret": os.Getenv("simkl_SECRET"), "code": token["device_code"], "refresh_token": token["refresh_token"]}})
			if status < 300 && data != nil {
				data.(map[string]any)["device_code"] = token["device_code"]
				r.Set("simkl_token", data)
				t.app.Dao().Save(r)
				log.Info("simkl refresh token", "user", r.Get("id"))
			}
		}

	}
}

func (t *Simkl) normalize(objmap []types.SimklItem, isShow bool) []types.SimklItem {
	for i, o := range objmap {
		if o.Movie != nil || o.Episode != nil || o.Show != nil {
			m := types.SimklItem{}
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

func (t *Simkl) ObjToItems(objmap []any, isShow bool) []types.SimklItem {
	jm, err := json.Marshal(objmap)
	items := []types.SimklItem{}
	if err != nil {
		return items
	}

	err = json.Unmarshal(jm, &items)

	if err != nil {
		return items
	}

	if len(items) == 0 {
		return items
	}

	for i, item := range items {
		if isShow {
			items[i].Type = "show"
		} else {
			items[i].Type = "movie"
		}
		items[i].Language = items[i].Country
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

func (t *Simkl) ItemsToObj(items []types.SimklItem) []map[string]any {
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

func (t *Simkl) CallEndpoint(endpoint string, method string, params types.SimklParams) (any, http.Header, int) {
	origEp := endpoint

	// if !strings.Contains(endpoint, "/oauth") && !strings.Contains(endpoint, "watchlist") {

	// 	objcached := t.cache.ReadCache("simkl", fmt.Sprintf("%s-%s-%v", method, endpoint), "data")
	// 	headercached := t.cache.ReadCache("simkl", fmt.Sprintf("%s-%s-%v", method, endpoint), "headers")

	// 	if objcached != nil {
	// 		return objcached, headercached.(http.Header), 200
	// 	}
	// }

	url := URL

	if strings.Contains(endpoint, "discover/trending") {
		url = "https://data.simkl.in"
	}

	appid := "client_id=" + os.Getenv("SIMKL_CLIENTID") + "&app-name=odin&app-version=0.1.0"

	var objmap any
	endpoint = common.ParseDates(endpoint, time.Now())

	hs := map[string]string{}
	if params.Headers != nil {
		hs = params.Headers
	}
	hs["user-agent"] = "Odin/0.1.0"

	request := resty.New().SetRetryCount(10).SetRetryWaitTime(time.Second * 3).R()
	request.SetHeader("simkl-api-version", "2").SetHeader("content-type", "application/json").SetHeader("simkl-api-key", os.Getenv("simkl_CLIENTID")).AddRetryCondition(func(r *resty.Response, err error) bool {
		return r.StatusCode() == 401
	}).SetHeaders(hs)

	var respHeaders http.Header
	status := 200
	if params.Body != nil {
		request.SetBody(params.Body)
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
				endpoint += "&" + appid + "&"
			} else {
				endpoint += "?" + appid + "&"
			}
			if !strings.Contains(endpoint, "limit=") {
				endpoint += "&limit=30"
			}
		}
	}

	if resp, err := r(fmt.Sprintf("%s%s", url, endpoint)); err == nil {
		respHeaders = resp.Header()
		status = resp.StatusCode()
		if status > 299 {
			log.Error("simkl error", "fetch", endpoint, "status", status)
			log.Debug("simkl error", "res", string(resp.Body()), "body", params.Body, "headers", respHeaders)
		} else {
			log.Debug("simkl fetch", "url", endpoint, "method", method, "status", status)
		}
		err := json.Unmarshal(resp.Body(), &objmap)
		if err != nil {
			log.Error("simkl", "unmarshal", err)
		}

		if strings.Contains(endpoint, "episodes/") {
			return objmap, respHeaders, status
		}
		switch objmap := objmap.(type) {

		case []any:

			items := t.ObjToItems(objmap, strings.Contains(endpoint, "/tv"))
			var wg sync.WaitGroup
			var mux sync.Mutex

			if len(items) == 0 || strings.Contains(endpoint, "sync/history") {
				if params.FetchTMDB {
					t.getImages(&wg, &mux, items)
				}
				wg.Wait()
				return items, respHeaders, status
			}

			// t.GetWatched(items)

			if strings.Contains(endpoint, "calendars") {
				items = t.removeSeason0(items)
				items = t.removeWatched(items)
				items = t.RemoveDuplicates(items)
			}

			t.getImages(&wg, &mux, items)

			wg.Wait()

			if status < 300 {
				t.cache.WriteCache("simkl", fmt.Sprintf("%s-%s-%v", method, endpoint), "data", &items, 1)
				t.cache.WriteCache("simkl", fmt.Sprintf("%s-%s-%v", method, endpoint), "headers", &respHeaders, 1)
			}

			return t.ItemsToObj(items), respHeaders, status

		default:

		}

	} else {
		log.Error("simkl", "endpoint", endpoint, "body", params.Body, "err", err)
	}

	itype := "movie"
	if strings.Contains(endpoint, "/tv") {
		itype = "show"
	}

	o := t.ObjToItems([]any{objmap}, itype == "show")
	parts := strings.Split(origEp, "/")
	last := parts[len(parts)-1]
	if id, err := strconv.Atoi(last); err == nil {
		o[0].IDs.SimklID = uint(id)
	}
	o[0].Type = itype
	o[0].Language = o[0].Country
	o[0].Original = &objmap
	os := t.ItemsToObj(o)
	log.Debug(os)

	return os, respHeaders, status
}

func (t *Simkl) getImages(wg *sync.WaitGroup, _ *sync.Mutex, objmap []types.SimklItem) {
	for k := range objmap {

		if objmap[k].Show != nil {
			if objmap[k].Show.Poster != "" {
				objmap[k].Show.Images.Poster = []string{"https://wsrv.nl?url=https://simkl.in/posters/" + objmap[k].Poster + "_m.jpg"}
			}

			if objmap[k].Show.Fanart != "" {
				objmap[k].Show.Images.Fanart = []string{"https://wsrv.nl?url=https://simkl.in/fanart/" + objmap[k].Fanart + "_medium.jpg"}
			}
			if len(objmap[k].Show.Images.Fanart) == 0 || len(objmap[k].Show.Images.Poster) == 0 || len(objmap[k].Show.Images.Logo) == 0 {
				wg.Add(1)
				go func() {
					t.tmdb.PopulateSimklTMDB(k, objmap)
					wg.Done()
				}()
			}
		}

		if objmap[k].Poster != "" {
			objmap[k].Images.Poster = []string{"https://wsrv.nl?url=https://simkl.in/posters/" + objmap[k].Poster + "_m.jpg"}
		}

		if objmap[k].Fanart != "" {
			objmap[k].Images.Fanart = []string{"https://wsrv.nl?url=https://simkl.in/fanart/" + objmap[k].Fanart + "_medium.jpg"}
		}

		if len(objmap[k].Images.Fanart) == 0 || len(objmap[k].Images.Poster) == 0 || len(objmap[k].Images.Logo) == 0 {
			wg.Add(1)
			go func() {
				t.tmdb.PopulateSimklTMDB(k, objmap)
				wg.Done()
			}()
		}
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
	Movie types.SimklItem `json:"movie"`
	Show  types.SimklItem `json:"show"`
	Plays int             `json:"plays"`
}

func (t *Simkl) GetWatched(objmap []types.SimklItem) []types.SimklItem {
	if len(objmap) == 0 {
		return objmap
	}
	return t.AssignWatched(objmap, objmap[0].Type)
}

func (t *Simkl) getHistory(htype string) []any {
	records, _ := t.app.Dao().FindRecordsByFilter("history", "type = {:htype}", "-watched_at", -1, 0, dbx.Params{"htype": htype})
	data := make([]any, 0)
	for _, r := range records {
		item := make(map[string]any)
		item["type"] = r.Get("type")
		item["simkl_id"] = r.Get("simkl_id")
		data = append(data, item)
	}
	return data
}

func (t *Simkl) AssignWatched(objmap []types.SimklItem, typ string) []types.SimklItem {
	if typ == "season" {
		typ = "episode"
	}
	history := t.getHistory(typ)
	for i, o := range objmap {
		if o.Episodes != nil {
			for j, e := range *o.Episodes {
				oid := e.IDs.SimklID
				(*objmap[i].Episodes)[j].Watched = false
				for _, h := range history {
					hid := uint(h.(map[string]any)["simkl_id"].(float64))
					if hid == oid {
						(*objmap[i].Episodes)[j].Watched = true
						log.Debug(oid, "watched", (*objmap[i].Episodes)[j].Watched)
						break
					}
				}
			}
		} else {

			oid := o.IDs.SimklID
			objmap[i].Watched = false
			for _, h := range history {
				hid := uint(h.(map[string]any)["simkl_id"].(float64))
				if hid == oid {
					objmap[i].Watched = true
					break
				}
			}
		}
	}
	newmap := make([]types.SimklItem, 0)

	for _, o := range objmap {
		newmap = append(newmap, o)
	}

	return newmap
}

func (t *Simkl) GetSeasons(id uint) any {

	endpoint := fmt.Sprintf("/tv/episodes/%d", id)
	result, _, _ := t.CallEndpoint(endpoint, "GET", types.SimklParams{})
	episodes := []types.Episode{}
	seasons := []types.Season{}

	d, err := json.Marshal(result)
	if err != nil {
		log.Error("simkl", "marshal", err)
		return nil
	}
	err = json.Unmarshal(d, &episodes)
	if err != nil {
		log.Error("simkl", "unmarshal", err)
		return nil
	}

	for _, e := range episodes {
		e.Number = e.Episode
		s := types.Season{Number: e.Season, Title: fmt.Sprintf("Season %d", e.Season), Episodes: []types.Episode{}}
		idx := funk.IndexOf(seasons, func(season types.Season) bool {
			return season.Number == e.Season
		})

		if idx == -1 {
			s.Episodes = append(s.Episodes, e)
			seasons = append(seasons, s)
		} else {
			seasons[idx].Episodes = append(seasons[idx].Episodes, e)
		}

	}

	return seasons
}

func (t *Simkl) FillCaches() {
	records := []*models.Record{}
	t.app.Dao().RecordQuery("users").All(&records)

	for _, r := range records {

		var sections struct {
			Home   []types.SimklSection `json:"home"`
			Movies []types.SimklSection `json:"movies"`
			Shows  []types.SimklSection `json:"shows"`
		}
		if err := r.UnmarshalJSONField("simkl_sections", &sections); err != nil {
			continue
		}
		for _, s := range sections.Home {
			t.cacheSection(s, r)
		}
		for _, s := range sections.Movies {
			t.cacheSection(s, r)
		}

		for _, s := range sections.Shows {
			t.cacheSection(s, r)
		}

	}
}

func (t *Simkl) cacheSection(s types.SimklSection, r *models.Record) {
	if !s.Cache {
		return
	}
	var token map[string]any
	theaders := map[string]string{}

	r.UnmarshalJSONField("simkl_token", &token)
	if token != nil && token["access_token"] != nil {
		theaders["authorization"] = "Bearer " + token["access_token"].(string)
	}

	id := r.GetId()

	u := common.ParseDates(s.URL, time.Now())
	log.Info(u, "id", id)

	t.CallEndpoint(s.URL, "GET", types.SimklParams{Headers: theaders, FetchTMDB: true})

	if !s.Paginate {
		return
	}

	for i := 1; i <= 10; i++ {
		t.CallEndpoint(u, "GET", types.SimklParams{Headers: theaders, FetchTMDB: true})
	}
}
