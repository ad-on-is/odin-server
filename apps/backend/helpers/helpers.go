package helpers

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

type Helpers struct {
	app *pocketbase.PocketBase
}

func New(app *pocketbase.PocketBase) *Helpers {
	return &Helpers{app: app}
}

func (h *Helpers) ConvertToAnySlice(input []map[string]any) []any {
	result := make([]any, len(input))
	for i, v := range input {
		result[i] = v
	}
	return result
}

func (h *Helpers) GetTraktHeadersForUser(info *models.RequestInfo, url string) map[string]string {
	id := info.AuthRecord.Id

	t := make(map[string]any)
	u, _ := h.app.Dao().FindRecordById("users", id)
	u.UnmarshalJSONField("trakt_token", &t)
	// delete(trakt.Headers, "authorization")
	//
	theaders := map[string]string{}

	if t != nil && t["access_token"] != nil {
		theaders["authorization"] = "Bearer " + t["access_token"].(string)
	}
	if strings.Contains(url, "fresh=true") {
		delete(theaders, "authorization")
	}

	return theaders
}

func (h *Helpers) GetHomeDir() string {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return currentUser.HomeDir
}
