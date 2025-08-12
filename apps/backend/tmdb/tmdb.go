package tmdb

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/odin-movieshow/backend/cache"
	"github.com/odin-movieshow/backend/settings"
	"github.com/odin-movieshow/backend/types"
	"github.com/thoas/go-funk"

	"github.com/charmbracelet/log"

	resty "github.com/go-resty/resty/v2"
)

type Tmdb struct {
	settings *settings.Settings
	cache    *cache.Cache
}

func New(settings *settings.Settings, cache *cache.Cache) *Tmdb {
	return &Tmdb{settings: settings, cache: cache}
}

const (
	TMDB_URL = "https://api.themoviedb.org/3"
)

func (t *Tmdb) GetItem(tmdbResource string, resource string, id uint) any {
	url := fmt.Sprintf("/%s/%d", tmdbResource, id)
	cache := t.cache.ReadCache("tmdb", fmt.Sprintf("%d", id), resource)
	if cache != nil {
		return cache
	} else {
		res := t.CallEndpoint(url)
		tmdb := t.prepare(res)
		res = t.tmdbToObj(tmdb)
		t.cache.WriteCache("tmdb", fmt.Sprintf("%d", id), resource, &res, 12)

		return res
	}
}

func (t *Tmdb) PopulateTMDB(
	k int,
	objmap []types.TraktItem,
) {
	resource := "movie"
	tmdbResource := "movie"
	if objmap[k].Type == "show" || objmap[k].Type == "episode" && objmap[k].Show != nil {
		resource = "show"
		tmdbResource = "tv"
	}
	if objmap[k].Season > 0 {
		resource = "season"
		tmdbResource = "tv"
	}
	id := objmap[k].IDs.Tmdb

	if objmap[k].Show != nil {
		id = objmap[k].Show.IDs.Tmdb
	}
	if objmap[k].IDs.Tmdb == 0 {
		return
	}

	tmdbObj := t.GetItem(tmdbResource, resource, id)

	if tmdbObj == nil {
		return
	}
	tmdb := t.prepare(tmdbObj)
	tmdbObj = t.tmdbToObj(tmdb)
	if len(objmap[k].Images.Fanart) == 0 {
		objmap[k].Images.Fanart = []string{"https://image.tmdb.org/t/p/w1280/" + tmdb.BackdropPath}
	}
	if len(objmap[k].Images.Logo) == 0 {
		objmap[k].Images.Logo = []string{"https://image.tmdb.org/t/p/w780/" + tmdb.LogoPath}
	}
	if len(objmap[k].Images.Poster) == 0 {
		objmap[k].Images.Poster = []string{"https://image.tmdb.org/t/p/w780/" + tmdb.PosterPath}
	}
	objmap[k].Tmdb = tmdbObj
	if objmap[k].Show != nil {
		objmap[k].Show.Tmdb = tmdbObj
		if len(objmap[k].Show.Images.Fanart) == 0 {
			objmap[k].Show.Images.Fanart = []string{"https://image.tmdb.org/t/p/w1280/" + tmdb.BackdropPath}
		}
		if len(objmap[k].Show.Images.Logo) == 0 {
			objmap[k].Show.Images.Logo = []string{"https://image.tmdb.org/t/p/w780/" + tmdb.LogoPath}
		}
		if len(objmap[k].Show.Images.Poster) == 0 {
			objmap[k].Show.Images.Poster = []string{"https://image.tmdb.org/t/p/w780/" + tmdb.PosterPath}
		}
	}
}

func (t *Tmdb) prepare(obj any) *types.TmdbItem {
	tmdb := types.TmdbItem{}
	ms, err := json.Marshal(obj)
	if err != nil {
		log.Error(err)
		return nil
	}
	err = json.Unmarshal(ms, &tmdb)
	if err != nil {
		log.Error(err)
		return nil
	}
	tmdb.Original = &obj

	if (tmdb).Credits != nil {
		if (tmdb).Credits.Crew != nil {
			(tmdb).Credits.Crew = nil
		}
		if tmdb.Credits.Cast != nil {
			// strip down cast
			cast := tmdb.Credits.Cast
			castlen := len(*cast)
			if castlen > 15 {
				castlen = 15
			}
			castcut := (*cast)[0:castlen]
			tmdb.Credits.Cast = &castcut
		}
	}

	if tmdb.Images != nil &&
		tmdb.Images.Logos != nil {

		for _, l := range *tmdb.Images.Logos {
			if l.Iso_639_1 != nil && *(l.Iso_639_1) == "en" {
				tmdb.LogoPath = l.FilePath
				break
			}
		}
		if tmdb.LogoPath == "" && len(*tmdb.Images.Logos) > 0 {
			tmdb.LogoPath = (*tmdb.Images.Logos)[0].FilePath
		}
		tmdb.Images = nil
	} else {
		tmdb.LogoPath = ""
	}

	return &tmdb
}

func (t *Tmdb) tmdbToObj(tmdb *types.TmdbItem) any {
	var obj any
	ms, err := json.Marshal(tmdb)
	if err == nil {
		err = json.Unmarshal(ms, &obj)
		if err == nil {
			orig := *(tmdb.Original)
			if (orig) == nil {
				return obj
			}
			for k, v := range (orig).(map[string]interface{}) {
				if funk.Contains([]string{"images", "credits"}, k) {
					continue
				}
				obj.(map[string]any)[k] = v
			}
			obj.(map[string]any)["original"] = nil
		}
	}
	return obj
}

func (t *Tmdb) CallEndpoint(endpoint string) any {
	var data any
	request := resty.New().
		SetRetryCount(3).
		SetTimeout(time.Second * 30).
		SetRetryWaitTime(time.Second).
		R()
	url := TMDB_URL + endpoint + "?api_key=" + os.Getenv("TMDB_KEY") + "&append_to_response=credits,videos,images"
	if res, err := request.SetResult(&data).SetHeader("content-type", "application/json").Get(url); err != nil {
		log.Error("TMDB", endpoint, "status", res.StatusCode())
	}
	return data
}

func (t *Tmdb) GetEpisodes(showId string, seasons []string) *[]any {
	var wg sync.WaitGroup
	res := make([]any, 0)

	for _, s := range seasons {
		wg.Add(1)
		go func(s string) {
			endpoint := fmt.Sprintf("/tv/%s/season/%s", showId, s)

			defer wg.Done()
			obj := t.CallEndpoint(endpoint)
			res = append(res, obj)
		}(s)
	}
	wg.Wait()

	return &res
}
