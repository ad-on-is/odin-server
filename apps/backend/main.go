package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
	"github.com/odin-movieshow/backend/common"
	"github.com/odin-movieshow/backend/downloader/alldebrid"
	"github.com/odin-movieshow/backend/downloader/realdebrid"
	"github.com/odin-movieshow/backend/helpers"
	"github.com/odin-movieshow/backend/imdb"
	"github.com/odin-movieshow/backend/scraper"
	"github.com/odin-movieshow/backend/settings"
	"github.com/odin-movieshow/backend/tmdb"
	"github.com/odin-movieshow/backend/trakt"

	"github.com/labstack/echo/v5"
	_ "github.com/odin-movieshow/backend/migrations"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/thoas/go-funk"
)

func getDevice(app *pocketbase.PocketBase, c echo.Context) (*models.Record, error) {
	device := c.Request().Header.Get("Device")
	return app.Dao().FindRecordById("devices", device)
}

func RequireDeviceOrRecordAuth(app *pocketbase.PocketBase) echo.MiddlewareFunc {
	mut := sync.RWMutex{}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			mut.Lock()
			record, _ := c.Get("authRecord").(*models.Record)
			mut.Unlock()
			if record == nil {
				d, _ := getDevice(app, c)
				if d != nil {
					if d.GetBool("verified") {
						u, err := app.Dao().FindRecordById("users", d.Get("user").(string))
						if err == nil {
							mut.Lock()
							c.Set("authRecord", u)
							mut.Unlock()
						}
					}
				}
			}

			if c.Get("authRecord") == nil {
				return apis.NewBadRequestError("Verified device code or Auth are required", nil)
			}

			return next(c)
		}
	}
}

func getVersion() string {
	b, err := os.ReadFile("./version.txt")
	if err == nil {
		v := string(b)
		v = strings.ReplaceAll(v, "\r\n", "")
		return v
	}
	return "Missing file: version.txt"
}

