package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/odin-movieshow/backend/types"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	"github.com/redis/go-redis/v9"
)

type Cache struct {
	redis *redis.Client
	app   *pocketbase.PocketBase
}

func New(app *pocketbase.PocketBase) *Cache {
	opts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Error(err)
	}
	rdb := redis.NewClient(opts)
	err = rdb.Set(context.Background(), "test", "test", 0).Err()
	if err != nil {
		log.Error("REDIS", "Cannot connect to", opts.Addr, opts)
		log.Error(err)
	}
	return &Cache{app: app, redis: rdb}
}

func (c *Cache) ReadCache(service string, id string, resource string) interface{} {
	if c.redis == nil {
		return nil
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s-%s-%s", service, resource, id)

	data, err := c.redis.Get(ctx, key).Result()
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

func (c *Cache) WriteCache(service string, id string, resource string, data any, hours int) {
	d := time.Duration(hours) * time.Hour

	if c.redis == nil {
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
	err = c.redis.Set(ctx, key, m, d).Err()
	if err != nil {
		log.Error(err)
	}
}

func (c *Cache) ReadRDCache(resource string, magnet string) *types.Torrent {
	record, err := c.app.Dao().
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

func (c *Cache) ReadRDCacheByResource(resource string) []types.Torrent {
	records, err := c.app.Dao().
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

func (c *Cache) WriteRDCache(resource string, magnet string, data interface{}) {
	// c.WriteCache("stream", resource, magnet, &data, 12)
	log.Info("cache write", "for", "RD", "resource", resource)
	record, err := c.app.Dao().
		FindFirstRecordByFilter("rd_resolved", "magnet = {:magnet}", dbx.Params{"magnet": magnet})

	if err == nil {
		record.Set("data", &data)
		c.app.Dao().SaveRecord(record)
	} else {
		collection, _ := c.app.Dao().FindCollectionByNameOrId("rd_resolved")
		record := models.NewRecord(collection)
		record.Set("data", &data)
		record.Set("magnet", magnet)
		record.Set("resource", resource)
		c.app.Dao().SaveRecord(record)
	}
}
