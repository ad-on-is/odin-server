package alldebrid

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/odin-movieshow/backend/settings"
	"github.com/odin-movieshow/backend/types"

	"github.com/charmbracelet/log"
	"github.com/go-resty/resty/v2"
	"github.com/pocketbase/pocketbase"
)

type AllDebrid struct {
	app      *pocketbase.PocketBase
	settings *settings.Settings
	Headers  map[string]any
}

type Magnet struct {
	Magnet string `json:"magnet"`
	Hash   string `json:"hash"`
	Name   string `json:"name"`
	Size   int    `json:"size"`
	Ready  bool   `json:"ready"`
	ID     int    `json:"id"`
}

type FileNode struct {
	N string     `json:"n"`
	S int        `json:"s"`
	L string     `json:"l"`
	E []FileNode `json:"e"`
}

func New(app *pocketbase.PocketBase, settings *settings.Settings) *AllDebrid {
	return &AllDebrid{app: app, settings: settings}
}

func (ad *AllDebrid) CallEndpoint(
	endpoint string,
	method string,
	body map[string]string,
	res interface{},
) (http.Header, int) {
	appendix := "?"
	if strings.Contains(endpoint, "?") {
		appendix = "&"
	}
	endpoint = endpoint + appendix + "agent=odinMovieShow&apikey=" + os.Getenv("ALLDEBRID_KEY")

	request := resty.New().
		SetRetryCount(3).
		SetRetryWaitTime(time.Second * 1).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return r.StatusCode() == 429
		}).
		R()
	request.SetResult(res)
	var respHeaders http.Header
	status := 200

	if body != nil {
		request.SetFormData(body)
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

	resp, err := r("https://api.alldebrid.com/v4" + endpoint)
	if err == nil {
		respHeaders = resp.Header()
		status = resp.StatusCode()
	} else {
		log.Error("alldebrid", "url", endpoint, "error", err)
		return respHeaders, status
	}

	if status > 299 {
		log.Error(
			"alldebrid",
			"status",
			status,
			"url",
			endpoint,
			"data",
			strings.ReplaceAll(string(resp.Body()), "\n", ""),
		)
	} else {
		log.Debug("alldebrid", "call", fmt.Sprintf("%s %s", method, endpoint))
	}

	return respHeaders, status
}

func getLinks(files []FileNode) []string {
	links := []string{}
	for _, v := range files {
		if v.L != "" {
			links = append(links, v.L)
			continue
		}
		if len(v.E) > 0 {
			links = append(links, getLinks(v.E)...)
		}
	}
	return links
}

func (ad *AllDebrid) Unrestrict(m string) []types.Unrestricted {
	var res struct {
		Data struct {
			Magnets []Magnet `json:"magnets"`
		} `json:"data"`
	}

	ad.CallEndpoint("/magnet/upload?magnets[]="+m, "GET", nil, &res)

	if len(res.Data.Magnets) == 0 {
		log.Debug("No magnets found for", m)
		return nil
	}

	magnetId := strconv.Itoa(int(res.Data.Magnets[0].ID))
	magnetReady := res.Data.Magnets[0].Ready
	defer ad.CallEndpoint(
		fmt.Sprintf("/magnet/delete?id=%s", magnetId),
		"GET",
		nil,
		nil,
	)

	if !magnetReady {
		log.Debug("Magnet not available in cache", magnetId)
		return nil
	}

	var files struct {
		Data struct {
			Magnets []struct {
				Files []FileNode `json:"files"`
			} `json:"magnets"`
		} `json:"data"`
	}

	ad.CallEndpoint("/magnet/files?id[]="+magnetId, "GET", nil, &files)

	if len(files.Data.Magnets) == 0 || len(files.Data.Magnets[0].Files) == 0 {
		log.Debug("No files found for magnet", magnetId)
		return nil
	}

	us := []types.Unrestricted{}

	for _, f := range files.Data.Magnets[0].Files {

		link := f.L
		if link == "" {
			continue
		}
		var u struct {
			Data struct {
				Link     string `json:"link"`
				Filename string `json:"filename"`
				Filesize int    `json:"filesize"`
			} `json:"data"`
		}
		ad.CallEndpoint("/link/unlock?link="+link, "GET", nil, &u)
		fname := u.Data.Filename
		mimetype := mime.TypeByExtension(fname[strings.LastIndex(fname, "."):])

		isVideo := strings.Contains(
			mimetype,
			"video",
		)

		match, _ := regexp.MatchString("^[Ss]ample[ -_]?[0-9].", fname)

		if !match && isVideo {
			un := types.Unrestricted{Filename: fname, Filesize: u.Data.Filesize, Download: u.Data.Link}
			us = append(us, un)
		}

	}
	return us
}