func main() {
	godotenv.Load()

	log.SetReportCaller(true)
	l, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))

	if err == nil {
		log.SetLevel(l)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	conf := pocketbase.Config{DefaultDev: false}
	app := pocketbase.NewWithConfig(conf)
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		settings := settings.New(app)
		helpers := helpers.New(app)
		tmdb := tmdb.New(settings, helpers)
		trakt := trakt.New(app, tmdb, settings, helpers)
		realdebrid := realdebrid.New(app, settings)
		alldebrid := alldebrid.New(app, settings)
		scraper := scraper.New(app, settings, helpers, realdebrid, alldebrid)

		email := "admin@odin.local"
		if os.Getenv("ADMIN_EMAIL") != "" {
			email = os.Getenv("ADMIN_EMAIL")
		}
		password := "odinAdmin1"
		if os.Getenv("ADMIN_PASSWORD") != "" {
			password = os.Getenv("ADMIN_PASSWORD")
		}
		a, _ := app.Dao().FindAdminByEmail(email)
		if a == nil {
			a = &models.Admin{Email: email}
			a.SetPassword(password)
			app.Dao().SaveAdmin(a)
		} else {
			a.SetPassword(password)
			app.Dao().SaveAdmin(a)
		}

		scheduler := cron.New()
		scheduler.MustAdd("hourly", "0 * * * *", func() {
			trakt.RefreshTokens()
			realdebrid.RefreshTokens()
			trakt.SyncHistory()
		})

		scheduler.MustAdd("daily", "0 4 * * *", func() {
			realdebrid.Cleanup()
		})

		scheduler.Start()

		go func() {
			// trakt.SyncHistory()
		}()

		e.Router.POST("/-/scrape", func(c echo.Context) error {
			mq := common.MqttClient()
			var pl common.Payload
			log.Debug("Scraping")
			c.Bind(&pl)
			log.Debug(pl)
			go func() {
				scraper.GetLinks(pl, mq)
			}()
			return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
		}, RequireDeviceOrRecordAuth(app))

		e.Router.GET("/-/imdb/:id", func(c echo.Context) error {
			id := c.PathParam("id")
			return c.JSON(http.StatusOK, imdb.Get(id))
		})

		e.Router.GET("/-/device/verify/:id/:name", func(c echo.Context) error {
			id := c.PathParam("id")
			name := c.PathParam("name")
			d, err := app.Dao().FindRecordById("devices", id)
			if err != nil {
				return c.JSON(http.StatusNotFound, nil)
			}
			uid := d.Get("user").(string)
			u, err := app.Dao().FindRecordById("users", uid)
			if err != nil {
				return c.JSON(http.StatusNotFound, nil)
			}
			d.Set("verified", true)
			d.Set("name", name)
			app.Dao().SaveRecord(d)
			log.Info("Device verified", "id", id)
			return c.JSON(http.StatusOK, u)
		}, apis.RequireGuestOnly())

		e.Router.GET("/-/user", func(c echo.Context) error {
			u := c.Get("authRecord")
			sections := make(map[string]any)
			err := u.(*models.Record).UnmarshalJSONField("trakt_sections", &sections)

			if err == nil {
				for _, t := range []string{"home", "movies", "shows"} {
					s := sections[t].([]any)
					for i := range s {
						title := common.ParseDates(sections[t].([]any)[i].(map[string]any)["title"].(string))
						url := common.ParseDates(sections[t].([]any)[i].(map[string]any)["url"].(string))
						sections[t].([]any)[i].(map[string]any)["title"] = title
						sections[t].([]any)[i].(map[string]any)["url"] = url
					}

				}

				str, err := json.Marshal(sections)
				if err == nil {
					u.(*models.Record).Set("trakt_sections", string(str))
				}
			}

			return c.JSON(http.StatusOK, u)
		}, RequireDeviceOrRecordAuth(app))

		e.Router.Any("/-/trakt/*", func(c echo.Context) error {
			info := apis.RequestInfo(c)

			id := info.AuthRecord.Id
			url := strings.ReplaceAll(c.Request().URL.String(), "/-/trakt", "")

			// rheaders := map[string]string{}

			// for k, v := range apis.RequestInfo(c).Headers {
			// 	if k == "Host" || k == "Connection" || k == "authorization" {
			// 		continue
			// 	}
			// 	rheaders[k] = v.(string)
			// }
			// trakt.Headers = rheaders
			// delete passed header of pocketbase

			t := make(map[string]any)
			u, _ := app.Dao().FindRecordById("users", id)
			u.UnmarshalJSONField("trakt_token", &t)
			// delete(trakt.Headers, "authorization")

			if t != nil && t["access_token"] != nil {
				trakt.Headers["authorization"] = "Bearer " + t["access_token"].(string)
			}
			if strings.Contains(url, "fresh=true") {
				delete(trakt.Headers, "authorization")
			}

			trakt.FetchTMDB = true

			jsonData := apis.RequestInfo(c).Data

			if strings.Contains(url, "scrobble/stop") {
				go func() {
					trakt.SyncHistory()
				}()
			}
			url = common.ParseDates(url)
			result, headers, status := trakt.CallEndpoint(
				url,
				c.Request().Method,
				jsonData,
				true,
			)

			for k, v := range headers {
				if funk.Contains([]string{
					"Content-Encoding",
					"Access-Control-Allow-Origin",
				}, k) {
					continue
				}
				c.Response().Header().Add(k, v[0])
			}
			c.Response().Status = status

			return c.JSON(http.StatusOK, result)
		}, RequireDeviceOrRecordAuth(app))

		e.Router.Any("/-/realdebrid/*", func(c echo.Context) error {
			url := strings.ReplaceAll(c.Request().URL.String(), "/-/realdebrid", "")
			var result interface{}
			var headers http.Header
			isAuth := false
			status := 0
			if strings.Contains(url, "/isAuth") {
				url = strings.ReplaceAll(url, "/isAuth", "")
				isAuth = true
			}

			data := apis.RequestInfo(c).Data

			var body map[string]string
			if data != nil {
				jm, err := json.Marshal(data)
				log.Debug(string(jm))
				if err == nil {
					err := json.Unmarshal(jm, &body)
					if err != nil {
						log.Warn(body)
					}
				}
			}

			headers, status = realdebrid.CallEndpoint(url, c.Request().Method, body, &result, isAuth)

			for k, v := range headers {
				if funk.Contains([]string{
					"Content-Encoding",
					"Access-Control-Allow-Origin",
				}, k) {
					continue
				}
				c.Response().Header().Add(k, v[0])
			}
			c.Response().Status = status
			return c.JSON(http.StatusOK, result)
		}, apis.RequireAdminAuth())

		e.Router.Any("/-/alldebrid/*", func(c echo.Context) error {
			url := strings.ReplaceAll(c.Request().URL.String(), "/-/alldebrid", "")
			var result interface{}
			headers, status := alldebrid.CallEndpoint(url, c.Request().Method, nil, &result)

			for k, v := range headers {
				if funk.Contains([]string{
					"Content-Encoding",
					"Access-Control-Allow-Origin",
				}, k) {
					continue
				}
				c.Response().Header().Add(k, v[0])
			}
			c.Response().Status = status
			return c.JSON(http.StatusOK, result)
		}, apis.RequireAdminAuth())

		e.Router.GET("/-/traktseasons/:id", func(c echo.Context) error {
			fmt.Println(c.PathParam("id"))
			id, _ := strconv.Atoi(c.PathParam("id"))
			res := trakt.GetSeasons(id)
			return c.JSON(http.StatusOK, res)
		}, RequireDeviceOrRecordAuth(app))

		e.Router.GET("/-/secrets", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{"TRAKT_CLIENTID": os.Getenv("TRAKT_CLIENTID"), "TRAKT_SECRET": os.Getenv("TRAKT_SECRET")})
		})

		e.Router.GET("/-/health", func(c echo.Context) error {
			ping := c.QueryParam("ping")
			if ping != "" {
				return c.String(http.StatusOK, "pong")
			}

			info := apis.RequestInfo(c)

			id := info.AuthRecord.Id

			t := make(map[string]any)
			u, _ := app.Dao().FindRecordById("users", id)
			u.UnmarshalJSONField("trakt_token", &t)
			// delete(trakt.Headers, "authorization")

			if t != nil && t["access_token"] != nil {
				trakt.Headers["authorization"] = "Bearer " + t["access_token"].(string)
			}

			var rd any
			realdebrid.CallEndpoint("/user", "GET", nil, &rd, false)
			var ad any
			alldebrid.CallEndpoint("/user", "GET", nil, &ad)
			tr, _, _ := trakt.CallEndpoint("/users/settings", "GET", nil, false)
			return c.JSON(http.StatusOK, map[string]any{"realdebrid": rd, "alldebrid": ad, "trakt": tr, "version": getVersion()})
		}, RequireDeviceOrRecordAuth(app))

		e.Router.GET("/-/tmdbseasons/:id", func(c echo.Context) error {
			fmt.Println(c.PathParam("id"))
			seasons := c.QueryParam("seasons")
			res := tmdb.GetEpisodes(c.PathParam("id"), strings.Split(seasons, ","))
			return c.JSON(http.StatusOK, res)
		}, RequireDeviceOrRecordAuth(app))

		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
