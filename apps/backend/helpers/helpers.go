package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/charmbracelet/log"
	"github.com/odin-movieshow/backend/types"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	"github.com/redis/go-redis/v9"
)

type Helpers struct {
	app   *pocketbase.PocketBase
	redis *redis.Client
}

func New(app *pocketbase.PocketBase) *Helpers {
	opts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Error(err)
	}
	rdb := redis.NewClient(opts)
	err = rdb.Set(context.Background(), "test", "test", 0).Err()
	if err != nil {
		log.Error("REDIS", "Cannot conect to", opts.Addr, opts)

		log.Error(err)
	}

	return &Helpers{app: app, redis: rdb}
}

func (h *Helpers) GetHomeDir() string {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return currentUser.HomeDir
}

func (h *Helpers) ReadCache(service string, id string, resource string) interface{} {
	if h.redis == nil {
		return nil
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s-%s-%s", service, resource, id)

	data, err := h.redis.Get(ctx, key).Result()
	if err != nil {
		return nil
	}
	var res any
	err = json.Unmarshal([]byte(data), &res)
	if err != nil {
		return nil
	}
	log.Debug("cache hit", "for", service, "resource", resource, "id", id)
	return res
}

func (h *Helpers) WriteCache(service string, id string, resource string, data any, hours int) {
	d := time.Duration(hours) * time.Hour

	if h.redis == nil {
		return
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s-%s-%s", service, resource, id)

	if data == nil {
		return
	}
	log.Info("cache write", "for", service, "resource", resource, "id", id)
	m, err := json.Marshal(data)
	if err != nil {
		return
	}
	err = h.redis.Set(ctx, key, m, d).Err()
	if err != nil {
		log.Error(err)
	}
}

func (h *Helpers) ReadRDCache(resource string, magnet string) *types.Torrent {
	record, err := h.app.Dao().
		FindFirstRecordByFilter("rd_resolved", "magnet = {:magnet}", dbx.Params{"magnet": magnet})
	var res types.Torrent
	if err == nil {
		err := record.UnmarshalJSONField("data", &res)
		date := record.GetDateTime("updated")
		now := time.Now().Add(time.Duration((-8) * time.Hour))
		if err == nil {
			if date.Time().Before(now) {
				return nil
			}
			log.Debug("cache hit", "for", "RD", "resource", resource)
			return &res
		}
	}
	return nil
}

func (h *Helpers) ReadRDCacheByResource(resource string) []types.Torrent {
	records, err := h.app.Dao().
		FindRecordsByFilter("rd_resolved", "resource = {:resource}", "id", -1, 0, dbx.Params{"resource": resource})
	res := make([]types.Torrent, 0)
	if err == nil {
		for _, record := range records {
			var r types.Torrent
			date := record.GetDateTime("updated")
			now := time.Now().Add(time.Duration((-8) * time.Hour))
			// add 1 hour to date
			if date.Time().Before(now) {
				continue
			}
			err := record.UnmarshalJSONField("data", &r)
			if err == nil {
				res = append(res, r)
			}
		}
	}
	return res
}

func (h *Helpers) WriteRDCache(resource string, magnet string, data interface{}) {
	// h.WriteCache("stream", resource, magnet, &data, 12)
	log.Info("cache write", "for", "RD", "resource", resource)
	record, err := h.app.Dao().
		FindFirstRecordByFilter("rd_resolved", "magnet = {:magnet}", dbx.Params{"magnet": magnet})

	if err == nil {
		record.Set("data", &data)
		h.app.Dao().SaveRecord(record)
	} else {
		collection, _ := h.app.Dao().FindCollectionByNameOrId("rd_resolved")
		record := models.NewRecord(collection)
		record.Set("data", &data)
		record.Set("magnet", magnet)
		record.Set("resource", resource)
		h.app.Dao().SaveRecord(record)
	}
}
